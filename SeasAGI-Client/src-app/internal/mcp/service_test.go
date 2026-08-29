package mcp

import (
	"os"
	"sync"
	"testing"
)

func setupTestDir(t *testing.T) (string, func()) {
	t.Helper()
	origHome := os.Getenv("HOME")
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	return tempDir, func() {
		os.Setenv("HOME", origHome)
	}
}

func TestNewService_CreatesEmptyService(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
	servers := svc.ListServers()
	if len(servers) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(servers))
	}
}

func TestSaveServer_AddsServer(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	server := MCPServer{
		Name:    "test-server",
		Command: "node",
		Args:    []string{"server.js"},
		Enabled: true,
		Apps:    []string{"claude"},
	}
	err := svc.SaveServer(server)
	if err != nil {
		t.Fatalf("SaveServer() returned error: %v", err)
	}

	servers := svc.ListServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if servers[0].Name != "test-server" {
		t.Errorf("expected Name 'test-server', got %q", servers[0].Name)
	}
	if servers[0].Command != "node" {
		t.Errorf("expected Command 'node', got %q", servers[0].Command)
	}
	if !servers[0].Enabled {
		t.Error("expected Enabled true")
	}
	if len(servers[0].Apps) != 1 || servers[0].Apps[0] != "claude" {
		t.Errorf("expected Apps ['claude'], got %v", servers[0].Apps)
	}
}

func TestSaveServer_UpdatesExisting(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	server := MCPServer{
		Name:    "test-server",
		Command: "node",
		Enabled: true,
		Apps:    []string{"claude"},
	}
	_ = svc.SaveServer(server)

	updated := MCPServer{
		Name:    "test-server",
		Command: "python",
		Args:    []string{"server.py"},
		Enabled: false,
		Apps:    []string{"codex"},
	}
	err := svc.SaveServer(updated)
	if err != nil {
		t.Fatalf("SaveServer() returned error: %v", err)
	}

	servers := svc.ListServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if servers[0].Command != "python" {
		t.Errorf("expected Command 'python', got %q", servers[0].Command)
	}
	if servers[0].Enabled {
		t.Error("expected Enabled false")
	}
	if len(servers[0].Apps) != 1 || servers[0].Apps[0] != "codex" {
		t.Errorf("expected Apps ['codex'], got %v", servers[0].Apps)
	}
}

func TestListServers_ReturnsAll(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	_ = svc.SaveServer(MCPServer{Name: "server1", Command: "node", Enabled: true, Apps: []string{"claude"}})
	_ = svc.SaveServer(MCPServer{Name: "server2", Command: "python", Enabled: true, Apps: []string{"codex"}})

	servers := svc.ListServers()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
}

func TestDeleteServer_RemovesServer(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	_ = svc.SaveServer(MCPServer{Name: "test-server", Command: "node", Enabled: true, Apps: []string{"claude"}})

	err := svc.DeleteServer("test-server")
	if err != nil {
		t.Fatalf("DeleteServer() returned error: %v", err)
	}

	servers := svc.ListServers()
	if len(servers) != 0 {
		t.Fatalf("expected 0 servers after delete, got %d", len(servers))
	}
}

func TestDeleteServer_UnknownName(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	err := svc.DeleteServer("unknown")
	if err != nil {
		t.Fatalf("DeleteServer() with unknown name should return nil, got: %v", err)
	}
}

func TestToggleServer_EnablesDisabled(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	_ = svc.SaveServer(MCPServer{Name: "test-server", Command: "node", Enabled: false, Apps: []string{"claude"}})

	err := svc.ToggleServer("test-server", true)
	if err != nil {
		t.Fatalf("ToggleServer() returned error: %v", err)
	}

	servers := svc.ListServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if !servers[0].Enabled {
		t.Error("expected server to be enabled after toggle")
	}
}

func TestToggleServer_DisablesEnabled(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	_ = svc.SaveServer(MCPServer{Name: "test-server", Command: "node", Enabled: true, Apps: []string{"claude"}})

	err := svc.ToggleServer("test-server", false)
	if err != nil {
		t.Fatalf("ToggleServer() returned error: %v", err)
	}

	servers := svc.ListServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if servers[0].Enabled {
		t.Error("expected server to be disabled after toggle")
	}
}

func TestToggleServer_UnknownName(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	err := svc.ToggleServer("unknown", true)
	if err != nil {
		t.Fatalf("ToggleServer() with unknown name should return nil, got: %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	svc := NewService()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = svc.ListServers()
			_ = svc.SaveServer(MCPServer{
				Name:    "concurrent-test",
				Command: "node",
				Enabled: true,
				Apps:    []string{"claude"},
			})
			_ = svc.ToggleServer("concurrent-test", i%2 == 0)
			_ = svc.DeleteServer("concurrent-test")
		}(i)
	}

	wg.Wait()
	// If we reach here without race condition or deadlock, the test passes
}