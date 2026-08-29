package sync

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
)

// ConfigBundle 配置同步包，包含多类配置。
type ConfigBundle struct {
	Settings        map[string]interface{} `json:"settings"`
	ProviderConns   []map[string]interface{} `json:"provider_connections"`
	ModelAliases    map[string]string `json:"model_aliases"`
	Combos          []map[string]interface{} `json:"combos"`
	APIKeys         []map[string]interface{} `json:"api_keys"`
	RoutingRules    []map[string]interface{} `json:"routing_rules"`
	Version         string `json:"version"` // SHA-256 确定性版本哈希
}

// BundleBuilder 构建配置同步包并计算版本哈希。
type BundleBuilder struct {
	mu sync.Mutex
}

// NewBundleBuilder 创建构建器。
func NewBundleBuilder() *BundleBuilder {
	return &BundleBuilder{}
}

// Build 构建配置同步包。
func (b *BundleBuilder) Build(
	settings map[string]interface{},
	providerConns []map[string]interface{},
	modelAliases map[string]string,
	combos []map[string]interface{},
	apiKeys []map[string]interface{},
	routingRules []map[string]interface{},
) (*ConfigBundle, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	bundle := &ConfigBundle{
		Settings:      settings,
		ProviderConns: providerConns,
		ModelAliases:  modelAliases,
		Combos:        combos,
		APIKeys:       apiKeys,
		RoutingRules:  routingRules,
	}

	// 计算确定性版本哈希
	data, err := serializeStableJSON(bundle)
	if err != nil {
		return nil, err
	}
	bundle.Version = ComputeVersionHash(data)

	return bundle, nil
}

// serializeStableJSON 序列化为确定性 JSON（键排序）。
func serializeStableJSON(bundle *ConfigBundle) ([]byte, error) {
	// 使用自定义序列化确保键排序
	data, err := json.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	// 重新解析并重新序列化以确保确定性
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return json.Marshal(stabilize(raw))
}

// stabilize 递归排序 map 键以确保确定性输出。
func stabilize(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		result := make(map[string]interface{}, len(v))
		for _, k := range keys {
			result[k] = stabilize(v[k])
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = stabilize(item)
		}
		return result
	default:
		return data
	}
}

// ConflictResult 引用完整性冲突检测结果。
type ConflictResult struct {
	Type       string // "combo_channel_missing" / "alias_model_missing"
	Conflicts  []string
}

// DetectConflicts 检测配置包中的引用完整性冲突。
func DetectConflicts(bundle *ConfigBundle) []ConflictResult {
	var results []ConflictResult

	// 检测 combo 引用的 channel 是否存在
	channelIDs := make(map[string]bool)
	for _, conn := range bundle.ProviderConns {
		if id, ok := conn["channel_id"].(string); ok {
			channelIDs[id] = true
		}
	}

	var comboConflicts []string
	for _, combo := range bundle.Combos {
		if steps, ok := combo["steps"].([]interface{}); ok {
			for _, step := range steps {
				if m, ok := step.(map[string]interface{}); ok {
					if chID, ok := m["channel_id"].(string); ok && chID != "" {
						if !channelIDs[chID] {
							comboConflicts = append(comboConflicts, chID)
						}
					}
				}
			}
		}
	}
	if len(comboConflicts) > 0 {
		results = append(results, ConflictResult{
			Type:      "combo_channel_missing",
			Conflicts: comboConflicts,
		})
	}

	// 检测 alias 引用的模型是否存在
	modelSet := make(map[string]bool)
	for _, conn := range bundle.ProviderConns {
		if models, ok := conn["models"].([]interface{}); ok {
			for _, m := range models {
				if name, ok := m.(string); ok {
					modelSet[name] = true
				}
			}
		}
	}

	var aliasConflicts []string
	for alias, target := range bundle.ModelAliases {
		_ = alias
		if !modelSet[target] {
			aliasConflicts = append(aliasConflicts, target)
		}
	}
	if len(aliasConflicts) > 0 {
		results = append(results, ConflictResult{
			Type:      "alias_model_missing",
			Conflicts: aliasConflicts,
		})
	}

	return results
}

// SerializeBundle 序列化配置包为 JSON 字符串。
func SerializeBundle(bundle *ConfigBundle) (string, error) {
	data, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseBundle 从 JSON 字符串解析配置包。
func ParseBundle(data string) (*ConfigBundle, error) {
	var bundle ConfigBundle
	if err := json.Unmarshal([]byte(data), &bundle); err != nil {
		return nil, err
	}
	return &bundle, nil
}

// HasConflicts 快速检查是否有冲突。
func HasConflicts(conflicts []ConflictResult) bool {
	for _, c := range conflicts {
		if len(c.Conflicts) > 0 {
			return true
		}
	}
	return false
}

// FormatConflicts 格式化冲突信息为字符串。
func FormatConflicts(conflicts []ConflictResult) string {
	var sb strings.Builder
	for _, c := range conflicts {
		sb.WriteString(c.Type + ": " + strings.Join(c.Conflicts, ", ") + "\n")
	}
	return sb.String()
}
