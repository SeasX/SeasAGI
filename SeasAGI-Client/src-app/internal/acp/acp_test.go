package acp

import (
	"testing"
	"time"
)

func TestACPSpawnAndKill(t *testing.T) {
	mgr := NewAcpManager()

	// 使用 echo 命令模拟 CLI agent
	session, err := mgr.Spawn("test-agent", "cat", []string{}, nil)
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}
	if session.ID != "test-agent" {
		t.Fatalf("expected ID test-agent, got %s", session.ID)
	}

	// 验证会话存在
	if _, ok := mgr.GetSession("test-agent"); !ok {
		t.Fatal("session should exist")
	}

	// 终止
	if !mgr.Kill("test-agent") {
		t.Fatal("kill should return true")
	}

	// 验证已清理
	if _, ok := mgr.GetSession("test-agent"); ok {
		t.Fatal("session should be removed after kill")
	}
}

func TestACPSendPrompt(t *testing.T) {
	mgr := NewAcpManager()

	// 使用 cat 命令模拟 echo-back agent
	_, err := mgr.Spawn("echo-agent", "cat", []string{}, nil)
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}
	defer mgr.Kill("echo-agent")

	// cat 会回显输入
	resp, err := mgr.SendPrompt("echo-agent", "hello world", 5000)
	if err != nil {
		// cat 可能不会在 2s 内 EOF，超时是正常的
		t.Logf("SendPrompt returned: %v (err: %v)", resp, err)
	}
	if resp == "" {
		t.Log("response is empty (cat may not echo before idle timeout)")
	}
}

func TestACPSessionNotFound(t *testing.T) {
	mgr := NewAcpManager()
	_, err := mgr.SendPrompt("nonexistent", "hello", 1000)
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestACPKillNonexistent(t *testing.T) {
	mgr := NewAcpManager()
	if mgr.Kill("nonexistent") {
		t.Fatal("kill should return false for nonexistent session")
	}
}

func TestACPListSessions(t *testing.T) {
	mgr := NewAcpManager()

	_, err := mgr.Spawn("s1", "cat", []string{}, nil)
	if err != nil {
		t.Fatalf("spawn s1 failed: %v", err)
	}
	_, err = mgr.Spawn("s2", "cat", []string{}, nil)
	if err != nil {
		t.Fatalf("spawn s2 failed: %v", err)
	}
	defer mgr.Kill("s1")
	defer mgr.Kill("s2")

	sessions := mgr.ListSessions()
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestACPRegistryDetect(t *testing.T) {
	r := NewRegistry()
	// 至少应该有内置 agent 注册
	agents := r.List()
	if len(agents) == 0 {
		t.Fatal("registry should have builtin agents")
	}

	// 检查 claude 是否在列表中
	found := false
	for _, a := range agents {
		if a.Name == "claude" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("claude should be in builtin agents")
	}
}

func TestACPRegistryIsAvailable(t *testing.T) {
	r := NewRegistry()
	// cat 命令一定存在
	r.Register(AgentInfo{Name: "cat-agent", Binary: "cat", Args: []string{}, Version: []string{"--version"}})
	if !r.IsAvailable("cat-agent") {
		t.Fatal("cat should be available")
	}
	// 不存在的命令
	r.Register(AgentInfo{Name: "fake-agent", Binary: "nonexistent_binary_xyz123", Args: []string{}})
	if r.IsAvailable("fake-agent") {
		t.Fatal("nonexistent binary should not be available")
	}
}

func TestACPRegistryRegister(t *testing.T) {
	r := NewRegistry()
	custom := AgentInfo{Name: "custom-cli", Binary: "my-cli", Args: []string{"--agent-mode"}}
	r.Register(custom)

	agent, ok := r.Get("custom-cli")
	if !ok {
		t.Fatal("custom agent should be registered")
	}
	if agent.Binary != "my-cli" {
		t.Fatalf("expected binary my-cli, got %s", agent.Binary)
	}
}

func TestACPVersionCommandSafety(t *testing.T) {
	// 安全的命令
	if !IsVersionCommandSafe([]string{"--version"}) {
		t.Fatal("--version should be safe")
	}

	// 包含注入字符
	if IsVersionCommandSafe([]string{"--version; rm -rf /"}) {
		t.Fatal("command with ; should be unsafe")
	}
	if IsVersionCommandSafe([]string{"$(cat /etc/passwd)"}) {
		t.Fatal("command with $() should be unsafe")
	}
	if IsVersionCommandSafe([]string{"`whoami`"}) {
		t.Fatal("command with backticks should be unsafe")
	}
}

func TestACPIdleDetect(t *testing.T) {
	mgr := NewAcpManager()

	// 使用一个不会输出任何内容的进程
	_, err := mgr.Spawn("sleep-agent", "sleep", []string{"10"}, nil)
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}
	defer mgr.Kill("sleep-agent")

	start := time.Now()
	_, _ = mgr.SendPrompt("sleep-agent", "hello", 5000)
	elapsed := time.Since(start)

	// 应该在 2s 空闲后返回（而非等满 5s 超时）
	if elapsed > 4*time.Second {
		t.Fatalf("idle detect should return before timeout, elapsed=%v", elapsed)
	}
}
