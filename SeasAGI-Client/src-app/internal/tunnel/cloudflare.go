package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

func ensureCloudflared() (string, error) {
	if path, err := exec.LookPath("cloudflared"); err == nil {
		return path, nil
	}

	home, _ := os.UserHomeDir()
	binPath := home + "/.seasagi/bin/cloudflared"
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}

	downloadURL := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-%s", runtime.GOARCH)
	if runtime.GOARCH == "arm64" {
		downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-arm64"
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download cloudflared: HTTP %d", resp.StatusCode)
	}

	os.MkdirAll(home+"/.seasagi/bin", 0755)
	f, err := os.Create(binPath)
	if err != nil {
		return "", err
	}
	io.Copy(f, resp.Body)
	f.Close()
	os.Chmod(binPath, 0755)

	return binPath, nil
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
