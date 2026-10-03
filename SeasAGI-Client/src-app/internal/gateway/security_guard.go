package gateway

import (
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/security"
)

// securityGuard 在网关主链路上执行内容治理：
//   - PII/DLP 脱敏：改写转发出站的请求 messages
//   - 提示注入检测：按策略告警（log）或拦截（block）
//   - 对外错误脱敏：剥离文件路径、堆栈、内网主机等内部信息
//
// 三个 matcher 均为只读可复用对象；开关策略每次从 config 读取，支持运行时热更新。
type securityGuard struct {
	dlp       *security.DLPMatcher
	injection *security.PromptInjectionDetector
	sanitizer *security.ErrorSanitizer
}

func newSecurityGuard() *securityGuard {
	return &securityGuard{
		dlp:       security.NewDLPMatcher(),
		injection: security.NewPromptInjectionDetector(),
		sanitizer: security.NewErrorSanitizer(true),
	}
}

// injectionFinding 是提示注入检测结果的精简视图。
type injectionFinding struct {
	Detected bool
	Severity string
	Pattern  string
}

// governRequestBody 对原始请求体执行内容治理：先按原始内容做注入检测，
// 再按需对 messages 做 PII/DLP 脱敏（原地改写 rawBody 中的 map，调用方
// 需在 hits>0 时重新序列化请求体）。返回注入检测结果与脱敏命中数。
func (g *securityGuard) governRequestBody(cfg config.SecurityConfig, rawBody map[string]any) (injectionFinding, int) {
	if g == nil || rawBody == nil {
		return injectionFinding{}, 0
	}
	rawMsgs, ok := rawBody["messages"].([]any)
	if !ok || len(rawMsgs) == 0 {
		return injectionFinding{}, 0
	}

	// rawBody 中的元素为 map[string]any，转换后与原 map 共享引用，可原地改写。
	msgs := make([]map[string]any, 0, len(rawMsgs))
	for _, m := range rawMsgs {
		if mm, ok := m.(map[string]any); ok {
			msgs = append(msgs, mm)
		}
	}

	finding := injectionFinding{}
	if cfg.PromptInjectionAction != config.PromptInjectionOff {
		if res := g.injection.CheckMessages(msgs); res.Detected {
			finding = injectionFinding{Detected: true, Severity: res.Severity, Pattern: res.Input}
		}
	}

	hits := 0
	if cfg.PIIMaskingEnabled {
		for _, m := range msgs {
			if content, ok := m["content"].(string); ok {
				hits += len(g.dlp.DetectAdvanced(content))
			}
		}
		if hits > 0 {
			g.dlp.MaskMessagesAdvanced(msgs)
		}
	}

	return finding, hits
}

// sanitizeError 按配置对外部错误信息脱敏；未开启时原样返回。
func (g *securityGuard) sanitizeError(cfg config.SecurityConfig, msg string) string {
	if g == nil || !cfg.ErrorSanitizeEnabled || msg == "" {
		return msg
	}
	return g.sanitizer.Sanitize(msg)
}

// sanitizeClientError 使用当前安全配置对将要返回给客户端的错误信息脱敏。
func (s *Service) sanitizeClientError(msg string) string {
	if s == nil || s.security == nil {
		return msg
	}
	return s.security.sanitizeError(s.configSvc.GetSecurityConfig(), msg)
}
