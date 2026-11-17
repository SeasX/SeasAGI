package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
	"github.com/SeasAGI/SeasAGI-Client/internal/providers"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
	"github.com/SeasAGI/SeasAGI-Client/internal/rtk"
)

func (s *Service) handleListModels(w http.ResponseWriter, r *http.Request) {
	if !s.validateToken(r) {
		writeJSONError(w, http.StatusUnauthorized, "Invalid access token")
		return
	}

	channels, _ := s.configSvc.ListChannels()
	seen := map[string]struct{}{}
	data := make([]map[string]any, 0)
	for _, ch := range channels {
		for _, model := range ch.Models {
			if _, exists := seen[model]; exists {
				continue
			}
			seen[model] = struct{}{}
			data = append(data, map[string]any{
				"id":       model,
				"object":   "model",
				"owned_by": ch.ProviderType,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   data,
	})
}

func (s *Service) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, "")
}

// handleAnthropicMessages 处理 Anthropic Messages API（/v1/messages）。被接管的
// Claude Code 等 SDK 使用该协议，网关按 Anthropic 语义解析请求并转换响应回该协议。
func (s *Service) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, protocol.FormatAnthropic)
}

// handleResponses 处理 OpenAI Responses API（/v1/responses）。被接管的 Codex 等工具
// 使用该协议。
func (s *Service) handleResponses(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, protocol.FormatOpenAIResponses)
}

// geminiPathCtxKey 携带从 /v1beta/models/{model}:{action} 解析出的模型与动作。
type geminiPathCtxKey struct{}

type geminiPathInfo struct {
	Model  string
	Action string
}

// handleGemini 处理 Gemini / AI Studio 原生路径（/v1beta/models/*），
// 供被接管的 Gemini SDK 走订阅授权转发。
// Gemini 原生请求把模型放在 URL 中（body 通常不含 model），这里先解析路径注入 payload。
func (s *Service) handleGemini(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "only generateContent/streamGenerateContent are supported")
		return
	}
	model, action := protocol.ParseGeminiPath(r.URL.Path)
	if model == "" {
		writeJSONError(w, http.StatusBadRequest, "model is required: /v1beta/models/{model}:generateContent")
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), geminiPathCtxKey{}, geminiPathInfo{Model: model, Action: action}))
	s.handleChat(w, r, protocol.FormatGemini)
}

