package policy

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

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
	url := c.platformAPIURL + "/api/v1/channels"
	resp, err := http.Get(url)
	if err != nil {
		logging.Errorf("channel fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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
