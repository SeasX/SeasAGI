package acp

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// AcpSession 表示一个 CLI agent 会话。
type AcpSession struct {
	ID        string
	Binary    string
	Args      []string
	cmd       *exec.Cmd
	stdin     chan string
	stdout    chan string
	stderr    chan string
	cancel    context.CancelFunc
	mu        sync.Mutex
	startedAt time.Time
}

// AcpManager 管理 CLI agent 子进程。
type AcpManager struct {
	mu       sync.Mutex
	sessions map[string]*AcpSession
	registry *Registry
}

// NewAcpManager 创建 ACP 管理器。
func NewAcpManager() *AcpManager {
	return &AcpManager{
		sessions: make(map[string]*AcpSession),
		registry: NewRegistry(),
	}
}

// Spawn 启动一个 CLI agent 子进程。
func (m *AcpManager) Spawn(agentID, binary string, args []string, env []string) (*AcpSession, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...)
	if len(env) > 0 {
		cmd.Env = env
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start process: %w", err)
	}

	session := &AcpSession{
		ID:        agentID,
		Binary:    binary,
		Args:      args,
		cmd:       cmd,
		stdin:     make(chan string, 16),
		stdout:    make(chan string, 64),
		stderr:    make(chan string, 64),
		cancel:    cancel,
		startedAt: time.Now(),
	}

	// 写入 stdin 的 goroutine
	go func() {
		for line := range session.stdin {
			_, err := stdinPipe.Write([]byte(line + "\n"))
			if err != nil {
				return
			}
		}
		stdinPipe.Close()
	}()

	// 读取 stdout 的 goroutine
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			session.stdout <- scanner.Text()
		}
	}()

	// 读取 stderr 的 goroutine
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			session.stderr <- scanner.Text()
		}
	}()

	m.mu.Lock()
	m.sessions[agentID] = session
	m.mu.Unlock()

	return session, nil
}

// SendPrompt 发送 prompt 并等待响应。
// 使用 2s 空闲检测判断响应完成，120s 超时。
func (m *AcpManager) SendPrompt(sessionID, prompt string, timeoutMs int) (string, error) {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("session %s not found", sessionID)
	}

	if timeoutMs <= 0 {
		timeoutMs = 120000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	idleTimeout := 2 * time.Second

	// 发送 prompt
	session.stdin <- prompt

	// 收集响应，2s 空闲检测
	var output strings.Builder
	deadline := time.Now().Add(timeout)
	idleTimer := time.NewTimer(idleTimeout)
	defer idleTimer.Stop()

	for {
		select {
		case line := <-session.stdout:
			output.WriteString(line)
			output.WriteString("\n")
			// 重置空闲计时器
			if !idleTimer.Stop() {
				<-idleTimer.C
			}
			idleTimer.Reset(idleTimeout)

		case <-idleTimer.C:
			// 2s 无新输出，认为响应完成
			return output.String(), nil

		case <-time.After(time.Until(deadline)):
			return output.String(), fmt.Errorf("timeout after %dms", timeoutMs)
		}
	}
}

// Kill 终止会话进程。先 SIGTERM，5s 后 SIGKILL。
func (m *AcpManager) Kill(sessionID string) bool {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return false
	}

	session.cancel()

	// 等待进程退出
	done := make(chan struct{})
	go func() {
		session.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// SIGTERM 后 5s 仍未退出，强制 SIGKILL
		if session.cmd.Process != nil {
			session.cmd.Process.Kill()
		}
	}

	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	close(session.stdin)
	return true
}

// GetSession 返回指定会话。
func (m *AcpManager) GetSession(sessionID string) (*AcpSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	return s, ok
}

// ListSessions 返回所有会话 ID。
func (m *AcpManager) ListSessions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Registry 返回注册表。
func (m *AcpManager) Registry() *Registry {
	return m.registry
}
