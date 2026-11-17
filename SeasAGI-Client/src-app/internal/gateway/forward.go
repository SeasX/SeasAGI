package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
	"github.com/SeasAGI/SeasAGI-Client/internal/providers"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
)

// grantClient 返回 Grant 中继 HTTP 客户端（构造期已初始化，once 兜底测试字面量）。
func (s *Service) grantClient() *http.Client {
	s.grantHTTPClientOnce.Do(func() {
		if s.grantHTTPClient == nil {
			s.grantHTTPClient = &http.Client{Timeout: 300 * time.Second}
		}
	})
	return s.grantHTTPClient
}

// forwardRequest tries each candidate step in order.
// When consecutive candidates share the same StepRole, they are treated as step-internal fallbacks.
// Request constraints (max_price, max_latency_ms, data_policy) are checked before sending.
func (s *Service) forwardRequest(ctx context.Context, candidates []routing.PlanStep, req *protocol.CanonicalRequest, sessionKey string) (*http.Response, *routing.PlanStep, []logs.RouteStep, error) {
	var lastErr error
	attempts := make([]logs.RouteStep, 0, len(candidates))
	classifier := providers.NewErrorClassifier()
	optCfg := s.configSvc.GetOptimizationConfig()

	if optCfg.StickyEnabled {
		candidates = s.resolver.ReorderBySession(candidates, sessionKey)
	}

	healthyCandidates := make([]routing.PlanStep, 0, len(candidates))
	if optCfg.HealthCheckEnabled {
		for _, step := range candidates {
			if step.Channel.HealthStatus == "unhealthy" {
				continue
			}
			healthyCandidates = append(healthyCandidates, step)
		}
		if len(healthyCandidates) == 0 {
			healthyCandidates = candidates
		}
	} else {
		healthyCandidates = candidates
	}

	var sorted []routing.PlanStep
	// BYOK: enterprise channels first, then platform channels (double-layer fallback)
	enterpriseFirst := make([]routing.PlanStep, 0, len(healthyCandidates))
	platformFallback := make([]routing.PlanStep, 0, len(healthyCandidates))
	for _, step := range healthyCandidates {
		if step.Channel.Source == "enterprise" {
			enterpriseFirst = append(enterpriseFirst, step)
		} else {
			platformFallback = append(platformFallback, step)
		}
	}
	sorted = append(enterpriseFirst, platformFallback...)

	if optCfg.PenaltyEnabled {
		sorted = s.sortCandidatesByPenalty(sorted)
	}

	// Track step roles for within-step fallback detection
	var prevStepRole string
	for _, step := range sorted {
		attempt := logs.RouteStep{
			ChannelID:     step.Channel.ChannelID,
			UpstreamModel: step.UpstreamModel,
			StepRole:      step.StepRole,
			WithinStep:    step.StepRole != "" && step.StepRole == prevStepRole,
		}
		prevStepRole = step.StepRole

		cb := s.getCircuitBreaker(step.Channel.ChannelID)
		if !cb.Allow() {
			attempt.Status = "circuit_open"
			attempt.Error = "circuit breaker is open"
			attempts = append(attempts, attempt)
			lastErr = fmt.Errorf("circuit breaker open for channel %s", step.Channel.ChannelID)
			continue
		}

		providerCfg, err := s.providerConfigForChannel(step.Channel)
		if err != nil {
			attempt.Status = "configuration_error"
			attempt.Error = err.Error()
			attempts = append(attempts, attempt)
			lastErr = err
			continue
		}

		// 多 key 轮询：在一个候选渠道内依次尝试多个 key。处于冷却（等价于"拉黑"）的 key
		// 会被跳过，鉴权失败/限流的 key 会被记入冷却——避免永远只用 keys[0]，也避免因单个
		// 坏 key 而跳过整个渠道。
		keyCandidates := step.Channel.APIKeys
		rotateKeys := len(keyCandidates) > 1

		// Per-channel concurrency limit (P2-23)
		if !s.concurrencyLimiter.Acquire(step.Channel.ChannelID) {
			attempt.Status = "concurrency_limit"
			attempt.Error = "channel at max concurrent requests"
			attempts = append(attempts, attempt)
			lastErr = fmt.Errorf("channel %s at concurrency limit", step.Channel.ChannelID)
			continue
		}
		// 流式请求下槽位必须持有到响应体读完/关闭，否则 MaxConcurrent 对流式形同虚设（CL-15）。
		var releaseOnce sync.Once
		releaseSlot := func() {
			releaseOnce.Do(func() { s.concurrencyLimiter.Release(step.Channel.ChannelID) })
		}

		// reasoning 占位符按本候选渠道判定：不修改原始请求，回退到其它厂商时不受影响（CL-14）。
		messages := req.Messages
		if protocol.NeedsReasoningContent(step.Channel.ProviderType, step.UpstreamModel) {
			messages = messagesWithReasoningPlaceholder(req.Messages)
		}

		executor := providers.ResolveExecutor(providerCfg)
		var upstreamResp *providers.UpstreamResponse
		var execErr error
		cooldownSkipped := 0

		if rotateKeys {
			rotator := s.getKeyRotator(step.Channel.ChannelID, keyCandidates)
			tried := make(map[string]bool, len(keyCandidates))
			for i := 0; i < len(keyCandidates); i++ {
				key := rotator.Next()
				if key == "" || tried[key] {
					break
				}
				tried[key] = true
				if optCfg.CooldownEnabled && s.cooldownMgr.IsOnCooldown(key) {
					cooldownSkipped++
					continue
				}
				cfg := *providerCfg
				cfg.APIKey = key
				attemptStart := time.Now()
				resp, err := executor.ChatCompletions(ctx, &cfg, &providers.UpstreamRequest{
					Model:    step.UpstreamModel,
					Messages: messages,
					Stream:   req.Stream,
					Extra:    req.Extra,
				})
				if err == nil {
					upstreamResp = resp
					execErr = nil
					routing.RecordLatency(step.Channel.ChannelID, float64(time.Since(attemptStart).Milliseconds()))
					break
				}
				execErr = err
				if optCfg.CooldownEnabled {
					if pe, ok := err.(*providers.ProviderError); ok && providers.ShouldRotateKey(pe.StatusCode) {
						s.cooldownMgr.SetCooldownWithReason(key, "key_failed")
					}
				}
			}
		} else {
			attemptStart := time.Now()
			upstreamResp, execErr = executor.ChatCompletions(ctx, providerCfg, &providers.UpstreamRequest{
				Model:    step.UpstreamModel,
				Messages: messages,
				Stream:   req.Stream,
				Extra:    req.Extra,
			})
			if execErr == nil {
				routing.RecordLatency(step.Channel.ChannelID, float64(time.Since(attemptStart).Milliseconds()))
			}
		}
		if execErr == nil {
			cb.RecordSuccess()
			s.recordPenaltySuccess(step)
			s.resolver.RecordSessionStep(sessionKey, step)
			attempt.Status = "success"
			attempts = append(attempts, attempt)
			body := upstreamResp.Body
			if body != nil {
				// 槽位所有权转移给响应体：只有 Body.Close（或流结束关闭）才释放（CL-15）。
				body = &releaseOnCloseBody{ReadCloser: body, release: releaseSlot}
			} else {
				releaseSlot()
			}
			return &http.Response{
				StatusCode: upstreamResp.Status,
				Header:     upstreamResp.Headers,
				Body:       body,
			}, &step, attempts, nil
		}
		// 失败/回退：立即归还槽位，避免错误路径泄漏并发配额。
		releaseSlot()

		// 熔断只对服务端/网络类错误计数，避免单个坏 key 触发的 4xx 把整渠道熔断。
		if circuitBreakerCounts(execErr) {
			cb.RecordFailure()
		}
		errMsg := execErr.Error()
		if cooldownSkipped > 0 {
			errMsg = fmt.Sprintf("%s (%d key(s) skipped on cooldown)", errMsg, cooldownSkipped)
		}
		// 上游错误体可能回显 key，落库/回前端前统一脱敏（CL-S13）。
		attempt.Error = s.sanitizeClientError(errMsg)
		lastErr = execErr

		err = execErr
		providerErr, ok := err.(*providers.ProviderError)
		if ok {
			_, retryable := classifier.Classify(providerErr.StatusCode, providerErr.Message)
			if retryable {
				attempt.Status = "retryable_failure"
				if providerErr.StatusCode == 429 && optCfg.PenaltyEnabled {
					s.recordPenaltyRateLimit(step)
				}
			} else {
				attempt.Status = "failure"
			}
			attempts = append(attempts, attempt)
			if !retryable {
				return nil, &step, attempts, err
			}
			continue
		}

		errClass, retryable := classifier.Classify(0, err.Error())
		_ = errClass
		if retryable {
			attempt.Status = "retryable_failure"
		} else {
			attempt.Status = "failure"
		}
		attempts = append(attempts, attempt)
		if !retryable {
			return nil, &step, attempts, err
		}
	}

	return nil, nil, attempts, lastErr
}

