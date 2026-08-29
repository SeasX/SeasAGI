package mitm

import "sync"

// Rules 管理需要拦截的域名规则集。线程安全。
type Rules struct {
	mu   sync.RWMutex
	set  map[string]bool
	list []string // 有序列表（供前端展示）
}

// NewRules 创建空规则集。
func NewRules() *Rules {
	return &Rules{
		set:  make(map[string]bool),
		list: []string{},
	}
}

// NewDefaultRules 创建预置 AI API 域名的规则集（含国际厂商、国产厂商、IDE 专属域名）。
func NewDefaultRules() *Rules {
	r := NewRules()
	for _, d := range []string{
		// 国际厂商 API
		"api.openai.com",
		"api.anthropic.com",
		"generativelanguage.googleapis.com",
		"api.deepseek.com",
		"api.x.ai",
		"openrouter.ai",

		// 国产厂商 API
		"open.bigmodel.cn",                    // 智谱 GLM
		"api.moonshot.cn",                     // 月之暗面 Kimi
		"api.minimax.chat",                    // MiniMax
		"api.xiaomimimo.com",                  // 小米 MiMo
		"ark.cn-beijing.volces.com",          // 字节豆包
		"api.hunyuan.cloud.tencent.com",       // 腾讯混元
		"dashscope.aliyuncs.com",             // 阿里通义/百炼
		"qianfan.baidubce.com",               // 百度文心
		"spark-api-open.xf-yun.com",           // 科大讯飞
		"api.coze.cn",                        // Coze
		"api.baichuan-ai.com",                // 百川
		"api.stepfun.com",                    // 阶跃星辰
		"api.sensenova.cn",                   // 商汤

		// IDE / 编程工具专属域名
		"api2.cursor.sh",                     // Cursor
		"api.githubcopilot.com",              // GitHub Copilot
		"api.zed.dev",                        // Zed
		"api.trae.ai",                        // Trae
	} {
		r.Add(d)
	}
	return r
}

// Match 检查 host 是否命中拦截规则（精确匹配）。
func (r *Rules) Match(host string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.set[host]
}

// Add 添加拦截域名。幂等。
func (r *Rules) Add(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.set[domain] {
		return
	}
	r.set[domain] = true
	r.list = append(r.list, domain)
}

// Remove 移除拦截域名。幂等。
func (r *Rules) Remove(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.set[domain] {
		return
	}
	delete(r.set, domain)
	for i, d := range r.list {
		if d == domain {
			r.list = append(r.list[:i], r.list[i+1:]...)
			break
		}
	}
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