// handleChat 是聊天补全主链路。forcedSourceFormat 为空时按请求体自动探测协议；
// 非空时按指定协议解析（用于 /v1/messages、/v1/responses 专用路由）。
func (s *Service) handleChat(w http.ResponseWriter, r *http.Request, forcedSourceFormat protocol.Format) {
	authorized := s.validateToken(r)
	if !authorized {
		// A 端节点入口已在 NodeProxy 校验 HMAC 签名，无需本地访问令牌。
		if v, _ := r.Context().Value(nodeAuthCtxKey{}).(bool); !v {
			writeJSONError(w, http.StatusUnauthorized, "Invalid access token")
			return
		}
	}

	// 把调用方请求头注入 context，供 Grant 转发透传协议版本头（anthropic-version 等）。
	r = r.WithContext(context.WithValue(r.Context(), grantHeaderCtxKey{}, r.Header))

	// 请求级限流预检：月度成本硬上限 / RPM / TPM / 最小间隔 / 全局并发。
	release, allowed := s.checkRateLimits(w)
	if !allowed {
		return
	}
	defer release()

	// DLP (Data Loss Prevention): 内容治理在下方完成 —— 先按原始内容做提示注入
	// 检测，再按需对 messages 做 PII/DLP 脱敏（PII、PEM 私钥块、secret 赋值行），
	// 防止敏感信息泄露给第三方 LLM。

	// 请求体上限：超过上限直接 413，避免无界读取造成内存放大（CL-13）。
	bodyBytes, ok := readLimitedBody(w, r, maxJSONRequestBody)
	if !ok {
		return
	}

	var rawBody map[string]any
	_ = json.Unmarshal(bodyBytes, &rawBody)

	// Gemini 原生请求把模型放在 URL 路径中，body 通常不含 model：
	// 注入后再走统一解析链路，否则必然 400（P0-10）。
	if info, ok := r.Context().Value(geminiPathCtxKey{}).(geminiPathInfo); ok && info.Model != "" {
		if rawBody == nil {
			rawBody = map[string]any{}
		}
		if m, _ := rawBody["model"].(string); strings.TrimSpace(m) == "" {
			rawBody["model"] = info.Model
		}
		if info.Action == "streamGenerateContent" {
			rawBody["stream"] = true
		}
		if remarshaled, mErr := json.Marshal(rawBody); mErr == nil {
			bodyBytes = remarshaled
		}
	}

	// 脱敏会原地改写 rawBody 中的 messages；命中后重新序列化请求体，确保 BYOK 与
	// Grant 中继两条转发路径都拿到脱敏后的内容。
	secCfg := s.configSvc.GetSecurityConfig()
	injection := injectionFinding{}
	if rawBody != nil {
		var maskedHits int
		injection, maskedHits = s.security.governRequestBody(secCfg, rawBody)
		if maskedHits > 0 {
			if remarshaled, mErr := json.Marshal(rawBody); mErr == nil {
				bodyBytes = remarshaled
			}
			logging.Warningf("PII/DLP masked %d content block(s) in chat request", maskedHits)
		}
	}

	req, err := func() (*protocol.CanonicalRequest, error) {
		if forcedSourceFormat == "" {
			return protocol.ParseRequest(bodyBytes)
		}
		return protocol.ParseRequestAs(bodyBytes, forcedSourceFormat)
	}()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 旁路采集响应用量（状态码/TTFT/token/限流），不影响原有响应语义。
	capture := newUsageCapture(w, req.Stream)
	w = capture

	req.Messages = flattenMessagesContent(req.Messages)

	cfg := s.configSvc.GetConfig()

	// 工具发出的原始模型名（在任何改写之前），用于 MITM 匹配与日志。
	requestedModel := req.Model

	if cfg.DefaultComboName != "" && req.Model == cfg.DefaultModel {
		req.Model = cfg.DefaultComboName
	}

	// MITM 路由决策：命中「改写」动作后覆盖 req.Model（优先级高于默认 Combo 兜底，
	// 与「MITM 规则 > DefaultModel→DefaultComboName」的优先级链一致）。
	mitm := mitmDecision{}
	if cfg.MITMRouting != nil {
		mitm = resolveMITMRoute(r.Header, effectiveMITMRouting(*cfg.MITMRouting), requestedModel)
		// 头部一致性校验（P2-13）：strict 模式拒绝，lenient 模式仅告警放行。
		if mitm.HeaderMismatch {
			if cfg.MITMRouting.SecurityMode == config.MITMSecurityStrict {
				writeJSONError(w, http.StatusForbidden, "MITM header mismatch: target does not match original host")
				return
			}
			logging.Warningf("mitm header mismatch: target=%s host=%s (lenient, passthrough)", mitm.Target, r.Header.Get(config.HeaderMITMHost))
		}
		if mitm.Matched && mitm.GrantID == "" {
			req.Model = mitm.ResolvedTarget
		}
	}

	if rawBody != nil {
		rawBody = protocol.DedupToolsInRequest(rawBody)
		if toolsRaw, ok := rawBody["tools"]; ok {
			if toolsList, ok := toolsRaw.([]any); ok && len(toolsList) > 0 {
				if req.Extra == nil {
					req.Extra = make(map[string]any)
				}
				req.Extra["tools"] = toolsList
			}
		}
	}

	if cfg.CavemanEnabled {
		req.Messages = rtk.CavemanInjectIntoCanonical(req.Messages, true, cfg.CavemanStyle)
	}

	// RTK：压缩请求中回传的工具结果（role=tool），降低上游 token 消耗
	rtkPipeline := s.currentRTKPipeline()
	if rtkPipeline != nil && rtkPipeline.Enabled {
		req.Messages = rtk.ApplyPipelineToMessages(req.Messages, rtkPipeline)
	}

	// Caveman/RTK 只改 canonical messages；这里把修改后的 messages 回写 rawBody 并重新
	// 序列化 bodyBytes，使 Grant 中继（直接转发 bodyBytes）与 Passthrough（转发 rawBody）
	// 两条路径同样拿到注入/压缩后的内容，模式与上方 DLP 一致。仅限 openai-chat 源格式：
	// gemini/anthropic 等源的 wire 键不同，回写会引入非法字段。
	if rawBody != nil && req.SourceFormat == protocol.FormatOpenAIChat {
		rawBody["messages"] = req.Messages
		if remarshaled, mErr := json.Marshal(rawBody); mErr == nil {
			bodyBytes = remarshaled
		}
	}

	// Extract task_type from request for task-aware routing
	taskType := ""
	if rawBody != nil {
		if tt, ok := rawBody["task_type"].(string); ok {
			taskType = tt
		} else if tools, ok := rawBody["tools"].([]any); ok && len(tools) > 0 {
			taskType = "tools"
		}
	}
	// v0.2.0: 场景化意图识别（Fast Path 启发式），驱动 Combo 按需适配路由
	intent := routing.DetectIntent(req.Messages)
	if taskType == "" || taskType == "chat" {
		// Auto-detect task type from message content when not explicitly set
		if intent.TaskType != "chat" {
			taskType = intent.TaskType
		}
	}
	if taskType == "" {
		taskType = "chat"
	}
	// Store task type in extra for downstream use
	if req.Extra == nil {
		req.Extra = make(map[string]any)
	}
	req.Extra["_task_type"] = taskType

	// Extract request-level constraints from the request body
	appliedConstraints := make(map[string]any)
	if rawBody != nil {
		if v, ok := rawBody["max_price"]; ok {
			appliedConstraints["max_price"] = v
		}
		if v, ok := rawBody["max_latency_ms"]; ok {
			appliedConstraints["max_latency_ms"] = v
		}
		if v, ok := rawBody["data_policy"]; ok {
			appliedConstraints["data_policy"] = v
		}
	}
	// 合并 MITM 映射注入的请求级约束（规则显式配置优先于请求体）。
	if len(mitm.Constraints) > 0 {
		for k, v := range mitm.Constraints {
			appliedConstraints[k] = v
		}
	}

	// 调试注入开关（仅本地 Playground 使用）：命中后在非流式响应附加 _combo_steps/_intent
	debugTrace := false
	if rawBody != nil {
		if v, ok := rawBody["_seasagi_debug"].(bool); ok {
			debugTrace = v
			delete(rawBody, "_seasagi_debug") // 不向下游透传
		}
	}

	normalizedModel, plan, err := s.resolver.ResolveChatPlan(req.Model, taskType, intent)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Model = normalizedModel

	// 目标 channel 已知，叠加 per-channel 限流覆盖。
	if len(plan) > 0 && !s.checkChannelRateLimit(w, plan[0].Channel.ChannelID) {
		return
	}

	// Apply request-level constraints to filter candidates
	if len(appliedConstraints) > 0 {
		plan = s.resolver.FilterCandidatesByConstraints(plan, appliedConstraints)
	}

	// 注意：needsReasoning 不能用「首选渠道」预判（CL-14）——回退到其它厂商时
	// 会注入错误的 reasoning 占位/合并推理。改为在确定实际使用的渠道后重算（见下方 usedStep）。
	start := time.Now()
	sessionKey := s.resolver.GetSessionKey(req.Messages)
	status := "failure"
	var errCode *string
	var errMessage *string
	selectedChannelID := ""
	selectedChannelName := ""
	selectedUpstreamModel := ""
	var routeSteps []logs.RouteStep
	routeTrace := summarizeRoutePlan(plan)

	// Serialize constraints for logging
	constraintsJSON := ""
	if len(appliedConstraints) > 0 {
		if b, err := json.Marshal(appliedConstraints); err == nil {
			constraintsJSON = string(b)
		}
	}

	defer func() {
		duration := float64(time.Since(start).Milliseconds())
		intentScenario := ""
		if intent != nil {
			intentScenario = intent.Scenario
		}
		// 采集响应侧指标（HTTP 码 / TTFT / token）写入请求日志，供监控与排障。
		httpStatus, ttftMs, usageObj, _ := capture.usageSnapshot()
		if ttftMs < 0 {
			ttftMs = 0
		}
		var inputTokens, outputTokens int64
		if usageObj != nil {
			inputTokens = tokenValue(usageObj, "prompt_tokens", "input_tokens")
			outputTokens = tokenValue(usageObj, "completion_tokens", "output_tokens")
		}
		// 默认不落请求体/prompt；只有显式打开调试开关才记录失败请求的脱敏快照（CL-S13）。
		requestPayload := ""
		if status != "success" && rawBody != nil && logging.PayloadLoggingEnabled() {
			requestPayload = logging.ProtectPayloadForLog(rawBody)
		}
		// MITM 可观测：仅在请求带 MITM 标记时记录来源与改写链路。
		mitmTarget, mitmRuleID, mitmRequested, mitmResolved := "", "", "", ""
		if mitm.Intercepted {
			mitmTarget = mitm.Target
			mitmRuleID = mitm.RuleID
			mitmRequested = requestedModel
			mitmResolved = mitm.ResolvedTarget
		}
		_ = s.logSvc.RecordLog(logs.RequestLog{
			RequestID:          fmt.Sprintf("req_%d", time.Now().UnixNano()),
			CreatedAt:          time.Now().UTC().Format(time.RFC3339),
			LogicalModelName:   req.Model,
			ChannelID:          selectedChannelID,
			UpstreamModel:      selectedUpstreamModel,
			RouteTrace:         routeTrace,
			RouteSteps:         routeSteps,
			Status:             status,
			DurationMs:         duration,
			HTTPStatus:         httpStatus,
			TTFTMs:             ttftMs,
			InputTokens:        inputTokens,
			OutputTokens:       outputTokens,
			ErrorCode:          errCode,
			ErrorMessage:       errMessage,
			AppliedConstraints: constraintsJSON,
			IntentScenario:     intentScenario,
			MITMTarget:         mitmTarget,
			MITMRuleID:         mitmRuleID,
			RequestedModel:     mitmRequested,
			ResolvedTarget:     mitmResolved,
			MITMHeaderMismatch: mitm.HeaderMismatch,
			RequestPayload:     requestPayload,
		})
		s.recordUsageAndTokens(capture, usageMeta{
			ChannelID:   selectedChannelID,
			ChannelName: selectedChannelName,
			Model:       req.Model,
			Headers:     w.Header(),
			Always:      true,
		})
	}()

	// 提示注入处置：block 直接拦截（HTTP 4xx，用量不记账），log 仅告警后放行。
	if injection.Detected {
		if secCfg.PromptInjectionAction == config.PromptInjectionBlock {
			status = "failure"
			code := "prompt_injection_blocked"
			message := "Request blocked by security policy: potential prompt injection detected"
			errCode = &code
			errMessage = &message
			writeJSONError(w, http.StatusBadRequest, message)
			return
		}
		logging.Warningf("prompt injection detected: severity=%s model=%s", injection.Severity, req.Model)
	}

	// MITM 规则直选订阅资产（market_asset）：命中后直接经企业 relay 转发该 Grant。
	if mitm.GrantID != "" {
		relayURL := s.configSvc.GetSelectedGrantRelayURL()
		if relayURL == "" {
			writeJSONError(w, http.StatusBadGateway, "token market relay url is not configured")
			return
		}
		resp, gerr := s.forwardViaGrant(r.Context(), relayURL, mitm.GrantID, req.SourceFormat, "", r.URL.Path, bodyBytes)
		if gerr == nil {
			status = "success"
			selectedChannelID = config.MarketGrantChannelPrefix + mitm.GrantID
			selectedChannelName = "Token Market Relay"
			defer resp.Body.Close()
			s.proxyResponse(w, resp)
			return
		}
		logging.Warningf("mitm market_asset relay failed: %v; falling back to normal routing", gerr)
	}

	// P8: Grant routing — if user has an active grant, route through enterprise relay.
	// 优先级统一（§4.9-4）：MITM 规则命中改写时，跳过全局 Grant 短路，改用改写后的
	// Combo/模型路由，避免改写被全局开关绕过。
	grantRelayURL := s.configSvc.GetSelectedGrantRelayURL()
	grantID := s.configSvc.GetSelectedGrantID()
	if grantRelayURL != "" && grantID != "" && !mitm.Matched {
		resp, err := s.forwardViaGrant(r.Context(), grantRelayURL, grantID, req.SourceFormat, "", r.URL.Path, bodyBytes)
		if err == nil {
			status = "success"
			selectedChannelID = "grant_" + grantID
			selectedChannelName = "Token Market Relay"
			defer resp.Body.Close()
			s.proxyResponse(w, resp)
			return
		}
		// Grant routing failed, fall through to normal BYOK routing
	}

	// Combo 内嵌 Token 市场资产：将计划中的 grant:// 虚拟渠道经企业 relay 转发；
	// 全部失败时回退到其余（自有 Key）步骤。市场优先（P3-21）可关闭：
	// 显式 market_priority=false 时忽略市场资产，直接走自有 Key 步骤。
	if grantSteps, rest := splitMarketGrantSteps(plan); len(grantSteps) > 0 {
		combo, ok := s.configSvc.GetModelCombo(normalizedModel)
		marketFirst := marketFirstForCombo(combo, ok)
		if marketFirst {
			for _, step := range grantSteps {
				gid := grantIDFromChannelID(step.Channel.ChannelID)
				if gid == "" || step.Channel.BaseURL == "" {
					continue
				}
				resp, gerr := s.forwardViaGrant(r.Context(), step.Channel.BaseURL, gid, req.SourceFormat,
					step.UpstreamModel, r.URL.Path, bodyBytes)
				if gerr != nil {
					continue
				}
				status = "success"
				selectedChannelID = step.Channel.ChannelID
				selectedChannelName = "Token Market Relay"
				selectedUpstreamModel = step.UpstreamModel
				defer resp.Body.Close()
				s.proxyResponse(w, resp)
				return
			}
		}
		// 未启用市场优先（或全部 grant 步骤失败）→ 回退到自有 Key 步骤。
		plan = rest
		if len(plan) == 0 {
			status = "failure"
			code := "grant_relay_error"
			message := "all token market grant routes failed"
			if !marketFirst {
				message = "market assets disabled for this combo and no fallback channel available"
			}
			errCode = &code
			errMessage = &message
			writeJSONError(w, http.StatusBadGateway, message)
			return
		}
	}

	resp, usedStep, attempts, forwardErr := s.forwardRequest(r.Context(), plan, req, sessionKey)
	routeSteps = attempts
	routeTrace = summarizeAttemptTrace(attempts)
	if usedStep != nil {
		selectedChannelID = usedStep.Channel.ChannelID
		selectedChannelName = usedStep.Channel.DisplayName
		selectedUpstreamModel = usedStep.UpstreamModel
	}
	// 按实际命中的渠道/上游模型判断是否需要 reasoning 处理，避免回退后仍按首选渠道判定（CL-14）。
	needsReasoning := usedStep != nil &&
		protocol.NeedsReasoningContent(usedStep.Channel.ProviderType, usedStep.UpstreamModel)
	if forwardErr != nil {
		// Record failure metrics for the combo
		s.recordComboMetrics(normalizedModel, len(attempts), status == "success")
		providerErr, ok := forwardErr.(*providers.ProviderError)
		if ok {
			code := providerErr.Type
			message := s.security.sanitizeError(secCfg, providerErr.Message)
			errCode = &code
			errMessage = &message
			writeJSONError(w, statusFromProviderError(providerErr), message)
			return
		}

		code := "upstream_error"
		message := s.security.sanitizeError(secCfg, forwardErr.Error())
		errCode = &code
		errMessage = &message
		writeJSONError(w, http.StatusBadGateway, message)
		return
	}
	defer resp.Body.Close()

	status = "success"
	// Record success metrics for the combo
	s.recordComboMetrics(normalizedModel, len(attempts), true)
	copyHeaders(w.Header(), resp.Header)
	// 源协议非 OpenAI Chat（/v1/messages、/v1/responses）时需把上游响应转换回该协议，
	// 转换后长度可能变化，故移除上游 Content-Length 交给 Go 重新分块。
	convertSource := req.SourceFormat != protocol.FormatOpenAIChat
	injectDebug := debugTrace && !req.Stream
	if injectDebug || (convertSource && !req.Stream) {
		w.Header().Del("Content-Length")
	}
	w.WriteHeader(resp.StatusCode)

	if req.Stream {
		if convertSource {
			converted := protocol.ConvertStreamToSource(resp.Body, req.SourceFormat, req.Model)
			_, _ = io.Copy(w, converted)
			_ = converted.Close()
			return
		}
		if needsReasoning {
			_ = protocol.ProcessReasoningSSEStream(resp.Body, w)
			return
		}
		// 响应方向原样透传：模型输出（最终回答/工具入参）不做 RTK 改写
		_, _ = io.Copy(w, resp.Body)
		return
	}

	if convertSource {
		respBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return
		}
		converted, convErr := protocol.ConvertOpenAIResponseToSource(respBytes, req.SourceFormat, req.Model)
		if convErr != nil {
			_, _ = w.Write(respBytes)
			return
		}
		_, _ = w.Write(converted)
		return
	}

	if needsReasoning {
		respBytes, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			var respMap map[string]any
			if json.Unmarshal(respBytes, &respMap) == nil {
				reasoning, cleanBody := protocol.ExtractReasoningFromResponse(respMap)
				if reasoning != "" {
					merged := protocol.MergeReasoningIntoContent(cleanBody, reasoning)
					if mergedBytes, marshalErr := json.Marshal(merged); marshalErr == nil {
						w.Write(mergedBytes)
						return
					}
				}
			}
			w.Write(respBytes)
			return
		}
	}

	if injectDebug {
		// ponytail: 仅覆盖普通非流式路径；reasoning 分支提前 return，不含调试字段
		respBytes, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			_, _ = w.Write(injectDebugKeys(respBytes, routeSteps, intent))
			return
		}
	}

	_, _ = io.Copy(w, resp.Body)
}

