package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/channel"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/health"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/middleware"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/ops"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/policy"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/ratelimit"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/relay"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/secpolicy"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/trace"
	"github.com/gin-gonic/gin"
)

var (
	routerStore   *channel.Store
	routerLimiter *ratelimit.Limiter
	routerRateRPM float64
)

func Execute() error {
	secpolicy.ValidateStartup()

	r, err := BuildRouter()
	if err != nil {
		return err
	}
	store := routerStore
	defer store.Close()

	if err := store.LoadModelCatalog(); err != nil {
		logging.Warningf("model catalog load failed: %v", err)
	} else {
		logging.Info("Model catalog loaded")
	}

	trace.InitStore(store.DBPath())
	policy.InitCache()

	// 优雅退出上下文：trace 保留清理与 HTTP 服务共用，进程退出时一并停止。
	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()
	trace.StartRetention(appCtx, trace.GetStore(), trace.RetentionConfigFromEnv())

	go routerLimiter.CleanupLoop(time.Minute)

	checkInterval, _ := strconv.Atoi(os.Getenv("HEALTH_CHECK_INTERVAL_SEC"))
	if checkInterval <= 0 {
		checkInterval = 60
	}
	checker := health.NewChecker(store, time.Duration(checkInterval)*time.Second)
	checker.Start()
	defer checker.Stop()

	port := os.Getenv("RELAY_PORT")
	if port == "" {
		port = "8318"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopCh
		stopApp()
		_ = server.Shutdown(context.Background())
	}()

	logging.Infof("Relay gateway starting on :%s (rate limit: %.0f rpm, health check: %ds)", port, routerRateRPM, checkInterval)
	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// BuildRouter 构建 relay 数据面路由（不监听端口、不启动健康检查/后台循环），
// 供进程启动与路由安全/攻击测试复用。
func BuildRouter() (*gin.Engine, error) {
	store := channel.NewStore()
	routerStore = store
	relay.SetChannelStore(store)
	middleware.SetChannelStatsProvider(func() []middleware.ChannelStat {
		stats := make([]middleware.ChannelStat, 0)
		for _, ch := range store.ListChannels() {
			stats = append(stats, middleware.ChannelStat{
				ChannelID:    ch.ChannelID,
				ProviderType: ch.ProviderType,
				Enabled:      ch.Enabled,
				Healthy:      ch.Health.Healthy,
			})
		}
		return stats
	})

	ratePerMin, _ := strconv.ParseFloat(os.Getenv("RATE_LIMIT_RPM"), 64)
	if ratePerMin <= 0 {
		ratePerMin = 60
	}
	routerRateRPM = ratePerMin
	limiter := ratelimit.NewLimiter(ratePerMin)
	routerLimiter = limiter

	r := gin.New()
	r.Use(bodyLimitMiddleware(requestBodyLimit()), corsMiddleware(), middleware.Logger(), middleware.Recovery(), middleware.Metrics())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", middleware.MetricsHandler)
	// 运维面统一鉴权：/ops 全部端点必须携带 ADMIN_SECRET，且输出不含上游凭证。
	opsGroup := r.Group("/ops", middleware.AdminAuth())
	{
		opsGroup.GET("/channels", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"data":  channel.PublicList(store.ListChannels()),
				"stats": store.GetStats(),
			})
		})
		opsGroup.GET("/overview", ops.GetOverview)
		opsGroup.GET("/channels/health", ops.ListChannelHealth)
		opsGroup.GET("/traces/:trace_id", ops.GetTraceByID)
		opsGroup.GET("/traces", ops.QueryTraces)
		opsGroup.POST("/channels/:channel_id/drain", ops.DrainChannel)
		opsGroup.POST("/channels/:channel_id/restore", ops.RestoreChannel)
		opsGroup.PUT("/channels/:channel_id/read-only", ops.SetChannelReadOnly)
		opsGroup.PUT("/channels/:channel_id/weight", ops.SetChannelWeight)
		opsGroup.PUT("/channels/:channel_id/gray", ops.SetChannelGray)
		opsGroup.GET("/alerts", ops.GetAlertPanel)
	}

	relayGroup := r.Group("/relay")
	relayGroup.Use(middleware.AuthRequired())
	relayGroup.Use(limiter.Middleware())
	{
		relayGroup.POST("/chat/completions", relay.ChatCompletions)
		relayGroup.POST("/embeddings", relay.Embeddings)
		relayGroup.POST("/images/generations", relay.CreateImage)
		relayGroup.POST("/audio/speech", relay.CreateSpeech)
		relayGroup.POST("/audio/transcriptions", relay.CreateTranscription)
		relayGroup.GET("/models", relay.ListModels)
	}

	return r, nil
}

