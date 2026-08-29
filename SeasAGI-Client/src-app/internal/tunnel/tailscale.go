package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type TailscaleTunnel struct {
	cmd       *exec.Cmd
	running   bool
	mu        sync.Mutex
	localPort int
}

func NewTailscaleTunnel(localPort int) *TailscaleTunnel {
	return &TailscaleTunnel{localPort: localPort}
}

func (t *TailscaleTunnel) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}

	if _, err := exec.LookPath("tailscale"); err != nil {
		return fmt.Errorf("tailscale CLI not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, "tailscale", "funnel", "--bg",
		fmt.Sprintf("--listen=:%d", t.localPort),
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start tailscale funnel: %w", err)
	}

	t.cmd = cmd
	t.running = true

	return nil
}

func (t *TailscaleTunnel) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return nil
	}

	stopCmd := exec.Command("tailscale", "funnel", "--off")
	_ = stopCmd.Run()

	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}

	t.running = false
	return nil
}

func (t *TailscaleTunnel) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *TailscaleTunnel) GetURL() string {
	statusCmd := exec.Command("tailscale", "status", "--json")
	output, err := statusCmd.Output()
	if err != nil {
		return ""
	}
	var status struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	}
	if err := json.Unmarshal(output, &status); err != nil {
		return ""
	}
	if status.Self.DNSName != "" {
		host := strings.TrimSuffix(status.Self.DNSName, ".")
		return fmt.Sprintf("https://%s", host)
	}
	return ""
}
