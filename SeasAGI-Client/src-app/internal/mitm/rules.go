package mitm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Rules 管理需要拦截的域名规则集。线程安全。
//
// 生效域名 = 基线域名（厂商 API + 受支持目标的 Hosts）∪ 用户新增 − 用户移除。
// 用户的增删改动可通过 SetStorePath + Save/Load 持久化到本地文件，重启后仍然生效；
// 基线域名每次启动重新派生，保证新增/企业下发的受支持目标自动被接管。
type Rules struct {
	mu   sync.RWMutex
	path string // 持久化文件路径，空串表示不持久化

	base    []string        // 基线域名（有序）
	added   []string        // 用户新增（有序）
	removed map[string]bool // 用户移除（覆盖基线）

	set  map[string]bool // 派生：实际生效集合
	list []string        // 派生：实际生效有序列表
}

// persistPayload 是持久化文件的内容：仅记录用户改动，而非完整清单。
type persistPayload struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// vendorAPIDomains 厂商大模型 API 域名（非 IDE 专属）。
var vendorAPIDomains = []string{
	// 国际厂商 API
	"api.openai.com",
	"api.anthropic.com",
	"generativelanguage.googleapis.com",
	"api.deepseek.com",
	"api.x.ai",
	"openrouter.ai",

	// 国产厂商 API
	"open.bigmodel.cn",              // 智谱 GLM
	"api.moonshot.cn",               // 月之暗面 Kimi
	"api.minimax.chat",              // MiniMax
	"api.xiaomimimo.com",            // 小米 MiMo
	"ark.cn-beijing.volces.com",     // 字节豆包
	"api.hunyuan.cloud.tencent.com", // 腾讯混元
	"dashscope.aliyuncs.com",        // 阿里通义/百炼
	"qianfan.baidubce.com",          // 百度文心
	"spark-api-open.xf-yun.com",     // 科大讯飞
	"api.coze.cn",                   // Coze
	"api.baichuan-ai.com",           // 百川
	"api.stepfun.com",               // 阶跃星辰
	"api.sensenova.cn",              // 商汤
}

// defaultRuleDomains 默认拦截域名列表。
// 由「厂商 API 域名」∪「viability=supported 的 IDE 目标 Hosts」派生，
// 保证前端展示为「已支持」的目标一定落在拦截清单内（展示与生效一致）。
var defaultRuleDomains = DefaultBaseDomains()

// DefaultBaseDomains 返回当前应始终拦截的基线域名：
// 厂商 API 域名 + 所有 viability=supported 目标（含企业动态下发）的 Hosts。
func DefaultBaseDomains() []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(vendorAPIDomains)+len(AllTargets))
	add := func(d string) {
		n := normalizeDomain(d)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	for _, d := range vendorAPIDomains {
		add(d)
	}
	for _, t := range MergedTargets() {
		if t.Viability != "supported" {
			continue
		}
		for _, h := range t.Hosts {
			add(h)
		}
	}
	return out
}

// normalizeDomain 归一化域名：去首尾空白 + 转小写。
// 拦截匹配一律使用归一化后的值，避免大小写/空白导致的漏拦截。
func normalizeDomain(d string) string {
	return strings.ToLower(strings.TrimSpace(d))
}

// NewRules 创建空规则集。
func NewRules() *Rules {
	r := &Rules{removed: map[string]bool{}}
	r.rebuild()
	return r
}

// NewRulesWithBase 创建以指定域名作为基线的规则集。
func NewRulesWithBase(base []string) *Rules {
	r := NewRules()
	r.SetBase(base)
	return r
}

// NewDefaultRules 创建预置 AI API 域名的规则集。
func NewDefaultRules() *Rules {
	return NewRulesWithBase(defaultRuleDomains)
}

// SetBase 设置基线域名，保留用户既有的增删改动。
func (r *Rules) SetBase(domains []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.base = make([]string, 0, len(domains))
	for _, d := range domains {
		if n := normalizeDomain(d); n != "" {
			r.base = append(r.base, n)
		}
	}
	r.rebuild()
}

// SetStorePath 设置持久化文件路径。设为空串表示不持久化。
func (r *Rules) SetStorePath(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = path
}

// Load 从持久化文件恢复用户的增删改动。文件不存在时视为无自定义改动。
func (r *Rules) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path == "" {
		return nil
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read rules file: %w", err)
	}
	var p persistPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("parse rules file: %w", err)
	}
	r.added = nil
	for _, d := range p.Added {
		r.added = appendUnique(r.added, normalizeDomain(d))
	}
	r.removed = make(map[string]bool, len(p.Removed))
	for _, d := range p.Removed {
		if n := normalizeDomain(d); n != "" {
			r.removed[n] = true
		}
	}
	r.rebuild()
	return nil
}

// Save 将用户改动写入持久化文件。未设置路径时为 no-op。
func (r *Rules) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return fmt.Errorf("mkdir rules dir: %w", err)
	}
	removed := make([]string, 0, len(r.removed))
	for d := range r.removed {
		removed = append(removed, d)
	}
	sort.Strings(removed)
	data, err := json.MarshalIndent(persistPayload{Added: r.added, Removed: removed}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	if err := os.WriteFile(r.path, data, 0o600); err != nil {
		return fmt.Errorf("write rules file: %w", err)
	}
	return nil
}

// Match 检查 host 是否命中拦截规则（归一化后精确匹配）。
func (r *Rules) Match(host string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.set[normalizeDomain(host)]
}

// Add 添加拦截域名（用户新增）。幂等。
func (r *Rules) Add(domain string) {
	n := normalizeDomain(domain)
	if n == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.removed, n)
	r.added = appendUnique(r.added, n)
	r.rebuild()
}

// Remove 移除拦截域名（用户移除，会覆盖基线）。幂等。
func (r *Rules) Remove(domain string) {
	n := normalizeDomain(domain)
	if n == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.added = removeString(r.added, n)
	r.removed[n] = true
	r.rebuild()
}

// List 返回有序域名列表副本。
func (r *Rules) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, len(r.list))
	copy(result, r.list)
	return result
}

// Count 返回当前规则数量。
func (r *Rules) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.list)
}

// rebuild 依据 base/added/removed 重算 set 与 list。调用方需持锁。
func (r *Rules) rebuild() {
	set := make(map[string]bool, len(r.base)+len(r.added))
	list := make([]string, 0, len(r.base)+len(r.added))
	appendDomain := func(d string) {
		n := normalizeDomain(d)
		if n == "" || r.removed[n] || set[n] {
			return
		}
		set[n] = true
		list = append(list, n)
	}
	for _, d := range r.base {
		appendDomain(d)
	}
	for _, d := range r.added {
		appendDomain(d)
	}
	r.set = set
	r.list = list
}

func appendUnique(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}

func removeString(list []string, v string) []string {
	for i, e := range list {
		if e == v {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
}