func (s *Service) getCircuitBreaker(channelID string) *providers.CircuitBreaker {
	s.cbMu.RLock()
	cb, exists := s.circuitBreakers[channelID]
	s.cbMu.RUnlock()
	if exists {
		return cb
	}

	s.cbMu.Lock()
	defer s.cbMu.Unlock()
	if cb, exists = s.circuitBreakers[channelID]; exists {
		return cb
	}
	cb = providers.NewCircuitBreaker(5, 30*time.Second)
	s.circuitBreakers[channelID] = cb
	return cb
}

func (s *Service) getKeyRotator(channelID string, keys []string) *providers.KeyRotator {
	s.cbMu.RLock()
	kr, exists := s.keyRotators[channelID]
	s.cbMu.RUnlock()
	if exists {
		kr.SetKeys(keys)
		return kr
	}

	s.cbMu.Lock()
	defer s.cbMu.Unlock()
	if kr, exists = s.keyRotators[channelID]; exists {
		kr.SetKeys(keys)
		return kr
	}
	kr = providers.NewKeyRotator(keys)
	s.keyRotators[channelID] = kr
	return kr
}

func (s *Service) providerConfigForChannel(channel config.Channel) (*providers.ProviderConfig, error) {
	apiKey := ""
	if channel.ChannelType == "custom" {
		var err error
		apiKey, err = keychain.GetChannelKey(channel.ChannelID)
		if err != nil {
			return nil, err
		}
	}

	// OAuth token refresh (P2-21): if channel has OAuth config, ensure valid token
	if channel.OAuthRefreshToken != "" && channel.OAuthTokenURL != "" {
		// 优先使用内存中最新轮换的 refresh token，避免重复使用已失效的旧令牌（CL-11）。
		refreshToken := channel.OAuthRefreshToken
		if cached := s.oauthRefresher.GetToken(channel.ChannelID); cached != nil && cached.RefreshToken != "" {
			refreshToken = cached.RefreshToken
		}
		token, err := s.oauthRefresher.EnsureValidToken(
			channel.ChannelID,
			refreshToken,
			channel.OAuthClientID,
			channel.OAuthClientSecret,
			channel.OAuthTokenURL,
		)
		if err == nil && token != "" {
			apiKey = token
		}
	}

	return &providers.ProviderConfig{
		ChannelType:            channel.ChannelType,
		ProviderType:           channel.ProviderType,
		BaseURL:                channel.BaseURL,
		APIKey:                 apiKey,
		PlatformToken:          s.authSvc.GetPlatformToken(),
		ProviderSpecificConfig: channel.ProviderSpecificConfig,
	}, nil
}

