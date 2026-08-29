package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricsNamespace  = "seasagi"
	metricsSubsystem  = "relay"
	metricsLabelUnset = "none"
)

// gatewayDurationBuckets is sized for model API calls, which range from a fast cached
// completion to a multi-minute reasoning request. The client_golang defaults stop at
// 10s and would put almost every LLM request in +Inf.
var gatewayDurationBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60, 120, 300}

// ChannelStat is one channel's health snapshot for metrics exposition.
type ChannelStat struct {
	ChannelID    string
	ProviderType string
	Enabled      bool
	Healthy      bool
}

// ChannelStatsProvider returns channel stats for metrics exposition. It is registered
// by cmd/root.go to avoid a circular import between middleware and channel.
type ChannelStatsProvider func() []ChannelStat

// MetricsCollector holds Prometheus collectors for the relay gateway.
//
// It owns its registry rather than using the default one, so each process gets an
// isolated instance instead of racing on process-global state.
type MetricsCollector struct {
	registry        *prometheus.Registry
	requests        *prometheus.CounterVec
	duration        *prometheus.HistogramVec
	inFlight        prometheus.Gauge
	channelHealth   *prometheus.GaugeVec
	channelsTotal   prometheus.Gauge
	channelsEnabled prometheus.Gauge
	channelsHealthy prometheus.Gauge
}

var globalCollector *MetricsCollector
var statsProvider ChannelStatsProvider

func init() {
	globalCollector = newMetricsCollector()
}

func newMetricsCollector() *MetricsCollector {
	m := &MetricsCollector{
		registry: prometheus.NewRegistry(),
	}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "requests_total",
		Help:      "Relay API requests by outcome. Counts logical requests, so a request that failed over across several candidates counts once.",
	}, []string{"channel_id", "model", "status_code", "outcome"})
	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "request_duration_seconds",
		Help:      "End-to-end relay request latency, including any failover attempts.",
		Buckets:   gatewayDurationBuckets,
	}, []string{"channel_id", "model", "outcome"})
	m.inFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "requests_in_flight",
		Help:      "Relay API requests currently being routed to an upstream.",
	})
	m.channelHealth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "channel_health",
		Help:      "Channel health status (1 = healthy, 0 = unhealthy).",
	}, []string{"channel_id", "provider_type"})
	m.channelsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "channels_total",
		Help:      "Total number of configured channels.",
	})
	m.channelsEnabled = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "channels_enabled",
		Help:      "Number of enabled channels.",
	})
	m.channelsHealthy = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "channels_healthy",
		Help:      "Number of healthy channels.",
	})

	m.registry.MustRegister(
		m.requests, m.duration, m.inFlight,
		m.channelHealth, m.channelsTotal, m.channelsEnabled, m.channelsHealthy,
	)
	// Process and Go runtime metrics are what an operator reaches for first when the
	// gateway itself is the suspect, and they cost nothing to collect.
	m.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

// SetChannelStatsProvider registers a provider for channel health metrics.
func SetChannelStatsProvider(p ChannelStatsProvider) {
	statsProvider = p
}

// Metrics is the Gin middleware that tracks request count, duration, and in-flight
// concurrency. The decrement is deferred so a handler that panics or writes a partial
// stream cannot leak the gauge upward for the lifetime of the process.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalCollector == nil {
			c.Next()
			return
		}
		globalCollector.inFlight.Inc()
		defer globalCollector.inFlight.Dec()

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		status := c.Writer.Status()
		channelID := c.GetString("channel_id")
		model := c.GetString("request_model")
		outcome := metricsOutcome(status)

		globalCollector.requests.WithLabelValues(
			metricsLabel(channelID), metricsLabel(model),
			strconv.Itoa(status), outcome,
		).Inc()
		globalCollector.duration.WithLabelValues(
			metricsLabel(channelID), metricsLabel(model), outcome,
		).Observe(duration.Seconds())
	}
}

// MetricsHandler serves the Prometheus exposition endpoint.
//
// Metrics disclose internal topology — channel identifiers, model names, traffic
// volume — so the endpoint always authenticates. It fails closed: with no usable token
// configured it reports 404 rather than degrading to anonymous access.
func MetricsHandler(c *gin.Context) {
	if globalCollector == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	token := strings.TrimSpace(os.Getenv("METRICS_TOKEN"))
	if token == "" {
		// Fall back to ADMIN_SECRET if available (enterprise), otherwise fail closed.
		token = strings.TrimSpace(os.Getenv("ADMIN_SECRET"))
	}
	if token == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if !metricsTokenMatches(token, c.GetHeader("Authorization")) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// Refresh channel health gauges on each scrape.
	if statsProvider != nil {
		stats := statsProvider()
		total := len(stats)
		enabled := 0
		healthy := 0
		for _, s := range stats {
			healthVal := 0.0
			if s.Healthy {
				healthVal = 1.0
			}
			globalCollector.channelHealth.WithLabelValues(
				metricsLabel(s.ChannelID), metricsLabel(s.ProviderType),
			).Set(healthVal)
			if s.Enabled {
				enabled++
			}
			if s.Healthy {
				healthy++
			}
		}
		globalCollector.channelsTotal.Set(float64(total))
		globalCollector.channelsEnabled.Set(float64(enabled))
		globalCollector.channelsHealthy.Set(float64(healthy))
	}

	promhttp.HandlerFor(globalCollector.registry, promhttp.HandlerOpts{}).ServeHTTP(c.Writer, c.Request)
}

// metricsTokenMatches accepts the token only from an Authorization: Bearer header.
// A query parameter is deliberately not supported: it would land the credential in
// access logs and proxy history.
//
// Both sides are hashed before comparison so the comparison runs over fixed-length
// input: ConstantTimeCompare returns early on a length mismatch, which would otherwise
// leak the token length.
func metricsTokenMatches(expected, authorization string) bool {
	presented := strings.TrimSpace(authorization)
	const prefix = "Bearer "
	if len(presented) <= len(prefix) || !strings.EqualFold(presented[:len(prefix)], prefix) {
		return false
	}
	presented = strings.TrimSpace(presented[len(prefix):])
	if presented == "" || expected == "" {
		return false
	}
	presentedSum := sha256.Sum256([]byte(presented))
	expectedSum := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(presentedSum[:], expectedSum[:]) == 1
}

func metricsLabel(value string) string {
	if value == "" {
		return metricsLabelUnset
	}
	return value
}

func metricsOutcome(statusCode int) string {
	if statusCode >= 200 && statusCode < 400 {
		return "success"
	}
	return "error"
}
