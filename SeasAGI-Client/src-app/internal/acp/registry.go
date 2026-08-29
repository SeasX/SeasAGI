package acp

import (
	"os/exec"
	"strings"
	"sync"
)

// AgentInfo 描述一个可探测的 CLI agent。
type AgentInfo struct {
	Name    string
	Binary  string
	Args    []string
	Version []string // 版本探测参数
}

// BuiltinAgents 内置支持的 CLI agent 列表。
var BuiltinAgents = []AgentInfo{
	{Name: "claude", Binary: "claude", Args: []string{}, Version: []string{"--version"}},
	{Name: "codex", Binary: "codex", Args: []string{}, Version: []string{"--version"}},
	{Name: "gemini", Binary: "gemini", Args: []string{}, Version: []string{"--version"}},
	{Name: "aider", Binary: "aider", Args: []string{}, Version: []string{"--version"}},
	{Name: "qwen", Binary: "qwen", Args: []string{}, Version: []string{"--version"}},
	{Name: "goose", Binary: "goose", Args: []string{}, Version: []string{"--version"}},
	{Name: "opencode", Binary: "opencode", Args: []string{}, Version: []string{"--version"}},
	{Name: "cline", Binary: "cline", Args: []string{}, Version: []string{"--version"}},
}

// DisallowedVersionChars 版本命令中不允许的字符（防注入）。
var DisallowedVersionChars = ";&|`$()\n\r"

// Registry 管理 CLI agent 的探测和注册。
type Registry struct {
	mu     sync.RWMutex
	agents map[string]AgentInfo
	cache  map[string]bool // binary → available
	cacheTTL int64         // unix seconds
}

// NewRegistry 创建注册表并加载内置 agent。
func NewRegistry() *Registry {
	r := &Registry{
		agents: make(map[string]AgentInfo),
		cache:  make(map[string]bool),
	}
	for _, agent := range BuiltinAgents {
		r.agents[agent.Name] = agent
	}
	return r
}

// Register 注册自定义 agent。
func (r *Registry) Register(agent AgentInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[agent.Name] = agent
}

// Get 获取 agent 信息。
func (r *Registry) Get(name string) (AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agents[name]
	return agent, ok
}

// List 返回所有已注册的 agent。
func (r *Registry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agents := make([]AgentInfo, 0, len(r.agents))
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	return agents
}

// IsVersionCommandSafe 检查版本命令是否安全（无注入字符）。
func IsVersionCommandSafe(cmd []string) bool {
	for _, part := range cmd {
		if strings.ContainsAny(part, DisallowedVersionChars) {
			return false
		}
	}
	return true
}

// DetectAvailable 探测哪些 CLI agent 可用（检查 binary 是否存在）。
func (r *Registry) DetectAvailable() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var available []string
	for name, agent := range r.agents {
		if _, err := exec.LookPath(agent.Binary); err == nil {
			available = append(available, name)
			r.cache[agent.Binary] = true
		}
	}
	return available
}

// IsAvailable 检查指定 agent 是否可用。
func (r *Registry) IsAvailable(name string) bool {
	r.mu.RLock()
	agent, ok := r.agents[name]
	r.mu.RUnlock()
	if !ok {
		return false
	}

	// 检查缓存
	r.mu.RLock()
	if cached, ok := r.cache[agent.Binary]; ok {
		r.mu.RUnlock()
		return cached
	}
	r.mu.RUnlock()

	// 实际探测
	_, err := exec.LookPath(agent.Binary)
	available := err == nil

	r.mu.Lock()
	r.cache[agent.Binary] = available
	r.mu.Unlock()

	return available
}
