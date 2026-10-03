package gateway

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/auth"
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
	"github.com/SeasAGI/SeasAGI-Client/internal/providers"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
	"github.com/SeasAGI/SeasAGI-Client/internal/rtk"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

type Service struct {
	mu              sync.RWMutex
	running         bool
	listenPort      int
	accessToken     string
	server          *http.Server
	configSvc       *config.Service
	authSvc         *auth.Service
	logSvc          *logs.Service
	usageSvc        *usage.Service
	security        *securityGuard
	resolver        *routing.Resolver
	rtkPipeline     *rtk.Pipeline
	circuitBreakers map[string]*providers.CircuitBreaker
	keyRotators     map[string]*providers.KeyRotator
	cbMu            sync.RWMutex
	penaltyMgr      *providers.PenaltyManager
	cooldownMgr     *providers.CooldownManager
	healthChecker   *providers.HealthChecker
	healthCtx       context.Context
	healthCancel    context.CancelFunc
	// Combo metrics tracking
	comboMetrics map[string]*ComboRouteMetrics
	metricsMu    sync.RWMutex
	// WebSocket bridge
	wsBridge *WSBridge
	// State persistence
	stateStore   *providers.StateStore
	stopAutoSave func()
	// OAuth token refresh (P2-21)
	oauthRefresher *providers.OAuthTokenRefresher
	// Per-channel concurrency limiter (P2-23)
	concurrencyLimiter *providers.ConcurrencyLimiter
	// Request-level rate limiter (RPM/TPM/min-interval/global concurrency)
	rateLimiter *rateLimiter
	// HTTP client for grant relay routing
	grantHTTPClient *http.Client
}

// ComboRouteMetrics tracks route-level metrics per combo
type ComboRouteMetrics struct {
	ComboName        string  `json:"combo_name"`
	TotalRequests    int     `json:"total_requests"`
	TotalFallbacks   int     `json:"total_fallbacks"`
	Step1Success     int     `json:"step1_success"`
	LastStepFallback int     `json:"last_step_fallback"`
	Step1HitRate     float64 `json:"step1_hit_rate"`
	LastStepHitRate  float64 `json:"last_step_hit_rate"`
	AvgAttempts      float64 `json:"avg_attempts"`
	TaskType         string  `json:"task_type"`
}