func statusFromProviderError(err *providers.ProviderError) int {
	if err == nil {
		return http.StatusBadGateway
	}
	if err.StatusCode > 0 {
		return err.StatusCode
	}
	switch err.Type {
	case "configuration_error":
		return http.StatusBadRequest
	case "network_error":
		return http.StatusBadGateway
	default:
		return http.StatusBadGateway
	}
}

func (s *Service) penaltyKey(step routing.PlanStep) string {
	return step.Channel.ChannelID + ":" + step.UpstreamModel
}

func (s *Service) sortCandidatesByPenalty(candidates []routing.PlanStep) []routing.PlanStep {
	if len(candidates) <= 1 {
		return candidates
	}

	type weighted struct {
		step    routing.PlanStep
		penalty int
	}

	weightedSteps := make([]weighted, len(candidates))
	for i, step := range candidates {
		p := s.penaltyMgr.GetPenalty(s.penaltyKey(step))
		weightedSteps[i] = weighted{step: step, penalty: p}
	}

	for i := 0; i < len(weightedSteps); i++ {
		for j := i + 1; j < len(weightedSteps); j++ {
			if weightedSteps[j].penalty < weightedSteps[i].penalty {
				weightedSteps[i], weightedSteps[j] = weightedSteps[j], weightedSteps[i]
			}
		}
	}

	result := make([]routing.PlanStep, len(weightedSteps))
	for i, ws := range weightedSteps {
		result[i] = ws.step
	}
	return result
}

