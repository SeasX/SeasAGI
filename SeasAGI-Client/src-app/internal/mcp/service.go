package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type MCPServer struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Transport   string            `json:"transport"`
	URL         string            `json:"url,omitempty"`
	Enabled     bool              `json:"enabled"`
	Apps        []string          `json:"apps"`
	Description string            `json:"description,omitempty"`
}

type Service struct {
	mu      sync.RWMutex
	servers []MCPServer
	path    string
}

func NewService() *Service {
	homeDir, _ := os.UserHomeDir()
	path := filepath.Join(homeDir, ".seasagi", "mcp_servers.json")
	svc := &Service{
		servers: make([]MCPServer, 0),
		path:    path,
	}
	svc.load()
	return svc
}

func (s *Service) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var loaded []MCPServer
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	s.servers = loaded
}

func (s *Service) persist() {
	data, err := json.MarshalIndent(s.servers, "", "  ")
	if err != nil {
		return
	}
	// env 中可能包含 API_KEY：目录 0700、文件 0600，仅当前用户可读（CL-S8）。
	os.MkdirAll(filepath.Dir(s.path), 0700)
	_ = os.Chmod(filepath.Dir(s.path), 0700)
	tmpFile := s.path + ".tmp"
	_ = os.WriteFile(tmpFile, data, 0600)
	_ = os.Chmod(tmpFile, 0600)
	_ = os.Rename(tmpFile, s.path)
}

func (s *Service) ListServers() []MCPServer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]MCPServer, len(s.servers))
	copy(result, s.servers)
	return result
}

func (s *Service) SaveServer(server MCPServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.servers {
		if existing.Name == server.Name {
			s.servers[i] = server
			s.persist()
			s.syncToApps()
			return nil
		}
	}
	s.servers = append(s.servers, server)
	s.persist()
	s.syncToApps()
	return nil
}

func (s *Service) DeleteServer(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.servers {
		if existing.Name == name {
			s.servers = append(s.servers[:i], s.servers[i+1:]...)
			s.persist()
			s.syncToApps()
			return nil
		}
	}
	return nil
}

func (s *Service) ToggleServer(name string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.servers {
		if existing.Name == name {
			s.servers[i].Enabled = enabled
			s.persist()
			s.syncToApps()
			return nil
		}
	}
	return nil
}

func (s *Service) syncToApps() {
	homeDir, _ := os.UserHomeDir()
	appConfigs := map[string]func() string{
		"claude": func() string { return filepath.Join(homeDir, ".claude", "settings.json") },
		"codex":  func() string { return filepath.Join(homeDir, ".codex", "config.toml") },
		"gemini": func() string { return filepath.Join(homeDir, ".gemini", ".env") },
	}

	for appName, configPath := range appConfigs {
		var relevantServers []MCPServer
		for _, srv := range s.servers {
			if !srv.Enabled {
				continue
			}
			for _, app := range srv.Apps {
				if app == appName || app == "all" {
					relevantServers = append(relevantServers, srv)
					break
				}
			}
		}
		_ = s.writeAppConfig(appName, configPath(), relevantServers)
	}
}

func (s *Service) writeAppConfig(appName, configPath string, servers []MCPServer) error {
	if appName == "claude" {
		return s.writeClaudeConfig(configPath, servers)
	}
	return nil
}

func (s *Service) writeClaudeConfig(configPath string, servers []MCPServer) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		data = []byte("{}")
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		config = make(map[string]interface{})
	}

	// 合并而非覆盖用户既有 mcpServers，避免清掉其它客户端配置（CL-S8）。
	mcpServers := make(map[string]interface{})
	if existing, ok := config["mcpServers"].(map[string]interface{}); ok {
		for name, entry := range existing {
			mcpServers[name] = entry
		}
	}
	for _, srv := range servers {
		entry := map[string]interface{}{
			"command": srv.Command,
		}
		if len(srv.Args) > 0 {
			entry["args"] = srv.Args
		}
		if len(srv.Env) > 0 {
			entry["env"] = srv.Env
		}
		mcpServers[srv.Name] = entry
	}
	config["mcpServers"] = mcpServers

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	// env 中可能包含 API_KEY：目录 0700、文件 0600，仅当前用户可读。
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(configPath, out, 0600); err != nil {
		return err
	}
	_ = os.Chmod(configPath, 0600)
	return nil
}
