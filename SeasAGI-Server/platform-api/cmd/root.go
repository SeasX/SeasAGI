package cmd

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/admin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/auth"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/backup"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/channel"
	combo "github.com/SeasAGI/SeasAGI-Server/platform-api/internal/combo"
	combotemplate "github.com/SeasAGI/SeasAGI-Server/platform-api/internal/combotemplate"
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

	if err := modelcatalog.LoadAndSyncCatalog(); err != nil {
		logging.Warningf("model catalog sync failed: %v", err)
	} else {
		logging.Info("Model catalog synced")
	}

	r := gin.New()
	r.Use(corsMiddleware(), traceMiddleware(), gin.Recovery(), gin.Logger())

	if os.Getenv("GIN_MODE") != "release" {
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
		}

		api.GET("/version/check", version.Check)
		api.GET("/version/migrations", version.Migrations)

		authenticated := api.Group("")
		authenticated.Use(auth.Middleware())
		{
			authenticated.GET("/user/profile", user.GetProfile)
			authenticated.PUT("/user/profile", user.UpdateProfile)
			authenticated.GET("/user/optimization", user.GetOptimizationConfig)
			authenticated.PUT("/user/optimization", user.SetOptimizationConfig)

			authenticated.GET("/channels", channel.ListChannels)
			authenticated.GET("/channels/:id", channel.GetChannel)
			authenticated.GET("/free-channels", channel.ListFreeChannels)
			authenticated.POST("/channels", channel.CreateChannel)
			authenticated.PUT("/channels/:id", channel.UpdateChannel)
			authenticated.DELETE("/channels/:id", channel.DeleteChannel)

			authenticated.GET("/channels/:id/resources", providerresource.ListResources)
			authenticated.GET("/channels/:id/resources/:resource_id", providerresource.GetResource)
			authenticated.POST("/channels/:id/resources", providerresource.CreateResource)
			authenticated.PUT("/channels/:id/resources/:resource_id", providerresource.UpdateResource)
			authenticated.DELETE("/channels/:id/resources/:resource_id", providerresource.DeleteResource)
			authenticated.GET("/channels/:id/resources/resolve", providerresource.ResolveResource)

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
		adminGroup := api.Group("/admin")
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

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "9318"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopCh
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	logging.Infof("Platform API starting on :%s", port)
	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
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
	// WARNING: This middleware logs request and response bodies including sensitive data (passwords, API keys, tokens).
	// It MUST only be enabled in development/debug environments. NEVER enable in production.
	return func(c *gin.Context) {
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		logging.Debugf(">>> %s %s", c.Request.Method, c.Request.URL.String())
		if len(bodyBytes) > 0 {
			bodyStr := string(bodyBytes)
			if len(bodyStr) > 2048 {
				bodyStr = bodyStr[:2048] + "... (truncated)"
			}
			logging.Debugf(">>> Request Body: %s", bodyStr)
		}

		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw
		c.Next()

		respBody := blw.body.String()
		if len(respBody) > 2048 {
			respBody = respBody[:2048] + "... (truncated)"
		}
		logging.Debugf("<<< %d %s", c.Writer.Status(), respBody)
	}
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := os.Getenv("CORS_ALLOW_ORIGIN")
		if allowed == "" {
			allowed = "*"
		}
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		} else {
			c.Header("Access-Control-Allow-Origin", allowed)
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-Id")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
