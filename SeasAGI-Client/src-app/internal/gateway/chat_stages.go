package gateway

// handleChat 的各处理阶段。主链路按固定顺序调用：
//
//	authorizeChat → checkRateLimits → readChatBody → governChatBody（DLP）→
//	parseChatRequest → prepareChatRequest → extractRouteHints →
//	路由规划 / 转发 / 记账（handleChat 内）→ writeChatResponse
//
// 拆分只改变代码组织，不改变任何阶段的行为与先后顺序；
// 行为由 gateway 的集成测试（protocol_smoke / mitm_routing / security_guard / usage 等）覆盖。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
	"github.com/SeasAGI/SeasAGI-Client/internal/rtk"
)

// chatBody 在阶段之间传递请求体的两种形态：转发用的字节与解析后的 map。
// raw 为 nil 表示请求体不是 JSON 对象（由解析阶段返回 400）。
type chatBody struct {
	bytes []byte
	raw   map[string]any
}

// remarshal 在 raw 被改写后重新序列化 bytes，保证直接转发字节的路径（Grant 中继）
// 与转发 map 的路径拿到同样的内容；序列化失败时保留原字节。
func (b *chatBody) remarshal() {
	if remarshaled, err := json.Marshal(b.raw); err == nil {
		b.bytes = remarshaled
	}
}

// authorizeChat 校验本地访问令牌（节点入口已由 NodeProxy 校验 HMAC，可免令牌），
// 并把调用方请求头注入 context，供 Grant 转发透传协议版本头（anthropic-version 等）。
func (s *Service) authorizeChat(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if !s.validateToken(r) {
		if v, _ := r.Context().Value(nodeAuthCtxKey{}).(bool); !v {
			writeJSONError(w, http.StatusUnauthorized, "Invalid access token")
			return r, false
		}
	}
	return r.WithContext(context.WithValue(r.Context(), grantHeaderCtxKey{}, r.Header)), true
}

// readChatBody 读取有上限的请求体（超限 413，CL-13），并为 Gemini 原生路径注入 model/stream。
func (s *Service) readChatBody(w http.ResponseWriter, r *http.Request) (*chatBody, bool) {
	bodyBytes, ok := readLimitedBody(w, r, maxJSONRequestBody)
	if !ok {
		return nil, false
	}
	body := &chatBody{bytes: bodyBytes}
	_ = json.Unmarshal(bodyBytes, &body.raw)

	// Gemini 原生请求把模型放在 URL 路径中，body 通常不含 model：
	// 注入后再走统一解析链路，否则必然 400（P0-10）。
	if info, ok := r.Context().Value(geminiPathCtxKey{}).(geminiPathInfo); ok && info.Model != "" {
		if body.raw == nil {
			body.raw = map[string]any{}
		}
		if m, _ := body.raw["model"].(string); strings.TrimSpace(m) == "" {
			body.raw["model"] = info.Model
		}
		if info.Action == "streamGenerateContent" {
			body.raw["stream"] = true
		}
		body.remarshal()
	}
	return body, true
}

// governChatBody 是 DLP 阶段：先按原始内容做提示注入检测，再按需对 messages 做
// PII/DLP 脱敏（PII、PEM 私钥块、secret 赋值行），防止敏感信息泄露给第三方 LLM。
// 脱敏原地改写 raw，命中后重新序列化 bytes，使 BYOK 与 Grant 中继两条路径都拿到脱敏内容。
func (s *Service) governChatBody(secCfg config.SecurityConfig, body *chatBody) injectionFinding {
	if body.raw == nil {
		return injectionFinding{}
	}
	injection, maskedHits := s.security.governRequestBody(secCfg, body.raw)
	if maskedHits > 0 {
		body.remarshal()
		logging.Warningf("PII/DLP masked %d content block(s) in chat request", maskedHits)
	}
	return injection
}

// parseChatRequest 解析为 canonical 请求：forced 为空时自动探测协议，
// 否则按指定协议解析（/v1/messages、/v1/responses 专用路由）。
func parseChatRequest(bodyBytes []byte, forced protocol.Format) (*protocol.CanonicalRequest, error) {
	if forced == "" {
		return protocol.ParseRequest(bodyBytes)
	}
	return protocol.ParseRequestAs(bodyBytes, forced)
}

