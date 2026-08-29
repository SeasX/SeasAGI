package mitm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TargetDescriptor 声明式 MITM 目标描述符。
type TargetDescriptor struct {
	ID               string
	Name             string
	Icon             string
	Color            string
	Hosts            []string
	Port             int
	EndpointPatterns []string
	DefaultModels    []TargetModel
	Viability        string // "supported" / "investigating" / "deprecated"
}

// TargetModel 目标支持的默认模型。
type TargetModel struct {
	ID    string
	Name  string
	Alias string
}

// ZedTarget Zed IDE MITM 目标。
var ZedTarget = TargetDescriptor{
	ID:               "zed",
	Name:             "Zed",
	Icon:             "bolt",
	Color:            "#EF4444",
	Hosts:            []string{"api.zed.dev"},
	Port:             443,
	EndpointPatterns: []string{"/v1/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
		{ID: "gpt-5.4", Name: "GPT-5.4", Alias: "gpt-5.4"},
	},
	Viability: "supported",
}

// CursorTarget Cursor IDE MITM 目标。
var CursorTarget = TargetDescriptor{
	ID:               "cursor",
	Name:             "Cursor",
	Icon:             "cursor",
	Color:            "#000000",
	Hosts:            []string{"api2.cursor.sh"},
	Port:             443,
	EndpointPatterns: []string{"/copilot/chat/completions", "/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
		{ID: "gpt-5.4", Name: "GPT-5.4", Alias: "gpt-5.4"},
	},
	Viability: "supported",
}

// CodexTarget Codex CLI MITM 目标。
var CodexTarget = TargetDescriptor{
	ID:               "codex",
	Name:             "Codex",
	Icon:             "terminal",
	Color:            "#10A37F",
	Hosts:            []string{"api.openai.com"},
	Port:             443,
	EndpointPatterns: []string{"/v1/responses", "/v1/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "gpt-5.4", Name: "GPT-5.4", Alias: "gpt-5.4"},
		{ID: "gpt-5.3-codex", Name: "GPT-5.3 Codex", Alias: "gpt-5.3-codex"},
	},
	Viability: "supported",
}

// CopilotTarget GitHub Copilot MITM 目标。
var CopilotTarget = TargetDescriptor{
	ID:               "copilot",
	Name:             "GitHub Copilot",
	Icon:             "copilot",
	Color:            "#24292E",
	Hosts:            []string{"api.githubcopilot.com"},
	Port:             443,
	EndpointPatterns: []string{"/chat/completions", "/v1/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "gpt-5.4", Name: "GPT-5.4", Alias: "gpt-5.4"},
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
	},
	Viability: "supported",
}

// ClaudeCodeTarget Claude Code MITM 目标。
var ClaudeCodeTarget = TargetDescriptor{
	ID:               "claude-code",
	Name:             "Claude Code",
	Icon:             "code",
	Color:            "#D97757",
	Hosts:            []string{"api.anthropic.com"},
	Port:             443,
	EndpointPatterns: []string{"/v1/messages"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
		{ID: "claude-opus-4.8", Name: "Claude Opus 4.8", Alias: "claude-opus-4.8"},
	},
	Viability: "supported",
}

// TraeTarget Trae IDE MITM 目标。
var TraeTarget = TargetDescriptor{
	ID:               "trae",
	Name:             "Trae",
	Icon:             "trae",
	Color:            "#7C3AED",
	Hosts:            []string{"api.trae.ai"},
	Port:             443,
	EndpointPatterns: []string{"/v1/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini", Alias: "gpt-5.4-mini"},
	},
	Viability: "supported",
}

// AllTargets 所有 MITM 目标列表（有序）。
var AllTargets = []TargetDescriptor{
	CursorTarget,
	CopilotTarget,
	CodexTarget,
	ZedTarget,
	ClaudeCodeTarget,
	TraeTarget,
	WindsurfTarget,
	ContinueTarget,
	ClineTarget,
	AugmentTarget,
	CodeiumTarget,
	CodyTarget,
}