// injectDebugKeys 在非流式 JSON 响应中附加执行链轨迹与识别意图，供 Playground 诊断展示。
func injectDebugKeys(body []byte, routeSteps []logs.RouteStep, intent *routing.IntentContext) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil || m == nil {
		return body
	}
	if len(routeSteps) > 0 {
		m["_combo_steps"] = routeSteps
	}
	if intent != nil {
		m["_intent"] = intent
	}
	if out, err := json.Marshal(m); err == nil {
		return out
	}
	return body
}

// messagesWithReasoningPlaceholder 返回注入了 reasoning_content 占位符的 messages 副本。
// 只复制最后一条消息，避免污染原始请求：某候选渠道失败回退到其它厂商时，原始
// messages 仍保持干净（CL-14）。
func messagesWithReasoningPlaceholder(messages []map[string]any) []map[string]any {
	if len(messages) == 0 {
		return messages
	}
	copied := make([]map[string]any, len(messages))
	copy(copied, messages)
	lastIdx := len(copied) - 1
	last := make(map[string]any, len(copied[lastIdx])+1)
	for key, value := range copied[lastIdx] {
		last[key] = value
	}
	copied[lastIdx] = last
	return protocol.InjectReasoningPlaceholder(copied)
}

func flattenMessagesContent(messages []map[string]any) []map[string]any {
	for _, msg := range messages {
		content, ok := msg["content"]
		if !ok {
			continue
		}
		arr, ok := content.([]any)
		if !ok || len(arr) == 0 {
			continue
		}
		onlyText := true
		var parts []string
		for _, block := range arr {
			b, ok := block.(map[string]any)
			if !ok {
				onlyText = false
				break
			}
			typ, _ := b["type"].(string)
			if typ == "text" {
				text, _ := b["text"].(string)
				parts = append(parts, text)
			} else {
				onlyText = false
				break
			}
		}
		if onlyText && len(parts) > 0 {
			msg["content"] = strings.Join(parts, "\n")
		}
	}
	return messages
}
