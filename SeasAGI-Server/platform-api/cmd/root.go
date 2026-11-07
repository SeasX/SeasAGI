package cmd

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/admin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/auth"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/backup"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/channel"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/combo"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/combotemplate"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/device"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/logging"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/modelcatalog"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/providerresource"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/relay"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/secpolicy"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/usage"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/user"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/version"
	"github.com/gin-gonic/gin"
)

//go:embed all:admin_dist
var adminAssets embed.FS

func Execute() error {
	secpolicy.ValidateStartup()

	dbPath := os.Getenv("DB_PATH")
	if err := database.Init(dbPath); err != nil {
		return fmt.Errorf("database init failed: %w", err)
	}
	defer database.Close()
	logging.Info("Database initialized")

	// 优雅退出上下文：trace 保留清理与 HTTP 服务共用，进程退出时一并停止。
	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()
	database.StartTraceRetention(appCtx, database.TraceRetentionConfigFromEnv())

	if err := modelcatalog.LoadAndSyncCatalog(); err != nil {
		logging.Warningf("model catalog sync failed: %v", err)
	} else {
		logging.Info("Model catalog synced")
	}

	r, err := BuildRouter()
	if err != nil {
		return err
	}

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "9318"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
		// 避免 slowloris 与连接长期占用；不设置覆盖流式响应的整体超时。
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	// 关停宽限期，默认 20s，可通过 SHUTDOWN_TIMEOUT_SEC 调整。
	shutdownTimeout := 20 * time.Second
	if v := os.Getenv("SHUTDOWN_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			shutdownTimeout = time.Duration(n) * time.Second
		}
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopCh
		stopApp()
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logging.Warningf("graceful shutdown failed, forcing close: %v", err)
			_ = server.Close()
		}
	}()

	logging.Infof("Platform API starting on :%s", port)
	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// BuildRouter 构建 HTTP 路由（不监听端口、不启动后台任务），
