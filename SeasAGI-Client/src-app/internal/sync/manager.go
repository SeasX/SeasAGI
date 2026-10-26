package sync

import (
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
	ProviderICloud  SyncProvider = "icloud"
	ProviderWebDAV  SyncProvider = "webdav"
	ProviderDropbox SyncProvider = "dropbox"
	ProviderLocal   SyncProvider = "local"
)

type SyncConfig struct {
	Provider   SyncProvider `json:"provider"`
	RemotePath string       `json:"remote_path"`
	WebDAVURL  string       `json:"webdav_url,omitempty"`
	Username   string       `json:"username,omitempty"`
	Password   string       `json:"password,omitempty"`
	AutoSync   bool         `json:"auto_sync"`
	Interval   int          `json:"interval"`
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
	os.MkdirAll(icloudDir, 0700)
	return syncDirs(localDir, icloudDir, sealSyncFile)
}

func (m *Manager) pullICloud(localDir string) error {
	homeDir, _ := os.UserHomeDir()
	icloudDir := filepath.Join(homeDir, "Library", "Mobile Documents", "com~apple~CloudDocs", "SeasAGI")
	if _, err := os.Stat(icloudDir); err != nil {
		return fmt.Errorf("iCloud directory not found")
	}
	return syncDirs(icloudDir, localDir, openSyncFile)
}

func (m *Manager) pushWebDAV(localDir string) error {
	for _, file := range syncFileNames {
		localPath := filepath.Join(localDir, file)
		if _, err := os.Stat(localPath); err != nil {
			continue
		}
		data, err := os.ReadFile(localPath)
		if err != nil {
			continue
		}
		sealed, err := sealSyncFile(data)
		if err != nil {
			return fmt.Errorf("sign %s: %w", file, err)
		}
		remoteURL := strings.TrimRight(m.config.WebDAVURL, "/") + "/" + file
		req, err := http.NewRequest("PUT", remoteURL, strings.NewReader(string(sealed)))
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
	for _, file := range syncFileNames {
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
		if resp.StatusCode == 404 {
			resp.Body.Close()
			continue
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return fmt.Errorf("WebDAV GET failed: %d", resp.StatusCode)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		payload, err := openSyncFile(data)
		if err != nil {
			return fmt.Errorf("verify %s: %w", file, err)
		}
		if err := writeSyncFile(filepath.Join(localDir, file), payload); err != nil {
			return err
		}
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
	os.MkdirAll(m.config.RemotePath, 0700)
	return syncDirs(localDir, m.config.RemotePath, sealSyncFile)
}

func (m *Manager) pullLocal(localDir string) error {
	if m.config.RemotePath == "" {
		return fmt.Errorf("local sync path not configured")
	}
	return syncDirs(m.config.RemotePath, localDir, openSyncFile)
}

// syncFileNames 是可同步的配置文件清单。
var syncFileNames = []string{"mcp_servers.json", "prompt_presets.json", "skills.json", "usage_records.json"}

// syncDirs 读取 srcDir 中的同步文件，经 transform（推送加签 / 拉取验签）处理后写入 dstDir。
// 任一文件验签或加签失败都整体返回错误，调用方不得应用未经验证的远端内容（CL-S7）。
func syncDirs(srcDir, dstDir string, transform func([]byte) ([]byte, error)) error {
	for _, file := range syncFileNames {
		data, err := os.ReadFile(filepath.Join(srcDir, file))
		if err != nil {
			continue
		}
		out, err := transform(data)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if err := writeSyncFile(filepath.Join(dstDir, file), out); err != nil {
			return err
		}
	}
	return nil
}

// sealSyncFile 用本机同步密钥对文件内容加签，返回 HMAC 信封。
func sealSyncFile(data []byte) ([]byte, error) {
	key, err := syncSigningKey()
	if err != nil {
		return nil, err
	}
	return sealSyncPayload(data, key)
}

// openSyncFile 校验信封签名并返回原始文件内容；密钥缺失或签名不符均返回错误。
func openSyncFile(data []byte) ([]byte, error) {
	key, err := syncVerificationKey()
	if err != nil {
		return nil, err
	}
	return openSyncPayload(data, key)
}

// writeSyncFile 以 0600 原子写入同步文件，避免半截文件与全局可读（CL-S7/CL-S8）。
func writeSyncFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
