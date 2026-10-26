package gateway

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
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

// 请求体上限，避免客户端用超大请求打爆内存（CL-13）。
const (
	// maxJSONRequestBody JSON/文本请求体上限（32MB）。
	maxJSONRequestBody int64 = 32 << 20
	// maxMultipartRequestBody multipart 请求体上限；音频/文件上传单独放宽到 64MB。
	maxMultipartRequestBody int64 = 64 << 20
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
	grantHTTPClient     *http.Client
	grantHTTPClientOnce sync.Once
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
			// 健康检查必须使用真实凭证：ListChannelsWithCredentials 会从内存/keychain
			// 解析 Key，避免脱敏空 Key 导致正常渠道被误判 unhealthy（CL-05）。
			channels, err := configSvc.ListChannelsWithCredentials()
			if err != nil {
				logging.Warningf("health check: load channel credentials failed: %v", err)
				return nil
			}
			return healthChannelInfos(channels)
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
		// Grant 中继 HTTP 客户端在构造期初始化，避免首批并发请求懒初始化产生数据竞争（CL-09）。
		grantHTTPClient: &http.Client{Timeout: 300 * time.Second},
	}

	// Start auto-save if state store is available
	if stateStore != nil {
		svc.stopAutoSave = stateStore.StartAutoSave(penaltyMgr, cooldownMgr, 30*time.Second)
	}

	// Initialize OAuth token refresher (P2-21)
	svc.oauthRefresher = providers.NewOAuthTokenRefresher(60 * time.Second)
	// 提供方轮换 refresh token 后回写 keychain，重启后仍使用最新令牌（CL-11）。
	svc.oauthRefresher.SetRefreshTokenPersist(func(channelID, refreshToken string) error {
		return keychain.SaveSecret("oauth_refresh_"+channelID, refreshToken)
	})

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

// healthChannelInfos 把渠道转换为健康检查输入。没有可校验凭证的渠道
// （OAuth-only、平台托管、以及 keychain 读取失败的渠道）直接跳过：健康检查
// 只能探测 API Key 型渠道，跳过可避免每 5 分钟把正常渠道误写成 unhealthy。
func healthChannelInfos(channels []config.Channel) []providers.HealthChannelInfo {
	infos := make([]providers.HealthChannelInfo, 0, len(channels))
	for _, ch := range channels {
		if ch.ChannelType == "platform" {
			continue
		}
		apiKey := strings.TrimSpace(ch.APIKey)
		if apiKey == "" {
			for _, key := range ch.APIKeys {
				if strings.TrimSpace(key) != "" {
					apiKey = strings.TrimSpace(key)
					break
				}
			}
		}
		// OAuth 会话类渠道没有 API Key，ListModels 无法携带刷新令牌，跳过而不是误判。
		if apiKey == "" {
			continue
		}
		infos = append(infos, providers.HealthChannelInfo{
			ChannelID:    ch.ChannelID,
			ChannelType:  ch.ChannelType,
			ProviderType: ch.ProviderType,
			BaseURL:      ch.BaseURL,
			APIKey:       apiKey,
			Enabled:      ch.Enabled,
			HealthStatus: ch.HealthStatus,
		})
	}
	return infos
}

// grantClient 返回 Grant 中继 HTTP 客户端（构造期已初始化，once 兜底测试字面量）。
func (s *Service) grantClient() *http.Client {
	s.grantHTTPClientOnce.Do(func() {
		if s.grantHTTPClient == nil {
			s.grantHTTPClient = &http.Client{Timeout: 300 * time.Second}
		}
	})
	return s.grantHTTPClient
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
	mux.HandleFunc("/v1/messages", s.handleAnthropicMessages)
	mux.HandleFunc("/v1/responses", s.handleResponses)
	mux.HandleFunc("/v1beta/", s.handleGemini)
	mux.HandleFunc("/v1/embeddings", s.handleEmbeddings)
	mux.HandleFunc("/v1/images/generations", s.handleImageGenerations)
	mux.HandleFunc("/v1/audio/speech", s.handleTTS)
	mux.HandleFunc("/v1/audio/transcriptions", s.handleSTT)

	// WebSocket bridge（与 HTTP 主链路共用治理逻辑）
	if s.wsBridge == nil {
		s.wsBridge = NewWSBridge(s)
	}
	mux.HandleFunc("/v1/ws", s.wsBridge.HandleWS)

	// A 端节点入口（P2-1）：仅接受企业 relay 的签名请求，不暴露本地 /v1/* 匿名访问。
	mux.HandleFunc("/node/", s.NodeProxy)
	mux.HandleFunc("/node", s.NodeProxy)

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

// releaseOnCloseBody 在响应体关闭（流结束）时归还渠道并发槽位。流式请求的 Body
// 会一直存活到客户端读完，槽位也随之一直占用，使 MaxConcurrent 对流式真正生效（CL-15）。
type releaseOnCloseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *releaseOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
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
		attempt.Error = errMsg
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

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// readLimitedBody 读取带大小上限的请求体；超限返回 413 且 ok=false（CL-13）。
func readLimitedBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "Request body too large")
			return nil, false
		}
		writeJSONError(w, http.StatusBadRequest, "Failed to read request body")
		return nil, false
	}
	return body, true
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