func (s *Service) recordPenaltySuccess(step routing.PlanStep) {
	s.penaltyMgr.RecordSuccess(s.penaltyKey(step))
}

func (s *Service) recordPenaltyRateLimit(step routing.PlanStep) {
	s.penaltyMgr.RecordRateLimit(s.penaltyKey(step))
}

// circuitBreakerCounts 判断一次上游失败是否应计入熔断器。仅服务端错误（5xx）与网络类
// 错误计数；客户端错误（401/403/404/429 等 4xx）不计，避免单个坏 key 或单次限流把
// 整个渠道熔断。
func circuitBreakerCounts(err error) bool {
	if err == nil {
		return false
	}
	var pe *providers.ProviderError
	if errors.As(err, &pe) {
		return pe.StatusCode == 0 || pe.StatusCode >= 500
	}
	return true
}

func summarizeRoutePlan(plan []routing.PlanStep) string {
	if len(plan) == 0 {
		return ""
	}
	items := make([]string, 0, len(plan))
	for _, step := range plan {
		items = append(items, fmt.Sprintf("%s:%s", step.Channel.ChannelID, step.UpstreamModel))
	}
	return strings.Join(items, " -> ")
}

func summarizeAttemptTrace(attempts []logs.RouteStep) string {
	if len(attempts) == 0 {
		return ""
	}
	items := make([]string, 0, len(attempts))
	for _, step := range attempts {
		item := fmt.Sprintf("%s:%s(%s", step.ChannelID, step.UpstreamModel, step.Status)
		if step.StepRole != "" {
			item += fmt.Sprintf(",%s", step.StepRole)
		}
		if step.Error != "" {
			item += "," + step.Error
		}
		item += ")"
		items = append(items, item)
	}
	return strings.Join(items, " -> ")
}

// forwardViaGrant routes a request through the enterprise relay gateway using the
// user's active Token Market grant.
//
// 协议与模型处理：
//   - 按请求来源协议选择 relay 端点（/relay/messages、/relay/responses、/relay/gemini/*、
//     /relay/chat/completions），由 relay 透传到 A 的上游；响应保持同一协议，客户端无需转换。
//   - upstreamModel 非空时改写请求体 model，并附 X-Upstream-Model 供 relay 记账与覆盖。
func (s *Service) forwardViaGrant(ctx context.Context, relayURL, grantID string, sourceFormat protocol.Format,
	upstreamModel, requestedPath string, bodyBytes []byte) (*http.Response, error) {
	client := s.grantClient()

	body := bodyBytes
	if upstreamModel != "" {
		var payload map[string]any
		if err := json.Unmarshal(bodyBytes, &payload); err == nil {
			payload["model"] = upstreamModel
			if out, err := json.Marshal(payload); err == nil {
				body = out
			}
		}
	}

	url := strings.TrimRight(relayURL, "/") + grantRelayPath(sourceFormat, requestedPath)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.authSvc.GetPlatformToken())
	req.Header.Set("X-Grant-Id", grantID)
	if upstreamModel != "" {
		req.Header.Set("X-Upstream-Model", upstreamModel)
	}
	// 透传 SDK 依赖的协议版本头。
	for _, h := range []string{"anthropic-version", "anthropic-beta", "openai-beta", "x-goog-user-project"} {
		if v := reqHeaderValue(ctx, h); v != "" {
			req.Header.Set(h, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("grant relay returned %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return resp, nil
}

// grantRelayPath 按来源协议选择 relay 端点。
func grantRelayPath(sourceFormat protocol.Format, requestedPath string) string {
	switch sourceFormat {
	case protocol.FormatAnthropic:
		return "/relay/messages"
	case protocol.FormatOpenAIResponses:
		return "/relay/responses"
	case protocol.FormatGemini:
		if requestedPath != "" {
			return "/relay/gemini" + requestedPath
		}
		return "/relay/gemini"
	default:
		return "/relay/chat/completions"
	}
}

// reqHeaderValue 从 context 中取调用方请求头（由 handleChat 注入）。
func reqHeaderValue(ctx context.Context, key string) string {
	if h, ok := ctx.Value(grantHeaderCtxKey{}).(http.Header); ok {
		return h.Get(key)
	}
	return ""
}

type grantHeaderCtxKey struct{}

// proxyResponse copies an upstream HTTP response to the client ResponseWriter.
func (s *Service) proxyResponse(w http.ResponseWriter, resp *http.Response) {
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
