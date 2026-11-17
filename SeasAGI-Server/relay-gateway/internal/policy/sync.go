// Package policy 负责把控制面（platform-api）的渠道配置同步到 relay 数据面。
//
// 同步链路：GET {PLATFORM_API_URL}/api/v1/internal/relay/config（服务间令牌鉴权）
// → 校验快照 → channel.Store.ApplyControlPlane → 持久化（凭证加密落库）。
// 任何失败都保留上一次成功应用的配置（last known good），并按指数退避重试。
package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/channel"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

const (
	// relayConfigPath 是 platform-api 提供的服务间配置端点。
	relayConfigPath = "/api/v1/internal/relay/config"
	// supportedSchemaVersion 是本 relay 能理解的同步协议版本。
	supportedSchemaVersion = 1
	// maxSnapshotBytes 限制单次快照大小，防止异常响应耗尽内存。
	maxSnapshotBytes = 16 << 20

	defaultInterval   = 60 * time.Second
	defaultTimeout    = 10 * time.Second
	initialRetryDelay = 2 * time.Second
)

// Applier 是同步结果的落点；生产中为 *channel.Store。
type Applier interface {
	ApplyControlPlane([]channel.ControlPlaneChannel) channel.ApplyResult
	Flush() error
}

// Config 描述同步客户端的运行参数。
type Config struct {
	PlatformURL   string
	Token         string
	Interval      time.Duration
	Timeout       time.Duration
	AllowInsecure bool // 允许经非回环明文 HTTP 拉取凭证（默认拒绝）
}

// ConfigFromEnv 读取同步配置：
//
//	PLATFORM_API_URL            控制面地址，默认 http://127.0.0.1:9318
//	INTERNAL_API_TOKEN          服务间令牌（未配置时回退 ADMIN_SECRET，与 platform-api 一致）
//	POLICY_CACHE_SEC            同步周期（秒），默认 60
//	RELAY_SYNC_TIMEOUT_SEC      单次请求超时（秒），默认 10
//	RELAY_SYNC_ALLOW_INSECURE_HTTP=1  允许经非回环明文 HTTP 传输凭证
func ConfigFromEnv() Config {
	cfg := Config{
		PlatformURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("PLATFORM_API_URL")), "/"),
		Token:         internalToken(),
		Interval:      secondsFromEnv("POLICY_CACHE_SEC", defaultInterval),
		Timeout:       secondsFromEnv("RELAY_SYNC_TIMEOUT_SEC", defaultTimeout),
		AllowInsecure: os.Getenv("RELAY_SYNC_ALLOW_INSECURE_HTTP") == "1",
	}
	if cfg.PlatformURL == "" {
		cfg.PlatformURL = "http://127.0.0.1:9318"
	}
	return cfg
}

