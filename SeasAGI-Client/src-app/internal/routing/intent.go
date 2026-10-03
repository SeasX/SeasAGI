package routing

import (
	"strings"
)

// IntentContext 是意图协同引擎的统一意图表征（v0.2.0，场景 + 属性矩阵）。
// ponytail: v0.2.0-lite 由纯启发式直接产出（<1ms）；计划中的 go-llama 内嵌
// 模型 Slow Path 暂缓，待启发式出现实测误路由再接入，本结构即为两者共用契约。
type IntentContext struct {
	TaskType      string   `json:"task_type"`      // 兼容既有 task_detect 结果
	Scenario      string   `json:"scenario"`       // code_logic / image_gen / research_audit / data_extract / creative / casual
	RequiredIQ    string   `json:"required_iq"`    // high / balanced / low
	SecurityLevel string   `json:"security_level"` // public / sensitive（sensitive 强制 local_only）
	Tags          []string `json:"tags,omitempty"` // math / react / sql / svg 等
	Confidence    float64  `json:"confidence"`     // 启发式命中置信度 0.0-1.0
}

// scenarioRules 按优先级排列：先命中先赢（image_gen > code_logic > research > extract > creative）。
var scenarioRules = []struct {
	scenario string
	keywords []string
}{
	{"image_gen", []string{"draw ", "draw a", "painting", "generate an image", "image of", "picture of", "illustration", "poster", "logo for", "midjourney", "stable diffusion", "dall-e", "画一", "画个", "画只", "生成一张图", "生成图片", "帮我画"}},
	{"code_logic", []string{"write a function", "write a class", "refactor", "debug", "compile error", "stack trace", "unit test", "implement", "code review", "endpoint", "algorithm", "regex", "in go", "in python", "in rust", "typescript", "javascript", "sql query", "deploy", "docker", "写代码", "写一个函数", "实现一个", "修复", "报错", "重构"}},
	{"research_audit", []string{"summarize", "summary of", "analyze this", "audit", "literature", "citation", "review this", "research", "compare the", "总结", "分析一下", "检索", "审阅"}},
	{"data_extract", []string{"extract", "convert to json", "convert to csv", "parse the", "tabulate", "提取", "转成", "导出为"}},
	{"creative", []string{"poem", "poetry", "story", "novel", "song", "lyrics", "screenplay", "write a blog", "slogan", "写诗", "写一首", "小说", "歌词", "文案"}},
}

var highIQSignals = []string{"high-performance", "optimize", "parallel", "concurrent", "thorough", "complex", "architecture", "derive", "prove", "scalable", "分布式", "高并发", "性能优化"}

var sensitiveSignals = []string{"api key", "apikey", "password", "passwd", "private key", "begin rsa", "begin openssh", "bearer ", "ssn", "credit card", "身份证", "密码", "密钥", "口令", "私钥"}

var tagRules = []struct {
	tag      string
	keywords []string
}{
	{"math", []string{"prove", "equation", "integral", "derivative", "calculate the", "数学", "推导", "证明"}},
	{"react", []string{"react"}},
	{"sql", []string{"sql"}},
	{"svg", []string{"svg"}},
}

// DetectIntent 从消息文本中提取场景化意图（Fast Path，纯启发式）。
func DetectIntent(messages []map[string]interface{}) *IntentContext {
	intent := &IntentContext{
		TaskType:      DetectTaskType(messages),
		Scenario:      "casual",
		RequiredIQ:    "balanced",
		SecurityLevel: "public",
		Confidence:    0.5,
	}
	if len(messages) == 0 {
		return intent
	}

	text, _, _ := messageScanText(messages)
	content := strings.ToLower(text)

	// 场景识别：先命中先赢
	for _, rule := range scenarioRules {
		hits := 0
		for _, kw := range rule.keywords {
			if strings.Contains(content, kw) {
				hits++
			}
		}
		if hits > 0 {
			intent.Scenario = rule.scenario
			intent.Confidence = 0.6 + float64(hits-1)*0.1
			if intent.Confidence > 0.9 {
				intent.Confidence = 0.9
			}
			break
		}
	}

	// 智商要求：复杂编程/推理信号或超长 prompt → high；短闲聊 → low
	hasHighIQSignal := false
	for _, kw := range highIQSignals {
		if strings.Contains(content, kw) {
			hasHighIQSignal = true
			break
		}
	}
	switch {
	case (intent.Scenario == "code_logic" || intent.Scenario == "research_audit") && (hasHighIQSignal || len(content) > 600):
		intent.RequiredIQ = "high"
	case intent.Scenario == "casual" && len(content) < 120:
		intent.RequiredIQ = "low"
	}

	// 安全等级：出现敏感凭据/PII 信号 → sensitive（路由层强制 local_only）
	for _, kw := range sensitiveSignals {
		if strings.Contains(content, kw) {
			intent.SecurityLevel = "sensitive"
			break
		}
	}

	// 动态标签
	for _, rule := range tagRules {
		for _, kw := range rule.keywords {
			if strings.Contains(content, kw) {
				intent.Tags = append(intent.Tags, rule.tag)
				break
			}
		}
	}

	return intent
}