// WindsurfTarget Windsurf (Codeium) MITM 目标。
var WindsurfTarget = TargetDescriptor{
	ID:               "windsurf",
	Name:             "Windsurf",
	Icon:             "wind",
	Color:            "#3B82F6",
	Hosts:            []string{"windsurf.server.codeium.com"},
	Port:             443,
	EndpointPatterns: []string{"/v1/chat/completions", "/api/chat"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
		{ID: "gpt-5.4", Name: "GPT-5.4", Alias: "gpt-5.4"},
	},
	Viability: "supported",
}

// ContinueTarget Continue.dev MITM 目标。
var ContinueTarget = TargetDescriptor{
	ID:               "continue",
	Name:             "Continue",
	Icon:             "play",
	Color:            "#10B981",
	Hosts:            []string{"api.continue.dev"},
	Port:             443,
	EndpointPatterns: []string{"/v1/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini", Alias: "gpt-5.4-mini"},
	},
	Viability: "supported",
}

// ClineTarget Cline MITM 目标。
var ClineTarget = TargetDescriptor{
	ID:               "cline",
	Name:             "Cline",
	Icon:             "terminal",
	Color:            "#8B5CF6",
	Hosts:            []string{"api.cline.bot"},
	Port:             443,
	EndpointPatterns: []string{"/v1/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
	},
	Viability: "supported",
}

// AugmentTarget Augment Code MITM 目标。
var AugmentTarget = TargetDescriptor{
	ID:               "augment",
	Name:             "Augment",
	Icon:             "zap",
	Color:            "#F59E0B",
	Hosts:            []string{"api.augmentcode.com"},
	Port:             443,
	EndpointPatterns: []string{"/v1/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
	},
	Viability: "investigating",
}

// CodeiumTarget Codeium MITM 目标。
var CodeiumTarget = TargetDescriptor{
	ID:               "codeium",
	Name:             "Codeium",
	Icon:             "code",
	Color:            "#6366F1",
	Hosts:            []string{"server.codeium.com"},
	Port:             443,
	EndpointPatterns: []string{"/v1/chat/completions", "/api/completion"},
	DefaultModels: []TargetModel{
		{ID: "gpt-5.4", Name: "GPT-5.4", Alias: "gpt-5.4"},
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
	},
	Viability: "investigating",
}

// CodyTarget Sourcegraph Cody MITM 目标。
var CodyTarget = TargetDescriptor{
	ID:               "cody",
	Name:             "Sourcegraph Cody",
	Icon:             "search",
	Color:            "#EC4899",
	Hosts:            []string{"api.sourcegraph.com"},
	Port:             443,
	EndpointPatterns: []string{"/.api/graphql", "/v1/chat/completions"},
	DefaultModels: []TargetModel{
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "claude-sonnet-5"},
	},
	Viability: "investigating",
}

// ResolveTarget 根据 hostname 查找目标（大小写不敏感精确匹配）。
func ResolveTarget(hostname string) *TargetDescriptor {
	if hostname == "" {
		return nil
	}
	h := strings.ToLower(hostname)
	merged := MergedTargets()
	for i := range merged {
		for _, host := range merged[i].Hosts {
			if strings.ToLower(host) == h {
				return &merged[i]
			}
		}
	}
	return nil
}

// ConnectionRoute 路由决策。
type ConnectionRoute struct {
	Kind   string // "bypass" / "target" / "passthrough"
	Target *TargetDescriptor
}

// RouteConnection 决定 CONNECT/TLS 连接的路由。
// 优先级：bypass list > known target host > passthrough。
func RouteConnection(hostname string, bypassList []string) ConnectionRoute {
	for _, b := range bypassList {
		if strings.EqualFold(b, hostname) {
			return ConnectionRoute{Kind: "bypass"}
		}
	}

	target := ResolveTarget(hostname)
	if target != nil {
		return ConnectionRoute{Kind: "target", Target: target}
	}

	return ConnectionRoute{Kind: "passthrough"}
}

