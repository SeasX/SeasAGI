package health

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/channel"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

type Checker struct {
	store    *channel.Store
	interval time.Duration
	stopCh   chan struct{}
	mu       sync.Mutex
}

func NewChecker(store *channel.Store, interval time.Duration) *Checker {
	if interval == 0 {
		interval = 60 * time.Second
	}
	return &Checker{
		store:    store,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (h *Checker) Start() {
	go h.loop()
}

func (h *Checker) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case h.stopCh <- struct{}{}:
	default:
	}
}

func (h *Checker) loop() {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	h.checkAll()

	for {
		select {
		case <-ticker.C:
			h.checkAll()
		case <-h.stopCh:
			return
		}
	}
}

func (h *Checker) checkAll() {
	channels := h.store.ListEnabledChannels()
	for _, ch := range channels {
		healthy, statusCode, errStr := h.checkOne(ch.BaseURL, ch.APIKey)
		consecFail := 0
		if !healthy {
			consecFail = ch.Health.ConsecFail + 1
		}
		h.store.UpdateHealth(ch.ChannelID, channel.HealthStatus{
			Healthy:    healthy,
			StatusCode: statusCode,
			Error:      errStr,
			ConsecFail: consecFail,
		})
	}
}

func (h *Checker) checkOne(baseURL, apiKey string) (bool, int, string) {
	checkURL := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequest(http.MethodGet, checkURL, nil)
	if err != nil {
		return false, 0, err.Error()
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logging.Errorf("health check failed for %s: %v", baseURL, err)
		return false, 0, err.Error()
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	if !healthy {
		logging.Warningf("health check unhealthy for %s: status %d", baseURL, resp.StatusCode)
	}
	return healthy, resp.StatusCode, ""
}
