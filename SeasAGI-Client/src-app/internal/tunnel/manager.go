package tunnel

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type TunnelType string

const (
	TypeCloudflare TunnelType = "cloudflare"
	TypeTailscale  TunnelType = "tailscale"
)

type Manager struct {
	cf        *CloudflareTunnel
	ts        *TailscaleTunnel
	localPort int
	active    TunnelType
	running   bool
	mu        sync.Mutex
}

func NewManager(localPort int) *Manager {
	return &Manager{
		localPort: localPort,
		cf:        NewCloudflareTunnel(localPort),
		ts:        NewTailscaleTunnel(localPort),
	}
}

func (m *Manager) Start(ctx context.Context, tunnelType TunnelType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	switch tunnelType {
	case TypeCloudflare:
		if err := m.cf.Start(ctx); err != nil {
			return err
		}
	case TypeTailscale:
		if err := m.ts.Start(ctx); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown tunnel type: %s", tunnelType)
	}

	m.active = tunnelType
	m.running = true

	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	var err error
	switch m.active {
	case TypeCloudflare:
		err = m.cf.Stop()
	case TypeTailscale:
		err = m.ts.Stop()
	}

	m.running = false
	m.active = ""
	return err
}

func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Manager) GetURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.active {
	case TypeCloudflare:
		return m.cf.GetURL()
	case TypeTailscale:
		return m.ts.GetURL()
	}
	return ""
}

func (m *Manager) GetStatus() map[string]any {
	m.mu.Lock()
	running := m.running
	active := m.active
	localPort := m.localPort
	m.mu.Unlock()

	url := m.GetURL()

	return map[string]any{
		"running":     running,
		"active_type": string(active),
		"url":         url,
		"local_port":  localPort,
	}
}

func (m *Manager) CheckReachability() bool {
	url := m.GetURL()
	if url == "" {
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url + "/v1/models")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized
}
