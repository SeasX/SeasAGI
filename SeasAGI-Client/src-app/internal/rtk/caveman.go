package rtk

import (
	"fmt"
	"strings"
)

type CavemanStyle string

const (
	CavemanStyleConcise  CavemanStyle = "concise"
	CavemanStyleBrief    CavemanStyle = "brief"
	CavemanStyleMinimal  CavemanStyle = "minimal"
	CavemanStyleTerse    CavemanStyle = "terse"
)

var cavemanPrompts = map[CavemanStyle]string{
	CavemanStyleConcise: "Be concise. Use fewer words. Avoid unnecessary explanations. Give direct answers.",
	CavemanStyleBrief:   "Be brief. Skip preamble. Get to the point. Use short sentences.",
	CavemanStyleMinimal: "Minimal output. No filler. No repetition. Shortest possible answer unless detail is requested.",
	CavemanStyleTerse:   "Terse. One-line answers preferred. No elaboration unless asked. Zero fluff.",
}

type CavemanInjector struct {
	Enabled bool
	Style   CavemanStyle
}

func NewCavemanInjector(enabled bool, style string) *CavemanInjector {
	cs := CavemanStyle(style)
	if _, ok := cavemanPrompts[cs]; !ok {
		cs = CavemanStyleConcise
	}
	return &CavemanInjector{
		Enabled: enabled,
		Style:   cs,
	}
}

func (ci *CavemanInjector) Inject(messages []map[string]any) []map[string]any {
	if !ci.Enabled {
		return messages
	}

	prompt, ok := cavemanPrompts[ci.Style]
	if !ok {
		prompt = cavemanPrompts[CavemanStyleConcise]
	}

	hasSystem := false
	for i, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" {
			hasSystem = true
			content, ok := msg["content"].(string)
			// 非字符串 content（如多模态数组）原样保留，不做覆盖
			if ok &&
				!strings.Contains(content, "Be concise") &&
				!strings.Contains(content, "Be brief") &&
				!strings.Contains(content, "Minimal output") &&
				!strings.Contains(content, "Terse") {
				messages[i]["content"] = content + "\n\n" + prompt
			}
			return messages
		}
	}

	if !hasSystem {
		sysMsg := map[string]any{
			"role":    "system",
			"content": prompt,
		}
		messages = append([]map[string]any{sysMsg}, messages...)
	}

	return messages
}

func (ci *CavemanInjector) GetPrompt() string {
	if prompt, ok := cavemanPrompts[ci.Style]; ok {
		return prompt
	}
	return cavemanPrompts[CavemanStyleConcise]
}

func (ci *CavemanInjector) SetStyle(style string) {
	cs := CavemanStyle(style)
	if _, ok := cavemanPrompts[cs]; ok {
		ci.Style = cs
	}
}

func ListCavemanStyles() []map[string]string {
	result := make([]map[string]string, 0, len(cavemanPrompts))
	for k, v := range cavemanPrompts {
		result = append(result, map[string]string{
			"id":     string(k),
			"name":   string(k),
			"prompt": v,
		})
	}
	return result
}

// IsValidCavemanStyle 判断风格 ID 是否为受支持的四风格之一。
// 供设置层校验持久化值，避免非法风格在注入时静默降级为 concise。
func IsValidCavemanStyle(style string) bool {
	_, ok := cavemanPrompts[CavemanStyle(style)]
	return ok
}

func CavemanInjectIntoCanonical(messages []map[string]any, enabled bool, style string) []map[string]any {
	injector := NewCavemanInjector(enabled, style)
	return injector.Inject(messages)
}

func FormatCavemanStatus(enabled bool, style CavemanStyle) string {
	if !enabled {
		return "caveman: off"
	}
	prompt, ok := cavemanPrompts[style]
	if !ok {
		prompt = cavemanPrompts[CavemanStyleConcise]
	}
	if len(prompt) > 50 {
		return fmt.Sprintf("caveman: %s — %s...", style, prompt[:50])
	}
	return fmt.Sprintf("caveman: %s — %s", style, prompt)
}
