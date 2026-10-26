package tunnel

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type CloudflareTunnel struct {
	cmd       *exec.Cmd
	url       string
	running   bool
	mu        sync.Mutex
	localPort int
}

func NewCloudflareTunnel(localPort int) *CloudflareTunnel {
	return &CloudflareTunnel{localPort: localPort}
}

func (t *CloudflareTunnel) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}

	binPath, err := ensureCloudflared()
	if err != nil {
		return fmt.Errorf("cloudflared not available: %w", err)
	}

	cmd := exec.CommandContext(ctx, binPath, "tunnel",
		"--url", fmt.Sprintf("http://127.0.0.1:%d", t.localPort),
	)

	stderr, _ := cmd.StderrPipe()
	cmd.Stdout = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cloudflared: %w", err)
	}

	t.cmd = cmd
	t.running = true

	go t.watchOutput(stderr)

	return nil
}

func (t *CloudflareTunnel) watchOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "https://") && strings.Contains(line, ".trycloudflare.com") {
			start := strings.Index(line, "https://")
			rest := line[start:]
			end := strings.IndexAny(rest, " \t\n\r\"'")
			if end == -1 {
				end = len(rest)
			}
			url := rest[:end]
			t.mu.Lock()
			t.url = url
			t.mu.Unlock()
		}
	}
}

func (t *CloudflareTunnel) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || t.cmd == nil || t.cmd.Process == nil {
		return nil
	}

	err := t.cmd.Process.Kill()
	t.running = false
	t.url = ""
	return err
}

func (t *CloudflareTunnel) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *CloudflareTunnel) GetURL() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.url
}

// 锁定的 cloudflared 版本与官方发布的 SHA-256。
// 版本固定可避免「latest」被上游替换；摘要来自 Cloudflare 官方 GitHub Release，
// 校验失败时不执行、不缓存，避免第三方二进制落地。
const (
	cloudflaredVersion     = "2025.10.0"
	cloudflaredMaxDownload = 200 << 20 // 200MB
)

type cloudflaredAsset struct {
	Name    string
	SHA256  string
	Archive bool // darwin 发布物为 .tgz，需要解包出 cloudflared
}

var cloudflaredAssets = map[string]cloudflaredAsset{
	"darwin/arm64":  {"cloudflared-darwin-arm64.tgz", "03fab9b8b4b0305518fd8c23d90f94ee8e93fd940bb54a5027463fcafe1c039d", true},
	"darwin/amd64":  {"cloudflared-darwin-amd64.tgz", "11f54a2ef8eec9045b9cbf8537d2c17ef62322e7a1af80996e755a6507f8cff8", true},
	"linux/amd64":   {"cloudflared-linux-amd64", "d334c30ab628d4698455acadae5bdb3e33705a5f8dade510ce5ac7fc75d327fe", false},
	"linux/arm64":   {"cloudflared-linux-arm64", "ad60157f5c4ecf871ae4e9dc5d9a7774b456dbfea8cf26b586c62cb0ff69dd45", false},
	"windows/amd64": {"cloudflared-windows-amd64.exe", "07ce5e7ffc805734797d37061654f5f0152de2a2da241bf52e90822e9b8fc724", false},
}

func cloudflaredAssetFor(goos, goarch string) (cloudflaredAsset, bool) {
	asset, ok := cloudflaredAssets[goos+"/"+goarch]
	return asset, ok
}