func NewService(
	listenPort int,
	accessToken string,
	configSvc *config.Service,
	authSvc *auth.Service,
	logSvc *logs.Service,
) *Service {
	cfg := configSvc.GetConfig()
	optCfg := configSvc.GetOptimizationConfig()

	cooldownDur := 120 * time.Second
	if optCfg.CooldownSec > 0 {
		cooldownDur = time.Duration(optCfg.CooldownSec) * time.Second
	}

	healthInterval := 5 * time.Minute
	if optCfg.HealthCheckSec > 0 {
		healthInterval = time.Duration(optCfg.HealthCheckSec) * time.Second
	}
	maxFailures := optCfg.HealthMaxFailures
	if maxFailures <= 0 {
		maxFailures = 3
	}

	penaltyMgr := providers.NewPenaltyManager()
	if optCfg.PenaltyDecaySec > 0 {
		penaltyMgr.SetDecayInterval(time.Duration(optCfg.PenaltyDecaySec) * time.Second)
	}

	healthChecker := providers.NewHealthChecker(
		func() []providers.HealthChannelInfo {
			channels, _ := configSvc.ListChannels()
			infos := make([]providers.HealthChannelInfo, 0, len(channels))
			for _, ch := range channels {
				infos = append(infos, providers.HealthChannelInfo{
					ChannelID:    ch.ChannelID,
					ChannelType:  ch.ChannelType,
					ProviderType: ch.ProviderType,
					BaseURL:      ch.BaseURL,
					APIKey:       ch.APIKey,
					Enabled:      ch.Enabled,
					HealthStatus: ch.HealthStatus,
				})
			}
			return infos
		},
		configSvc.UpdateChannelHealth,
	)
	healthChecker.SetCheckInterval(healthInterval)
	healthChecker.SetMaxFailures(maxFailures)

	cooldownMgr := providers.NewCooldownManager(cooldownDur)

	// Initialize state persistence (PenaltyManager + CooldownManager)
	stateStore, err := providers.NewStateStore("")
	if err != nil {
		// Non-fatal: continue without persistence
		logging.Warningf("state store init failed: %v", err)
	} else {
		if err := stateStore.LoadAll(penaltyMgr, cooldownMgr); err != nil {
			logging.Warningf("state restore failed: %v", err)
		}
	}

	svc := &Service{
		listenPort:      listenPort,
		accessToken:     accessToken,
		configSvc:       configSvc,
		authSvc:         authSvc,
		logSvc:          logSvc,
		resolver:        routing.NewResolver(configSvc),
		rtkPipeline:     rtk.NewPipeline(cfg.RTKEnabled, cfg.RTKMaxOutputChars),
		circuitBreakers: make(map[string]*providers.CircuitBreaker),
		keyRotators:     make(map[string]*providers.KeyRotator),
		penaltyMgr:      penaltyMgr,
		cooldownMgr:     cooldownMgr,
		healthChecker:   healthChecker,
		healthCtx:       nil,
		healthCancel:    nil,
		comboMetrics:    make(map[string]*ComboRouteMetrics),
		stateStore:      stateStore,
		security:        newSecurityGuard(),
		rateLimiter:     newRateLimiter(),
	}

	// Start auto-save if state store is available
	if stateStore != nil {
		svc.stopAutoSave = stateStore.StartAutoSave(penaltyMgr, cooldownMgr, 30*time.Second)
	}

	// Initialize OAuth token refresher (P2-21)
	svc.oauthRefresher = providers.NewOAuthTokenRefresher(60 * time.Second)

	// Initialize per-channel concurrency limiter (P2-23)
	svc.concurrencyLimiter = providers.NewConcurrencyLimiter()
	// Populate limits from existing channels
	if channels, err := configSvc.ListChannels(); err == nil {
		for _, ch := range channels {
			if ch.MaxConcurrent > 0 {
				svc.concurrencyLimiter.SetLimit(ch.ChannelID, ch.MaxConcurrent)
			}
		}
	}

	// Normalize channel weights (P2-22)
	_ = configSvc.NormalizeChannelWeights()

	return svc
}

func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", s.handleListModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/embeddings", s.handleEmbeddings)
	mux.HandleFunc("/v1/images/generations", s.handleImageGenerations)
	mux.HandleFunc("/v1/audio/speech", s.handleTTS)
	mux.HandleFunc("/v1/audio/transcriptions", s.handleSTT)

	// WebSocket bridge（与 HTTP 主链路共用治理逻辑）
	if s.wsBridge == nil {
		s.wsBridge = NewWSBridge(s)
	}
	mux.HandleFunc("/v1/ws", s.wsBridge.HandleWS)

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.listenPort),
		Handler: mux,
	}

	s.mu.Lock()
	s.server = server
	s.running = true
	s.healthCtx, s.healthCancel = context.WithCancel(ctx)
	s.mu.Unlock()

	go s.healthChecker.Start(s.healthCtx)

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		s.server.Close()
	}
	if s.healthCancel != nil {
		s.healthCancel()
	}
	// Stop auto-save and persist final state
	if s.stopAutoSave != nil {
		s.stopAutoSave()
	}
	if s.stateStore != nil {
		_ = s.stateStore.SaveAll(s.penaltyMgr, s.cooldownMgr)
		_ = s.stateStore.Close()
	}
	if s.cooldownMgr != nil {
		s.cooldownMgr.Stop()
	}
	// Clean up OAuth token cache (P2-21)
	if s.oauthRefresher != nil {
		// Nothing to persist — tokens are runtime-only
	}
	s.running = false
}

func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *Service) GetListenPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listenPort
}

func (s *Service) SetAccessToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessToken = token
}

// SetUsageService 注入用量记账服务；未注入时网关跳过用量记录。
func (s *Service) SetUsageService(svc *usage.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usageSvc = svc
}