// 供进程启动与路由安全/攻击测试复用。
func BuildRouter() (*gin.Engine, error) {
	r := gin.New()
	r.Use(bodyLimitMiddleware(requestBodyLimit()), corsMiddleware(), traceMiddleware(), gin.Recovery(), gin.Logger())

	// 仅在显式开启 SEASAGI_DEBUG_BODY=1 时记录请求/响应体，且敏感字段会被打码。
	if os.Getenv("SEASAGI_DEBUG_BODY") == "1" {
		logging.Warningf("SEASAGI_DEBUG_BODY is enabled — request/response bodies are logged with sensitive fields masked")
		r.Use(debugLogMiddleware())
	}

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	registerAdminUI(r)

	api := r.Group("/api/v1")
	{
		authGroup := api.Group("/auth")
		{
			authGroup.POST("/login", auth.Login)
			authGroup.POST("/register", auth.Register)
			authGroup.POST("/refresh", auth.RefreshToken)
			authGroup.GET("/oauth/providers", auth.GetOAuthProviders)
			authGroup.POST("/oauth/authorize", auth.AuthorizeOAuth)
			authGroup.GET("/oauth/start", auth.StartOAuth)
			authGroup.GET("/oauth/callback", auth.OAuthCallback)
			authGroup.POST("/oauth/poll", auth.PollOAuth)
		}

		api.GET("/version/check", version.Check)
		api.GET("/version/migrations", version.Migrations)

		// 内部服务用量上报入口：必须携带 INTERNAL_API_TOKEN（或 ADMIN_SECRET），
		// 未配置令牌时直接 503，避免 usage_records 没有写入源。
		internalGroup := api.Group("/internal", usage.InternalAuth())
		{
			internalGroup.POST("/usage", usage.IngestUsage)
		}

		authenticated := api.Group("")
		authenticated.Use(auth.Middleware())
		{
			authenticated.GET("/user/profile", user.GetProfile)
			authenticated.GET("/user/optimization", user.GetOptimizationConfig)
			authenticated.PUT("/user/optimization", user.SetOptimizationConfig)

			authenticated.GET("/channels", channel.ListChannels)
			authenticated.GET("/channels/:id", channel.GetChannel)
			authenticated.GET("/free-channels", channel.ListFreeChannels)

			authenticated.GET("/models", modelcatalog.ListModels)
			authenticated.GET("/models/:name", modelcatalog.GetModel)

			authenticated.POST("/devices/bind", device.BindDevice)
			authenticated.GET("/devices", device.ListDevices)
			authenticated.DELETE("/devices/:id", device.RemoveDevice)

			authenticated.GET("/combo-templates", combotemplate.ListOfficialTemplates)

			authenticated.GET("/combos", combo.ListUserCombos)
			authenticated.GET("/combos/:id", combo.GetUserCombo)
			authenticated.POST("/combos", combo.CreateUserCombo)
			authenticated.PUT("/combos/:id", combo.UpdateUserCombo)
			authenticated.DELETE("/combos/:id", combo.DeleteUserCombo)

			authenticated.GET("/relay-gateways", relay.ListRelayGateways)

			authenticated.GET("/usage", usage.GetUserUsage)
			authenticated.GET("/usage/models", usage.GetUserModelUsage)
			authenticated.GET("/usage/error-distribution", usage.GetErrorDistribution)
			authenticated.GET("/usage/timeline", usage.GetUsageTimeline)
		}

		// Admin routes (community edition: user/channel/combo/relay basic management)
		// 全部管理端点必须携带 ADMIN_SECRET，禁止匿名访问。
		adminGroup := api.Group("/admin", admin.Middleware())
		{
			adminGroup.GET("/users", admin.ListUsers)
			adminGroup.GET("/users/:id", admin.GetUser)
			adminGroup.DELETE("/users/:id", admin.DeleteUser)

			adminGroup.POST("/channels/seed", admin.SeedChannel)
			adminGroup.POST("/channels/:id/models", admin.SeedChannelModels)
			adminGroup.POST("/models/sync", modelcatalog.SyncCatalog)
			adminGroup.GET("/channels", channel.ListChannels)
			adminGroup.POST("/channels", channel.CreateChannel)
			adminGroup.PUT("/channels/:id", channel.UpdateChannel)
			adminGroup.DELETE("/channels/:id", channel.DeleteChannel)

			// 通道资源（含明文解密端点）只允许管理员访问。
			adminGroup.GET("/channels/:id/resources", providerresource.ListResources)
			adminGroup.GET("/channels/:id/resources/:resource_id", providerresource.GetResource)
			adminGroup.POST("/channels/:id/resources", providerresource.CreateResource)
			adminGroup.PUT("/channels/:id/resources/:resource_id", providerresource.UpdateResource)
			adminGroup.DELETE("/channels/:id/resources/:resource_id", providerresource.DeleteResource)
			adminGroup.GET("/channels/:id/resources/resolve", providerresource.ResolveResource)

			adminGroup.GET("/users/:id/custom-channels", admin.ListUserCustomChannels)

			adminGroup.GET("/combos", combo.ListUserCombos)
			adminGroup.POST("/combos", combo.CreateUserCombo)
			adminGroup.PUT("/combos/:id", combo.UpdateUserCombo)
			adminGroup.DELETE("/combos/:id", combo.DeleteUserCombo)

			adminGroup.GET("/usage/stats", admin.GetUsageStats)
			adminGroup.GET("/usage/records", admin.ListAllUsage)

			adminGroup.GET("/relay-gateways", relay.AdminListRelayGateways)
			adminGroup.POST("/relay-gateways", relay.AdminCreateRelayGateway)
			adminGroup.PUT("/relay-gateways/:id", relay.AdminUpdateRelayGateway)
			adminGroup.DELETE("/relay-gateways/:id", relay.AdminDeleteRelayGateway)

			adminGroup.GET("/sqlite/backups", backup.ListBackups)
			adminGroup.POST("/sqlite/backups", backup.CreateBackup)
			adminGroup.GET("/sqlite/backups/:id", backup.GetBackup)
			adminGroup.DELETE("/sqlite/backups/:id", backup.DeleteBackup)
			adminGroup.POST("/sqlite/backups/:id/restore", backup.RestoreBackup)
			adminGroup.GET("/sqlite/backups/:id/verify", backup.VerifyBackup)
		}
	}

	return r, nil
}

func registerAdminUI(r *gin.Engine) {
	sub, err := fs.Sub(adminAssets, "admin_dist")
	if err != nil {
		logging.Warningf("admin UI assets not found: %v", err)
		return
	}

	r.GET("/admin/*filepath", func(c *gin.Context) {
		filepath := c.Param("filepath")
		if filepath == "" {
			c.Redirect(http.StatusMovedPermanently, "/admin/")
			return
		}
		serveAdminFile(c, sub)
	})
}