// prepareChatRequest 做转发前的请求改写：消息扁平化、默认 Combo、MITM 路由决策、
// 工具去重、Caveman 注入与 RTK 压缩。MITM 严格模式头部不一致时直接 403 并返回 false。
func (s *Service) prepareChatRequest(w http.ResponseWriter, r *http.Request, req *protocol.CanonicalRequest, body *chatBody, cfg config.AppConfig) (mitmDecision, bool) {
	req.Messages = flattenMessagesContent(req.Messages)

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
				return mitm, false
			}
			logging.Warningf("mitm header mismatch: target=%s host=%s (lenient, passthrough)", mitm.Target, r.Header.Get(config.HeaderMITMHost))
		}
		if mitm.Matched && mitm.GrantID == "" {
			req.Model = mitm.ResolvedTarget
		}
	}

	if body.raw != nil {
		body.raw = protocol.DedupToolsInRequest(body.raw)
		if toolsRaw, ok := body.raw["tools"]; ok {
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

	// Caveman/RTK 只改 canonical messages；这里把修改后的 messages 回写 raw 并重新
	// 序列化 bytes，使 Grant 中继（直接转发 bytes）与 Passthrough（转发 raw）
	// 两条路径同样拿到注入/压缩后的内容，模式与 DLP 一致。仅限 openai-chat 源格式：
	// gemini/anthropic 等源的 wire 键不同，回写会引入非法字段。
	if body.raw != nil && req.SourceFormat == protocol.FormatOpenAIChat {
		body.raw["messages"] = req.Messages
		body.remarshal()
	}
	return mitm, true
}

// routeHints 是路由规划所需的请求级提示。
type routeHints struct {
	taskType    string
	intent      *routing.IntentContext
	constraints map[string]any
	debugTrace  bool
}

// extractRouteHints 提取任务类型（显式 task_type > 工具 > 意图识别 > chat）、
// 请求级约束（请求体 + MITM 规则，规则优先）与调试注入开关。
// 任务类型写入 req.Extra["_task_type"]；调试开关从 raw 中移除，不向下游透传。
func extractRouteHints(body *chatBody, req *protocol.CanonicalRequest, mitm mitmDecision) routeHints {
	rawBody := body.raw
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
	if req.Extra == nil {
		req.Extra = make(map[string]any)
	}
	req.Extra["_task_type"] = taskType

	constraints := make(map[string]any)
	if rawBody != nil {
		for _, key := range []string{"max_price", "max_latency_ms", "data_policy"} {
			if v, ok := rawBody[key]; ok {
				constraints[key] = v
			}
		}
	}
	// 合并 MITM 映射注入的请求级约束（规则显式配置优先于请求体）。
	for k, v := range mitm.Constraints {
		constraints[k] = v
	}

	// 调试注入开关（仅本地 Playground 使用）：命中后在非流式响应附加 _combo_steps/_intent
	debugTrace := false
	if rawBody != nil {
		if v, ok := rawBody["_seasagi_debug"].(bool); ok {
			debugTrace = v
			delete(rawBody, "_seasagi_debug") // 不向下游透传
		}
	}
	return routeHints{taskType: taskType, intent: intent, constraints: constraints, debugTrace: debugTrace}
}

// writeChatResponse 把上游响应写回客户端：按源协议转换（/v1/messages、/v1/responses）、
// 按需合并 reasoning、或在调试模式下附加执行链信息；其余情况原样透传。
func writeChatResponse(w http.ResponseWriter, resp *http.Response, req *protocol.CanonicalRequest, needsReasoning, debugTrace bool, routeSteps []logs.RouteStep, intent *routing.IntentContext) {
	copyHeaders(w.Header(), resp.Header)
	// 源协议非 OpenAI Chat 时需把上游响应转换回该协议，
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
						_, _ = w.Write(mergedBytes)
						return
					}
				}
			}
			_, _ = w.Write(respBytes)
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