// currentUsageService 返回用量记账服务快照，避免热更新时的数据竞争。
func (s *Service) currentUsageService() *usage.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usageSvc
}

// SetRTKConfig 热更新 RTK 压缩管线（设置页保存后立即生效，无需重启网关）。
func (s *Service) SetRTKConfig(enabled bool, maxOutputChars int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rtkPipeline = rtk.NewPipeline(enabled, maxOutputChars)
}

// currentRTKPipeline 返回 RTK 管线快照，避免热更新时的数据竞争。
func (s *Service) currentRTKPipeline() *rtk.Pipeline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rtkPipeline
}

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
	if !s.validateToken(r) {
		writeJSONError(w, http.StatusUnauthorized, "Invalid access token")
		return
	}

	// 请求级限流预检：月度成本硬上限 / RPM / TPM / 最小间隔 / 全局并发。
	release, allowed := s.checkRateLimits(w)
	if !allowed {
		return
	}
	defer release()

	// DLP (Data Loss Prevention): 内容治理在下方完成 —— 先按原始内容做提示注入
	// 检测，再按需对 messages 做 PII/DLP 脱敏（PII、PEM 私钥块、secret 赋值行），
	// 防止敏感信息泄露给第三方 LLM。

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var rawBody map[string]any
	_ = json.Unmarshal(bodyBytes, &rawBody)

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

	req, err := protocol.ParseRequest(bodyBytes)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 旁路采集响应用量（状态码/TTFT/token/限流），不影响原有响应语义。
	capture := newUsageCapture(w, req.Stream)
	w = capture

	req.Messages = flattenMessagesContent(req.Messages)

	cfg := s.configSvc.GetConfig()

	if cfg.DefaultComboName != "" && req.Model == cfg.DefaultModel {
		req.Model = cfg.DefaultComboName
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

	if len(plan) > 0 {
		providerType := plan[0].Channel.ProviderType
		if protocol.ShouldPassthrough(req.SourceFormat, providerType) {
			if rawBody != nil {
				passthroughBytes, marshalErr := json.Marshal(rawBody)
				if marshalErr == nil {
					req.Extra = map[string]any{"_passthrough_body": passthroughBytes}
				}
			}
		}
	}

	needsReasoning := false
	if len(plan) > 0 {
		providerType := plan[0].Channel.ProviderType
		needsReasoning = protocol.NeedsReasoningContent(providerType, req.Model)
		if needsReasoning {
			req.Messages = protocol.InjectReasoningPlaceholder(req.Messages)
		}
	}

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

	// P8: Grant routing — if user has an active grant, route through enterprise relay
	grantRelayURL := s.configSvc.GetSelectedGrantRelayURL()
	grantID := s.configSvc.GetSelectedGrantID()
	if grantRelayURL != "" && grantID != "" {
		resp, err := s.forwardViaGrant(r.Context(), grantRelayURL, grantID, bodyBytes)
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

	resp, usedStep, attempts, forwardErr := s.forwardRequest(r.Context(), plan, req, sessionKey)
	routeSteps = attempts
	routeTrace = summarizeAttemptTrace(attempts)
	if usedStep != nil {
		selectedChannelID = usedStep.Channel.ChannelID
		selectedChannelName = usedStep.Channel.DisplayName
		selectedUpstreamModel = usedStep.UpstreamModel
	}
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
	injectDebug := debugTrace && !req.Stream
	if injectDebug {
		w.Header().Del("Content-Length") // 注入后长度变化，交给 Go 自动分块
	}
	w.WriteHeader(resp.StatusCode)

	if req.Stream {
		if needsReasoning {
			_ = protocol.ProcessReasoningSSEStream(resp.Body, w)
			return
		}
		// 响应方向原样透传：模型输出（最终回答/工具入参）不做 RTK 改写
		_, _ = io.Copy(w, resp.Body)
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

func (s *Service) validateToken(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if len(auth) <= 7 || !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return s.tokenMatches(strings.TrimSpace(auth[7:]))
}

// tokenMatches 以常量时间比较访问令牌，供 HTTP 与 WebSocket 入口共用。
func (s *Service) tokenMatches(token string) bool {
	if token == "" {
		return false
	}
	s.mu.RLock()
	currentToken := s.accessToken
	s.mu.RUnlock()
	return subtle.ConstantTimeCompare([]byte(token), []byte(currentToken)) == 1
}

// ServeGatewayRequest 在进程内复用 HTTP 主链路处理一次请求，返回（状态码, 响应体）。
// WebSocket 桥接通过它复用与 HTTP 完全一致的鉴权 / 限流 / DLP / 路由 / 转发 / 记账逻辑，
// 避免 WebSocket 成为绕过治理的旁路。
func (s *Service) ServeGatewayRequest(path string, body []byte, token string) (int, []byte) {
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return http.StatusBadRequest, []byte(`{"error":{"message":"invalid ws request"}}`)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	rec := &bodyRecorder{header: make(http.Header)}
	switch path {
	case "/v1/chat/completions":
		s.handleChatCompletions(rec, req)
	case "/v1/models":
		s.handleListModels(rec, req)
	default:
		return http.StatusNotFound, []byte(`{"error":{"message":"unsupported websocket message type"}}`)
	}
	return rec.snapshot()
}

// bodyRecorder 把 handler 的响应收集到内存，供进程内复用 HTTP 链路时取回结果。
type bodyRecorder struct {
	header http.Header
	status int
	buf    bytes.Buffer
}

func (r *bodyRecorder) Header() http.Header { return r.header }

func (r *bodyRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *bodyRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.buf.Write(p)
}

// Flush 让 bodyRecorder 满足 http.Flusher（流式 handler 会调用），此处仅累加不实时下发。
func (r *bodyRecorder) Flush() {}

func (r *bodyRecorder) snapshot() (int, []byte) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.status, r.buf.Bytes()
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

		if len(step.Channel.APIKeys) > 1 {
			rotator := s.getKeyRotator(step.Channel.ChannelID, step.Channel.APIKeys)
			selectedKey := rotator.Next()
			if selectedKey != "" {
				if optCfg.CooldownEnabled && s.cooldownMgr.IsOnCooldown(selectedKey) {
					attempt.Status = "cooldown"
					attempt.Error = fmt.Sprintf("key %s... is on cooldown", safeKeyPrefix(selectedKey))
					attempts = append(attempts, attempt)
					continue
				}
				providerCfg.APIKey = selectedKey
			}
		}

		// Per-channel concurrency limit (P2-23)
		if !s.concurrencyLimiter.Acquire(step.Channel.ChannelID) {
			attempt.Status = "concurrency_limit"
			attempt.Error = "channel at max concurrent requests"
			attempts = append(attempts, attempt)
			lastErr = fmt.Errorf("channel %s at concurrency limit", step.Channel.ChannelID)
			continue
		}

		executor := providers.ResolveExecutor(providerCfg)
		attemptStart := time.Now()
		upstreamResp, err := executor.ChatCompletions(ctx, providerCfg, &providers.UpstreamRequest{
			Model:    step.UpstreamModel,
			Messages: req.Messages,
			Stream:   req.Stream,
			Extra:    req.Extra,
		})
		s.concurrencyLimiter.Release(step.Channel.ChannelID)

		if err == nil {
			cb.RecordSuccess()
			routing.RecordLatency(step.Channel.ChannelID, float64(time.Since(attemptStart).Milliseconds()))
			s.recordPenaltySuccess(step)
			s.resolver.RecordSessionStep(sessionKey, step)
			attempt.Status = "success"
			attempts = append(attempts, attempt)
			return &http.Response{
				StatusCode: upstreamResp.Status,
				Header:     upstreamResp.Headers,
				Body:       upstreamResp.Body,
			}, &step, attempts, nil
		}

		cb.RecordFailure()
		attempt.Error = err.Error()
		lastErr = err

		if len(step.Channel.APIKeys) > 1 && providerCfg.APIKey != "" {
			providerErr, ok := err.(*providers.ProviderError)
			if ok && providers.ShouldRotateKey(providerErr.StatusCode) {
				rotator := s.getKeyRotator(step.Channel.ChannelID, step.Channel.APIKeys)
				rotator.MarkKeyFailed(providerCfg.APIKey)
			}
		}

		providerErr, ok := err.(*providers.ProviderError)
		if ok {
			_, retryable := classifier.Classify(providerErr.StatusCode, providerErr.Message)
			if retryable {
				attempt.Status = "retryable_failure"
				if providerErr.StatusCode == 429 {
					if optCfg.PenaltyEnabled {
						s.recordPenaltyRateLimit(step)
					}
					if optCfg.CooldownEnabled && providerCfg.APIKey != "" {
						s.cooldownMgr.SetCooldownWithReason(providerCfg.APIKey, "rate_limited")
					}
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

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"code":    status,
		},
	})
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
		token, err := s.oauthRefresher.EnsureValidToken(
			channel.ChannelID,
			channel.OAuthRefreshToken,
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

func safeKeyPrefix(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:8]
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

// recordComboMetrics records routing metrics for a combo request
func (s *Service) recordComboMetrics(comboName string, numAttempts int, success bool) {
	if comboName == "" {
		return
	}
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()

	metrics, ok := s.comboMetrics[comboName]
	if !ok {
		metrics = &ComboRouteMetrics{
			ComboName: comboName,
		}
		s.comboMetrics[comboName] = metrics
	}

	metrics.TotalRequests++
	if numAttempts > 1 {
		metrics.TotalFallbacks++
	}
	if numAttempts == 1 && success {
		metrics.Step1Success++
	} else if success && numAttempts >= 2 {
		metrics.LastStepFallback++
	}

	// Update derived rates
	if metrics.TotalRequests > 0 {
		metrics.Step1HitRate = float64(metrics.Step1Success) / float64(metrics.TotalRequests)
		metrics.LastStepHitRate = float64(metrics.LastStepFallback) / float64(metrics.TotalRequests)
	}
	metrics.AvgAttempts = (metrics.AvgAttempts*float64(metrics.TotalRequests-1) + float64(numAttempts)) / float64(metrics.TotalRequests)
}

// GetComboRouteMetrics returns aggregated route metrics for all tracked combos
func (s *Service) GetComboRouteMetrics() []ComboRouteMetrics {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()

	result := make([]ComboRouteMetrics, 0, len(s.comboMetrics))
	for _, m := range s.comboMetrics {
		result = append(result, *m)
	}
	return result
}

// ResetComboRouteMetrics resets all combo route metrics
func (s *Service) ResetComboRouteMetrics() {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	s.comboMetrics = make(map[string]*ComboRouteMetrics)
}

// SyncChannelConcurrencyLimits updates the concurrency limiter with the latest
// channel MaxConcurrent settings. Call this after channel configuration changes.
func (s *Service) SyncChannelConcurrencyLimits() {
	channels, _ := s.configSvc.ListChannels()
	for _, ch := range channels {
		s.concurrencyLimiter.SetLimit(ch.ChannelID, ch.MaxConcurrent)
	}
}

// GetConcurrencyInflight returns the current in-flight count for a channel.
func (s *Service) GetConcurrencyInflight(channelID string) int {
	return s.concurrencyLimiter.Inflight(channelID)
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

// forwardViaGrant routes a chat completions request through the enterprise relay gateway
// using the user's active Token Market grant. The relay gateway resolves the grant's
// escrow API key and forwards the request to the upstream provider.
func (s *Service) forwardViaGrant(ctx context.Context, relayURL, grantID string, bodyBytes []byte) (*http.Response, error) {
	if s.grantHTTPClient == nil {
		s.grantHTTPClient = &http.Client{Timeout: 120 * time.Second}
	}

	url := strings.TrimRight(relayURL, "/") + "/relay/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.authSvc.GetPlatformToken())
	req.Header.Set("X-Grant-Id", grantID)

	resp, err := s.grantHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("grant relay returned %d: %s", resp.StatusCode, string(body))
	}
	return resp, nil
}

// proxyResponse copies an upstream HTTP response to the client ResponseWriter.
func (s *Service) proxyResponse(w http.ResponseWriter, resp *http.Response) {
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
