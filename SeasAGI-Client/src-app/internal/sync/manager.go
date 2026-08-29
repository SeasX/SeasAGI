package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SyncProvider string

const (
	ProviderICloud   SyncProvider = "icloud"
	ProviderWebDAV   SyncProvider = "webdav"
	ProviderDropbox  SyncProvider = "dropbox"
	ProviderLocal    SyncProvider = "local"
)

type SyncConfig struct {
	Provider   SyncProvider `json:"provider"`
	RemotePath string      `json:"remote_path"`
	WebDAVURL  string      `json:"webdav_url,omitempty"`
	Username   string      `json:"username,omitempty"`
	Password   string      `json:"password,omitempty"`
	AutoSync   bool        `json:"auto_sync"`
	Interval   int         `json:"interval"`
}

type SyncStatus struct {
	LastSyncTime string `json:"last_sync_time"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

type Manager struct {
	mu     sync.Mutex
	config SyncConfig
	status SyncStatus
	client *http.Client
}

func NewManager(config SyncConfig) *Manager {
	return &Manager{
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *Manager) GetConfig() SyncConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

func (m *Manager) UpdateConfig(config SyncConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

func (m *Manager) GetStatus() SyncStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Manager) Push(localDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.status.Status = "syncing"
	m.status.Error = ""

	switch m.config.Provider {
	case ProviderICloud:
		err := m.pushICloud(localDir)
		m.updateResult(err)
		return err
	case ProviderWebDAV:
		err := m.pushWebDAV(localDir)
		m.updateResult(err)
		return err
	case ProviderDropbox:
		err := m.pushDropbox(localDir)
		m.updateResult(err)
		return err
	case ProviderLocal:
		err := m.pushLocal(localDir)
		m.updateResult(err)
		return err
	default:
		err := fmt.Errorf("unsupported sync provider: %s", m.config.Provider)
		m.updateResult(err)
		return err
	}
}

func (m *Manager) Pull(localDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.status.Status = "syncing"
	m.status.Error = ""

	switch m.config.Provider {
	case ProviderICloud:
		err := m.pullICloud(localDir)
		m.updateResult(err)
		return err
	case ProviderWebDAV:
		err := m.pullWebDAV(localDir)
		m.updateResult(err)
		return err
	case ProviderDropbox:
		err := m.pullDropbox(localDir)
		m.updateResult(err)
		return err
	case ProviderLocal:
		err := m.pullLocal(localDir)
		m.updateResult(err)
		return err
	default:
		err := fmt.Errorf("unsupported sync provider: %s", m.config.Provider)
		m.updateResult(err)
		return err
	}
}

func (m *Manager) updateResult(err error) {
	m.status.LastSyncTime = time.Now().Format(time.RFC3339)
	if err != nil {
		m.status.Status = "error"
		m.status.Error = err.Error()
	} else {
		m.status.Status = "success"
	}
}

func (m *Manager) pushICloud(localDir string) error {
	homeDir, _ := os.UserHomeDir()
	icloudDir := filepath.Join(homeDir, "Library", "Mobile Documents", "com~apple~CloudDocs", "SeasAGI")
	os.MkdirAll(icloudDir, 0755)
	return syncDirs(localDir, icloudDir)
}

func (m *Manager) pullICloud(localDir string) error {
	homeDir, _ := os.UserHomeDir()
	icloudDir := filepath.Join(homeDir, "Library", "Mobile Documents", "com~apple~CloudDocs", "SeasAGI")
	if _, err := os.Stat(icloudDir); err != nil {
		return fmt.Errorf("iCloud directory not found")
	}
	return syncDirs(icloudDir, localDir)
}

func (m *Manager) pushWebDAV(localDir string) error {
	files := []string{"mcp_servers.json", "prompt_presets.json", "skills.json", "usage_records.json"}
	for _, file := range files {
		localPath := filepath.Join(localDir, file)
		if _, err := os.Stat(localPath); err != nil {
			continue
		}
		data, err := os.ReadFile(localPath)
		if err != nil {
			continue
		}
		remoteURL := strings.TrimRight(m.config.WebDAVURL, "/") + "/" + file
		req, err := http.NewRequest("PUT", remoteURL, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		req.SetBasicAuth(m.config.Username, m.config.Password)
		resp, err := m.client.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("WebDAV PUT failed: %d", resp.StatusCode)
		}
	}
	return nil
}

func (m *Manager) pullWebDAV(localDir string) error {
	files := []string{"mcp_servers.json", "prompt_presets.json", "skills.json", "usage_records.json"}
	for _, file := range files {
		remoteURL := strings.TrimRight(m.config.WebDAVURL, "/") + "/" + file
		req, err := http.NewRequest("GET", remoteURL, nil)
		if err != nil {
			return err
		}
		req.SetBasicAuth(m.config.Username, m.config.Password)
		resp, err := m.client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			continue
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("WebDAV GET failed: %d", resp.StatusCode)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		os.MkdirAll(localDir, 0755)
		os.WriteFile(filepath.Join(localDir, file), data, 0644)
	}
	return nil
}

func (m *Manager) pushDropbox(localDir string) error {
	return fmt.Errorf("Dropbox sync requires OAuth integration (not yet implemented)")
}

func (m *Manager) pullDropbox(localDir string) error {
	return fmt.Errorf("Dropbox sync requires OAuth integration (not yet implemented)")
}

func (m *Manager) pushLocal(localDir string) error {
	if m.config.RemotePath == "" {
		return fmt.Errorf("local sync path not configured")
	}
	os.MkdirAll(m.config.RemotePath, 0755)
	return syncDirs(localDir, m.config.RemotePath)
}

func (m *Manager) pullLocal(localDir string) error {
	if m.config.RemotePath == "" {
		return fmt.Errorf("local sync path not configured")
	}
	return syncDirs(m.config.RemotePath, localDir)
}

func syncDirs(srcDir, dstDir string) error {
	files := []string{"mcp_servers.json", "prompt_presets.json", "skills.json", "usage_records.json"}
	for _, file := range files {
		srcPath := filepath.Join(srcDir, file)
		dstPath := filepath.Join(dstDir, file)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			continue
		}
		var srcJSON, dstJSON interface{}
		json.Unmarshal(data, &srcJSON)
		if dstData, err := os.ReadFile(dstPath); err == nil {
			json.Unmarshal(dstData, &dstJSON)
		}
		os.MkdirAll(filepath.Dir(dstPath), 0755)
		os.WriteFile(dstPath, data, 0644)
	}
	return nil
}