// corsPolicy 保存 CORS 白名单匹配规则：显式来源列表 + 未配置时的本机来源回退。
type corsPolicy struct {
	origins       map[string]bool
	allowLoopback bool
}

// buildCORSPolicy 解析 CORS_ALLOWED_ORIGINS（逗号分隔，兼容旧变量 CORS_ALLOW_ORIGIN）。
// 与 platform-api 保持一致：仅精确匹配；生产模式拒绝 "*"；
// 未配置时默认只允许本机来源（localhost / 127.0.0.1 / [::1]）。
func buildCORSPolicy(raw string, production bool) (corsPolicy, error) {
	policy := corsPolicy{origins: make(map[string]bool)}
	wildcard := false
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSuffix(strings.TrimSpace(part), "/")
		if origin == "" {
			continue
		}
		if origin == "*" {
			wildcard = true
			continue
		}
		policy.origins[origin] = true
	}
	if wildcard {
		if production {
			return policy, fmt.Errorf("CORS_ALLOWED_ORIGINS must not contain '*': list explicit origins instead")
		}
		logging.Warningf("CORS wildcard '*' is ignored; configure explicit origins via CORS_ALLOWED_ORIGINS")
	}
	if len(policy.origins) == 0 {
		policy.allowLoopback = true
	}
	return policy, nil
}

// allows 判断给定 Origin 是否命中白名单。
func (p corsPolicy) allows(origin string) bool {
	origin = strings.TrimSuffix(strings.TrimSpace(origin), "/")
	if origin == "" {
		return false
	}
	if p.origins[origin] {
		return true
	}
	return p.allowLoopback && isLoopbackOrigin(origin)
}

// isLoopbackOrigin 判断 Origin 是否指向本机（任意端口）。
func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isProductionMode 与 secpolicy 保持一致：未显式开启开发模式即视为生产。
func isProductionMode() bool {
	if os.Getenv("SEASAGI_ALLOW_DEV_SECRETS") == "1" {
		return false
	}
	return os.Getenv("GIN_MODE") != "debug"
}

func corsMiddleware() gin.HandlerFunc {
	raw := os.Getenv("CORS_ALLOWED_ORIGINS")
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv("CORS_ALLOW_ORIGIN")
	}
	policy, err := buildCORSPolicy(raw, isProductionMode())
	if err != nil {
		logging.Fatalf("invalid CORS configuration: %v", err)
	}
	if len(policy.origins) == 0 {
		logging.Warningf("CORS_ALLOWED_ORIGINS not set: only loopback origins (localhost/127.0.0.1/[::1]) are allowed")
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if policy.allows(origin) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", strings.TrimSuffix(strings.TrimSpace(origin), "/"))
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Trace-Id, X-Device-Id, X-Tenant-Id")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// defaultMaxRequestBodyBytes 是 relay-gateway 单请求正文兜底上限（64MB，覆盖 multipart）。
const defaultMaxRequestBodyBytes int64 = 64 << 20

// requestBodyLimit 读取 RELAY_MAX_REQUEST_BODY_BYTES，未配置时使用默认值。
// 与 platform-api 的 MAX_REQUEST_BODY_BYTES 分开命名，避免共用 .env 时把网关上限压到 8MB。
func requestBodyLimit() int64 {
	if v := strings.TrimSpace(os.Getenv("RELAY_MAX_REQUEST_BODY_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxRequestBodyBytes
}

// bodyLimitMiddleware 限制单请求正文大小，避免超大 body 打满内存。
func bodyLimitMiddleware(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
