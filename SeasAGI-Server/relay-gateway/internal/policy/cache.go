package policy

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

// 策略同步客户端必须带超时，避免控制面不可达时刷新协程永久挂住。
var channelSyncClient = &http.Client{Timeout: 10 * time.Second}

// internalToken 与 platform-api 的 usage.InternalAuth 采用同一取值顺序：
// 优先 INTERNAL_API_TOKEN，未配置时回退 ADMIN_SECRET。两者皆空则拒绝发送
// 匿名请求（fail-closed），不再让网关拿着用户态 URL 去撞 401。
func internalToken() string {
	if token := strings.TrimSpace(os.Getenv("INTERNAL_API_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("ADMIN_SECRET"))
}

type ChannelSnapshot struct {
	ChannelID    string   `json:"channel_id"`
	ProviderType string   `json:"provider_type"`
	BaseURL      string   `json:"base_url"`
	Enabled      bool     `json:"enabled"`
	Models       []string `json:"models"`
	SortOrder    int      `json:"sort_order"`
}

type Cache struct {
	mu               sync.RWMutex
	channels         []ChannelSnapshot
	lastChannelFetch time.Time
	refreshInterval  time.Duration
	platformAPIURL   string
}

var globalCache *Cache

func InitCache() {
	refreshSec := 60
	if v := os.Getenv("POLICY_CACHE_SEC"); v != "" {
		if n, err := time.ParseDuration(v + "s"); err == nil {
			refreshSec = int(n.Seconds())
		}
	}

	platformURL := os.Getenv("PLATFORM_API_URL")
	if platformURL == "" {
		platformURL = "http://127.0.0.1:9318"
	}

	globalCache = &Cache{
		channels:        []ChannelSnapshot{},
		refreshInterval: time.Duration(refreshSec) * time.Second,
		platformAPIURL:  platformURL,
	}

	go globalCache.refreshLoop()
}

func GetChannels() []ChannelSnapshot {
	if globalCache == nil {
		return nil
	}
	globalCache.mu.RLock()
	defer globalCache.mu.RUnlock()
	return globalCache.channels
}

func (c *Cache) refreshLoop() {
	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()

	c.refreshChannels()

	for range ticker.C {
		c.refreshChannels()
	}
}

func (c *Cache) refreshChannels() {
	token := internalToken()
	if token == "" {
		logging.Errorf("policy sync skipped: neither INTERNAL_API_TOKEN nor ADMIN_SECRET is configured")
		return
	}

	url := c.platformAPIURL + "/api/v1/internal/channels/snapshot"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		logging.Errorf("channel fetch request error: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := channelSyncClient.Do(req)
	if err != nil {
		logging.Errorf("channel fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logging.Errorf("channel snapshot returned status %d", resp.StatusCode)
		return
	}

	var result struct {
		Data []ChannelSnapshot `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logging.Errorf("channel decode error: %v", err)
		return
	}

	c.mu.Lock()
	c.channels = result.Data
	c.lastChannelFetch = time.Now()
	c.mu.Unlock()
}
