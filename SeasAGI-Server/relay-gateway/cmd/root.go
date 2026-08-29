package cmd

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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

func Execute() error {
	secpolicy.ValidateStartup()

	store := channel.NewStore()
	defer store.Close()
	if err := store.LoadModelCatalog(); err != nil {
		logging.Warningf("model catalog load failed: %v", err)
	} else {
		logging.Info("Model catalog loaded")
	}
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

	trace.InitStore(store.DBPath())
	policy.InitCache()

	ratePerMin, _ := strconv.ParseFloat(os.Getenv("RATE_LIMIT_RPM"), 64)
	if ratePerMin <= 0 {
		ratePerMin = 60
	}
	limiter := ratelimit.NewLimiter(ratePerMin)
	go limiter.CleanupLoop(time.Minute)

	checkInterval, _ := strconv.Atoi(os.Getenv("HEALTH_CHECK_INTERVAL_SEC"))
	if checkInterval <= 0 {
		checkInterval = 60
	}
	checker := health.NewChecker(store, time.Duration(checkInterval)*time.Second)
	checker.Start()
	defer checker.Stop()

	r := gin.New()
	r.Use(corsMiddleware(), middleware.Logger(), middleware.Recovery(), middleware.Metrics())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", middleware.MetricsHandler)
	r.GET("/ops/channels", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": store.ListChannels(), "stats": store.GetStats()})
	})

	opsGroup := r.Group("/ops")
	{
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
		_ = server.Shutdown(context.Background())
	}()

	logging.Infof("Relay gateway starting on :%s (rate limit: %.0f rpm, health check: %ds)", port, ratePerMin, checkInterval)
	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func corsMiddleware() gin.HandlerFunc {
	allowedOrigin := os.Getenv("CORS_ALLOW_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Trace-Id, X-Device-Id, X-Tenant-Id")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
