package sync

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/urlguard"
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

// validateWebDAVURL 强制 WebDAV 端点使用 HTTPS（仅允许本机回环使用明文 HTTP），
// 并拒绝链路本地/未指定等不可路由目标，避免 Basic 凭证明文外泄或被当作 SSRF 跳板（CS-12）。
func (m *Manager) validateWebDAVURL() error {
	raw := strings.TrimSpace(m.config.WebDAVURL)
	if err := urlguard.ValidateSecureURL(raw, true); err != nil {
		return fmt.Errorf("invalid WebDAV URL: %w", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid WebDAV URL: %w", err)
	}
	if err := urlguard.RejectLinkLocalHost(u.Host); err != nil {
		return fmt.Errorf("invalid WebDAV URL: %w", err)
	}
	return nil
}

func (m *Manager) pushWebDAV(localDir string) error {
	if err := m.validateWebDAVURL(); err != nil {
		return err
	}
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
	if err := m.validateWebDAVURL(); err != nil {
		return err
	}
	// 两阶段拉取：先把全部远端文件下载并验签/解密到内存，全部通过后再统一落盘，
	// 任一文件失败都不会留下半套本地配置（CS-06）。
	staged := make([]stagedSyncFile, 0, len(syncFileNames))
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
		staged = append(staged, stagedSyncFile{name: file, data: payload})
	}
	for _, item := range staged {
		if err := writeSyncFile(filepath.Join(localDir, item.name), item.data); err != nil {
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

// stagedSyncFile 表示已完成加签/验签、等待统一落盘的文件内容。
type stagedSyncFile struct {
	name string
	data []byte
}

// syncDirs 读取 srcDir 中的同步文件，经 transform（推送加签 / 拉取验签）处理后写入 dstDir。
// 采用两阶段处理：先对全部文件完成 transform 并暂存内存，全部成功后再统一写盘；
// 任一文件验签或加签失败都立即返回且不写入任何文件，避免留下半套本地配置（CL-S7/CS-06）。
func syncDirs(srcDir, dstDir string, transform func([]byte) ([]byte, error)) error {
	staged := make([]stagedSyncFile, 0, len(syncFileNames))
	for _, file := range syncFileNames {
		data, err := os.ReadFile(filepath.Join(srcDir, file))
		if err != nil {
			continue
		}
		out, err := transform(data)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		staged = append(staged, stagedSyncFile{name: file, data: out})
	}
	for _, item := range staged {
		if err := writeSyncFile(filepath.Join(dstDir, item.name), item.data); err != nil {
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