// GetTargetByID 根据 ID 获取目标。
func GetTargetByID(id string) *TargetDescriptor {
	merged := MergedTargets()
	for i := range merged {
		if merged[i].ID == id {
			return &merged[i]
		}
	}
	return nil
}

// IsEndpointMatch 检查请求路径是否匹配目标的 endpoint patterns。
func IsEndpointMatch(target *TargetDescriptor, path string) bool {
	for _, pattern := range target.EndpointPatterns {
		if strings.HasPrefix(path, pattern) {
			return true
		}
	}
	return false
}

// ---- 动态目标预设（7.3.4）----

var (
	dynamicTargets   []TargetDescriptor
	dynamicTargetsMu sync.Mutex
)

const mitmCacheFile = ".mitm-targets-cache.json"

// FetchTargetsFromEnterprise 从企业服务端动态拉取 MITM 目标列表。
// GET {ENTERPRISE_API_BASE_URL}/api/v1/mitm/targets
// 与本地 AllTargets 合并，企业配置优先（同 ID 覆盖）。
// 离线时使用本地缓存 fallback。
func FetchTargetsFromEnterprise(ctx context.Context, bearerToken string) error {
	enterpriseURL := os.Getenv("ENTERPRISE_API_BASE_URL")
	if enterpriseURL == "" {
		return nil // 未配置企业服务端，使用本地预设
	}

	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, enterpriseURL+"/mitm/targets", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// 网络失败，尝试加载缓存
		if cached := loadCachedTargets(); cached != nil {
			setDynamicTargets(cached)
			return nil
		}
		return fmt.Errorf("fetch targets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		if cached := loadCachedTargets(); cached != nil {
			setDynamicTargets(cached)
			return nil
		}
		return fmt.Errorf("enterprise API returned %d", resp.StatusCode)
	}

	var result struct {
		Data []TargetDescriptor `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// 缓存到本地
	saveCachedTargets(result.Data)
	setDynamicTargets(result.Data)
	return nil
}

func setDynamicTargets(targets []TargetDescriptor) {
	dynamicTargetsMu.Lock()
	dynamicTargets = targets
	dynamicTargetsMu.Unlock()
}

// MergedTargets 返回本地预设与动态拉取目标的合并列表。
// 动态目标按 ID 覆盖本地同 ID 目标。
func MergedTargets() []TargetDescriptor {
	dynamicTargetsMu.Lock()
	dynamic := dynamicTargets
	dynamicTargetsMu.Unlock()

	if len(dynamic) == 0 {
		return AllTargets
	}

	// 构建 ID 索引
	dynamicIDs := make(map[string]bool, len(dynamic))
	for _, t := range dynamic {
		dynamicIDs[t.ID] = true
	}

	merged := make([]TargetDescriptor, 0, len(AllTargets)+len(dynamic))
	// 本地目标中未被动态覆盖的保留
	for _, t := range AllTargets {
		if !dynamicIDs[t.ID] {
			merged = append(merged, t)
		}
	}
	// 追加动态目标
	merged = append(merged, dynamic...)
	return merged
}

func loadCachedTargets() []TargetDescriptor {
	cachePath := getCachePath()
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}
	var result struct {
		Data []TargetDescriptor `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result.Data
}

func saveCachedTargets(targets []TargetDescriptor) {
	cachePath := getCachePath()
	data, err := json.Marshal(struct {
		Data []TargetDescriptor `json:"data"`
	}{Data: targets})
	if err != nil {
		return
	}
	_ = os.WriteFile(cachePath, data, 0600)
}

func getCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return mitmCacheFile
	}
	return filepath.Join(home, ".seasagi", mitmCacheFile)
}