// internalToken 与 platform-api 的 usage.InternalAuth 采用同一取值顺序：
// 优先 INTERNAL_API_TOKEN，未配置时回退 ADMIN_SECRET。两者皆空则不发起同步（fail-closed）。
func internalToken() string {
	if token := strings.TrimSpace(os.Getenv("INTERNAL_API_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("ADMIN_SECRET"))
}

func secondsFromEnv(name string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
		logging.Warningf("invalid %s=%q, using default %s", name, v, fallback)
	}
	return fallback
}

// Status 是同步状态的只读快照（不含任何凭证），供运维接口展示。
type Status struct {
	Endpoint            string    `json:"endpoint"`
	LastAttempt         time.Time `json:"last_attempt,omitempty"`
	LastSuccess         time.Time `json:"last_success,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	AppliedVersion      string    `json:"applied_version,omitempty"`
	Channels            int       `json:"channels"`
	LastChange          time.Time `json:"last_change,omitempty"`
}

// Syncer 周期性拉取控制面配置并应用到 relay。
type Syncer struct {
	cfg     Config
	applier Applier
	client  *http.Client

	mu     sync.RWMutex
	status Status
}

// NewSyncer 创建同步客户端；不会发起任何网络请求。
func NewSyncer(cfg Config, applier Applier) *Syncer {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultInterval
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Syncer{
		cfg:     cfg,
		applier: applier,
		client:  &http.Client{Timeout: cfg.Timeout},
		status:  Status{Endpoint: cfg.PlatformURL + relayConfigPath},
	}
}

// Status 返回当前同步状态。
func (s *Syncer) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Run 立即同步一次，此后按周期同步；失败时以指数退避（上限为同步周期）重试，
// 期间 relay 继续使用最后一次成功应用的配置。ctx 取消后返回。
func (s *Syncer) Run(ctx context.Context) {
	delay := initialRetryDelay
	for {
		wait := s.cfg.Interval
		if err := s.SyncOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			logging.Errorf("relay config sync failed (keeping last known good config, retry in %s): %v", delay, err)
			wait = delay
			delay = min(delay*2, s.cfg.Interval)
		} else {
			delay = initialRetryDelay
		}
		// 抖动避免多个 relay 同时重试打满控制面。
		wait += time.Duration(rand.Int64N(int64(wait/10) + 1))

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// SyncOnce 拉取并应用一次配置。返回错误时不会修改 relay 的路由配置。
func (s *Syncer) SyncOnce(ctx context.Context) error {
	s.mu.Lock()
	s.status.LastAttempt = time.Now()
	applied := s.status.AppliedVersion
	s.mu.Unlock()

	result, version, err := s.fetchAndApply(ctx, applied)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.status.LastError = err.Error()
		s.status.ConsecutiveFailures++
		return err
	}
	s.status.LastError = ""
	s.status.ConsecutiveFailures = 0
	s.status.LastSuccess = time.Now()
	if version != "" {
		s.status.AppliedVersion = version
	}
	if result != nil {
		s.status.Channels = result.Added + result.Updated + result.Unchanged
		if result.Changed() {
			s.status.LastChange = time.Now()
		}
	}
	return nil
}

// errNotModified 表示控制面配置自上次应用后未变化。
var errNotModified = errors.New("not modified")

func (s *Syncer) fetchAndApply(ctx context.Context, appliedVersion string) (*channel.ApplyResult, string, error) {
	if s.cfg.Token == "" {
		return nil, "", errors.New("neither INTERNAL_API_TOKEN nor ADMIN_SECRET is configured; refusing to sync anonymously")
	}
	endpoint := s.cfg.PlatformURL + relayConfigPath
	if err := checkTransport(endpoint, s.cfg.AllowInsecure); err != nil {
		return nil, "", err
	}

	snap, err := s.fetch(ctx, endpoint, appliedVersion)
	if errors.Is(err, errNotModified) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	channels, err := snap.validate()
	if err != nil {
		return nil, "", fmt.Errorf("rejecting invalid relay config snapshot: %w", err)
	}

	res := s.applier.ApplyControlPlane(channels)
	if res.Changed() {
		logging.Infof("relay config %s applied: %d added, %d updated, %d removed, %d unchanged",
			shortVersion(snap.Version), res.Added, res.Updated, res.Removed, res.Unchanged)
	}
	// 立即持久化，保证控制面随后不可达时重启仍能使用这份配置。
	if err := s.applier.Flush(); err != nil {
		logging.Errorf("relay config %s applied in memory but could not be persisted (will retry in background): %v", shortVersion(snap.Version), err)
	}
	return &res, snap.Version, nil
}

type relayConfig struct {
	SchemaVersion int            `json:"schema_version"`
	Version       string         `json:"version"`
	Channels      []relayChannel `json:"channels"`
}

type relayChannel struct {
	ChannelID       string   `json:"channel_id"`
	ProviderType    string   `json:"provider_type"`
	DisplayName     string   `json:"display_name"`
	BaseURL         string   `json:"base_url"`
	APIKey          string   `json:"api_key"`
	Enabled         bool     `json:"enabled"`
	Models          []string `json:"models"`
	Weight          int      `json:"weight"`
	Priority        int      `json:"priority"`
	GrayPercent     int      `json:"gray_percent"`
	ReadOnly        bool     `json:"read_only"`
	CredentialError string   `json:"credential_error"`
}

func (s *Syncer) fetch(ctx context.Context, endpoint, appliedVersion string) (*relayConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build relay config request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	req.Header.Set("Accept", "application/json")
	if appliedVersion != "" {
		req.Header.Set("If-None-Match", `"`+appliedVersion+`"`)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch relay config: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		return nil, errNotModified
	default:
		// 只回显少量响应正文用于排查；控制面错误响应不包含凭证。
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("relay config endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSnapshotBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read relay config: %w", err)
	}
	if len(body) > maxSnapshotBytes {
		return nil, fmt.Errorf("relay config exceeds %d bytes", maxSnapshotBytes)
	}
	var snap relayConfig
	if err := json.Unmarshal(body, &snap); err != nil {
		return nil, fmt.Errorf("decode relay config: %w", err)
	}
	return &snap, nil
}

// validate 校验快照完整性；任何一处不合法都整体拒绝，避免部分应用。
func (c *relayConfig) validate() ([]channel.ControlPlaneChannel, error) {
	if c.SchemaVersion != supportedSchemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d (want %d)", c.SchemaVersion, supportedSchemaVersion)
	}
	if strings.TrimSpace(c.Version) == "" {
		return nil, errors.New("missing version")
	}
	if c.Channels == nil {
		return nil, errors.New("missing channels")
	}
	seen := make(map[string]bool, len(c.Channels))
	out := make([]channel.ControlPlaneChannel, 0, len(c.Channels))
	for i, ch := range c.Channels {
		id := strings.TrimSpace(ch.ChannelID)
		if id == "" {
			return nil, fmt.Errorf("channel #%d has no channel_id", i)
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate channel_id %q", id)
		}
		seen[id] = true
		u, err := url.Parse(ch.BaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("channel %q has invalid base_url", id)
		}
		if ch.Weight < 0 || ch.GrayPercent < 0 || ch.GrayPercent > 100 {
			return nil, fmt.Errorf("channel %q has out-of-range weight/gray_percent", id)
		}
		out = append(out, channel.ControlPlaneChannel{
			ChannelID:       id,
			ProviderType:    ch.ProviderType,
			DisplayName:     ch.DisplayName,
			BaseURL:         ch.BaseURL,
			APIKey:          ch.APIKey,
			Enabled:         ch.Enabled,
			Models:          ch.Models,
			Weight:          ch.Weight,
			Priority:        ch.Priority,
			GrayPercent:     ch.GrayPercent,
			ReadOnly:        ch.ReadOnly,
			CredentialError: ch.CredentialError,
		})
	}
	return out, nil
}

// checkTransport 拒绝经非回环明文 HTTP 拉取凭证，除非运维显式允许。
func checkTransport(endpoint string, allowInsecure bool) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid PLATFORM_API_URL %q", endpoint)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if allowInsecure || isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("refusing to fetch credentials over plain HTTP from non-loopback host %q; use https or set RELAY_SYNC_ALLOW_INSECURE_HTTP=1 on a trusted network", u.Hostname())
	default:
		return fmt.Errorf("unsupported PLATFORM_API_URL scheme %q", u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func shortVersion(v string) string {
	if len(v) > 12 {
		return v[:12]
	}
	return v
}