// ensureCloudflared 只使用 SeasAGI 自管的固定版本二进制，绝不从 PATH 解析，
// 避免命中用户/攻击者放置的同名可执行文件。
func ensureCloudflared() (string, error) {
	asset, ok := cloudflaredAssetFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return "", fmt.Errorf("no pinned cloudflared build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	dir, err := cloudflaredCacheDir()
	if err != nil {
		return "", err
	}
	name := "cloudflared-" + cloudflaredVersion
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binPath := filepath.Join(dir, name)

	if _, err := os.Stat(binPath); err == nil {
		if err := verifyCachedCloudflared(binPath, asset); err == nil {
			return binPath, nil
		}
		// 缓存损坏或摘要不一致：删除后重新下载。
		_ = os.Remove(binPath)
		_ = os.Remove(binPath + ".sha256")
	}

	if err := downloadCloudflared(binPath, asset); err != nil {
		return "", err
	}
	return binPath, nil
}

// cloudflaredCacheDir 返回 0700 的自管缓存目录。
func cloudflaredCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	dir := filepath.Join(home, ".seasagi", "bin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create cloudflared cache dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("chmod cloudflared cache dir: %w", err)
	}
	return dir, nil
}

func verifyCachedCloudflared(binPath string, asset cloudflaredAsset) error {
	expected := asset.SHA256
	if asset.Archive {
		// 解包后的二进制摘要无法由 tgz 摘要直接推导，首次安装时记录到 0600 sidecar。
		marker, err := os.ReadFile(binPath + ".sha256")
		if err != nil {
			return fmt.Errorf("read cached cloudflared digest: %w", err)
		}
		expected = strings.TrimSpace(string(marker))
	}
	if !isHexDigest(expected) {
		return fmt.Errorf("invalid expected digest for cached cloudflared")
	}
	return verifyFileSHA256(binPath, expected)
}

func verifyFileSHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("SHA-256 mismatch: expected %s, got %s", expected, got)
	}
	return nil
}

func isHexDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func downloadCloudflared(binPath string, asset cloudflaredAsset) error {
	downloadURL := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/download/%s/%s",
		cloudflaredVersion, asset.Name)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download cloudflared: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download cloudflared: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, cloudflaredMaxDownload+1))
	if err != nil {
		return fmt.Errorf("read cloudflared download: %w", err)
	}
	if len(data) > cloudflaredMaxDownload {
		return fmt.Errorf("cloudflared download exceeds maximum size")
	}

	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, asset.SHA256) {
		return fmt.Errorf("cloudflared SHA-256 mismatch: expected %s, got %s", asset.SHA256, got)
	}

	binary := data
	if asset.Archive {
		binary, err = extractCloudflared(data)
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(binPath)
	tmp, err := os.CreateTemp(dir, ".cloudflared-*")
	if err != nil {
		return fmt.Errorf("create temp cloudflared: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o700); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp cloudflared: %w", err)
	}
	if _, err := tmp.Write(binary); err != nil {
		cleanup()
		return fmt.Errorf("write temp cloudflared: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp cloudflared: %w", err)
	}
	if err := os.Rename(tmpName, binPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("install cloudflared: %w", err)
	}
	_ = os.Chmod(binPath, 0o700)

	// 为解包安装记录二进制自身摘要，后续启动时校验缓存未被替换。
	binSum := sha256.Sum256(binary)
	if err := os.WriteFile(binPath+".sha256", []byte(hex.EncodeToString(binSum[:])), 0o600); err != nil {
		_ = os.Remove(binPath)
		return fmt.Errorf("write cloudflared digest marker: %w", err)
	}
	return nil
}

// extractCloudflared 从官方 .tgz 中取出 cloudflared 可执行文件。
func extractCloudflared(tgz []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, fmt.Errorf("gzip cloudflared archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read cloudflared archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "cloudflared" && filepath.Base(hdr.Name) != "cloudflared.exe" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, cloudflaredMaxDownload+1))
		if err != nil {
			return nil, fmt.Errorf("extract cloudflared: %w", err)
		}
		if len(data) > cloudflaredMaxDownload {
			return nil, fmt.Errorf("extracted cloudflared exceeds maximum size")
		}
		return data, nil
	}
	return nil, fmt.Errorf("cloudflared binary not found in archive")
}

func (t *CloudflareTunnel) WaitForURL(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if url := t.GetURL(); url != "" {
			return url, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("timed out waiting for tunnel URL")
}