func serveAdminFile(c *gin.Context, sub fs.FS) {
	filepath := c.Param("filepath")
	if filepath == "/" || filepath == "" {
		filepath = "/index.html"
	}

	relPath := strings.TrimPrefix(filepath, "/")

	f, err := sub.Open(relPath)
	if err != nil {
		serveAdminIndexHTML(c, sub)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		serveAdminIndexHTML(c, sub)
		return
	}

	if stat.IsDir() {
		f.Close()
		serveAdminIndexHTML(c, sub)
		return
	}

	data, err := io.ReadAll(f)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	ext := path.Ext(filepath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if ext == ".html" {
		contentType = "text/html; charset=utf-8"
	}
	if ext == ".js" {
		contentType = "application/javascript; charset=utf-8"
	}
	if ext == ".css" {
		contentType = "text/css; charset=utf-8"
	}
	if ext == ".json" {
		contentType = "application/json; charset=utf-8"
	}
	if ext == ".svg" {
		contentType = "image/svg+xml"
	}

	c.Data(http.StatusOK, contentType, data)
}

func serveAdminIndexHTML(c *gin.Context, sub fs.FS) {
	f, err := sub.Open("index.html")
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func traceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
		}
		c.Set("trace_id", traceID)
		c.Writer.Header().Set("X-Trace-Id", traceID)
		c.Next()
	}
}

func debugLogMiddleware() gin.HandlerFunc {
	// 该中间件仅在显式开启 SEASAGI_DEBUG_BODY=1 时启用；
	// 所有请求/响应体都会先做敏感字段打码，非 JSON 正文不落盘。
	return func(c *gin.Context) {
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		logging.Debugf(">>> %s %s", c.Request.Method, c.Request.URL.String())
		if len(bodyBytes) > 0 {
			logging.Debugf(">>> Request Body: %s", maskSensitiveBody(bodyBytes))
		}

		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw
		c.Next()

		logging.Debugf("<<< %d %s", c.Writer.Status(), maskSensitiveBody(blw.body.Bytes()))
	}
}

// debugBodyLimit 限制 debug 日志中正文的最大长度。
const debugBodyLimit = 2048

// sensitiveBodyKeys 明确需要打码的字段名。
var sensitiveBodyKeys = map[string]struct{}{
	"password":           {},
	"passwd":             {},
	"api_key":            {},
	"apikey":             {},
	"authorization":      {},
	"token":              {},
	"access_token":       {},
	"refresh_token":      {},
	"id_token":           {},
	"secret":             {},
	"client_secret":      {},
	"encrypted_api_key":  {},
	"admin_secret":       {},
	"internal_api_token": {},
}

// isSensitiveKey 判断字段是否需要打码（兼容 *_token / *_secret / *_password / *_api_key）。
func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	if _, ok := sensitiveBodyKeys[k]; ok {
		return true
	}
	for _, suffix := range []string{"_token", "_secret", "_password", "_api_key"} {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// maskSensitiveBody 对 JSON 正文递归打码；非 JSON 正文直接省略，避免明文密钥入库日志。
func maskSensitiveBody(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	var v interface{}
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return "[non-JSON body omitted]"
	}
	masked, err := json.Marshal(maskSensitiveValue(v))
	if err != nil {
		return "[unparsable body omitted]"
	}
	if len(masked) > debugBodyLimit {
		return string(masked[:debugBodyLimit]) + "... (truncated)"
	}
	return string(masked)
}

func maskSensitiveValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if isSensitiveKey(k) {
				t[k] = "***"
				continue
			}
			t[k] = maskSensitiveValue(val)
		}
	case []interface{}:
		for i := range t {
			t[i] = maskSensitiveValue(t[i])
		}
	}
	return v
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// corsPolicy 保存 CORS 白名单匹配规则。
// origins 为显式来源白名单；未配置时 allowLoopback 为 true，仅放行本机来源。
type corsPolicy struct {
	origins       map[string]bool
	allowLoopback bool
}

// buildCORSPolicy 解析 CORS_ALLOWED_ORIGINS（逗号分隔，兼容旧变量 CORS_ALLOW_ORIGIN）。
// 规则：
//   - 仅做精确匹配，通配符 "*" 永远不构成白名单条目；
//   - 生产模式下出现 "*" 直接返回错误（拒绝启动），避免退化成反射任意 Origin；
//   - 未配置任何来源时默认只允许本机来源（localhost / 127.0.0.1 / [::1]）。
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

// corsMiddleware 依据 CORS 白名单下发跨域响应头。
// 只有命中白名单才回显 Origin 并允许凭证，不会出现 * + Credentials 的组合。
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
			c.Header("Access-Control-Allow-Origin", strings.TrimSuffix(strings.TrimSpace(origin), "/"))
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-Id")
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// defaultMaxRequestBodyBytes 是 platform-api 单请求正文上限（8MB）。
const defaultMaxRequestBodyBytes int64 = 8 << 20

// requestBodyLimit 读取 MAX_REQUEST_BODY_BYTES，未配置时使用默认值。
func requestBodyLimit() int64 {
	if v := strings.TrimSpace(os.Getenv("MAX_REQUEST_BODY_BYTES")); v != "" {
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
