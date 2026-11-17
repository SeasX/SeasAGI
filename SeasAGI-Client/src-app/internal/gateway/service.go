package gateway

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/auth"
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
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
	// Per-key monthly USD quota, persisted to disk so restarts keep accrual (P2-7).
	keyQuota *config.KeyQuota
	// HTTP client for grant relay routing
	grantHTTPClient     *http.Client
	grantHTTPClientOnce sync.Once
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
		keyQuota:        config.NewPersistentKeyQuota(config.DefaultQuotaPath()),
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
		Addr:              fmt.Sprintf("127.0.0.1:%d", s.listenPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
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

// KeyQuotaStatus 返回本地访问令牌的月度配额状态（limit, used, configured）。
func (s *Service) KeyQuotaStatus() (float64, float64, bool) {
	limit, used, ok := s.keyQuota.GetQuota(keyQuotaSubject)
	return limit, used, ok
}

// SetKeyQuota 设置本地访问令牌的月度美元配额；limitUSD<=0 视为不限但仍持久化。
func (s *Service) SetKeyQuota(limitUSD float64) {
	s.keyQuota.SetQuota(keyQuotaSubject, limitUSD)
}

// RemoveKeyQuota 删除本地访问令牌的配额限制。
func (s *Service) RemoveKeyQuota() {
	s.keyQuota.RemoveQuota(keyQuotaSubject)
}
