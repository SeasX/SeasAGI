package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
	"github.com/SeasAGI/SeasAGI-Client/internal/providers"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
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
	// 阶段 1 鉴权（含节点 HMAC 免令牌）。
	r, ok := s.authorizeChat(w, r)
	if !ok {
		return
	}

	// 阶段 2 请求级限流预检：月度成本硬上限 / RPM / TPM / 最小间隔 / 全局并发。
	release, allowed := s.checkRateLimits(w)
	if !allowed {
		return
	}
	defer release()

	// 阶段 3 读取请求体（有上限）+ DLP：提示注入检测与 PII/DLP 脱敏。
	body, ok := s.readChatBody(w, r)
	if !ok {
		return
	}
	secCfg := s.configSvc.GetSecurityConfig()
	injection := s.governChatBody(secCfg, body)

	// 阶段 4 解析为 canonical 请求。
	req, err := parseChatRequest(body.bytes, forcedSourceFormat)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 旁路采集响应用量（状态码/TTFT/token/限流），不影响原有响应语义。
	capture := newUsageCapture(w, req.Stream)
	w = capture

	// 阶段 5 转发前改写（默认 Combo、MITM、工具去重、Caveman/RTK）。
	cfg := s.configSvc.GetConfig()
	// 工具发出的原始模型名（在任何改写之前），用于 MITM 日志。
	requestedModel := req.Model
	mitm, ok := s.prepareChatRequest(w, r, req, body, cfg)
	if !ok {
		return
	}

	// 阶段 6 路由提示：任务类型、意图、请求级约束、调试开关。
	hints := extractRouteHints(body, req, mitm)
	taskType, intent, appliedConstraints, debugTrace := hints.taskType, hints.intent, hints.constraints, hints.debugTrace
	rawBody, bodyBytes := body.raw, body.bytes

	// 阶段 7 路由规划 → 转发 → 记账（defer）→ 写回响应。
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
	writeChatResponse(w, resp, req, needsReasoning, debugTrace, routeSteps, intent)
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
