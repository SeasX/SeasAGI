package compression

import (
	"strings"
)

// CavemanEngine 规则式散文压缩
type CavemanEngine struct{}

func (e *CavemanEngine) Name() EngineName { return EngineCaveman }

func (e *CavemanEngine) Compress(messages []map[string]any, cfg CompressionConfig) ([]map[string]any, CompressionResult, error) {
	originalTokens := estimateTokens(messages)

	for i, msg := range messages {
		role, _ := msg["role"].(string)
		// 只压缩 user 和 assistant 的散文内容，不压缩 system
		if role == "system" {
			continue
		}
		if content, ok := msg["content"].(string); ok {
			msg["content"] = cavemanCompress(content)
		} else if parts, ok := msg["content"].([]any); ok {
			msg["content"] = cavemanCompressParts(parts)
		}
		messages[i] = msg
	}

	compressedTokens := estimateTokens(messages)
	return messages, CompressionResult{
		OriginalTokens:   originalTokens,
		CompressedTokens: compressedTokens,
		SavingsPct:       savingsPct(originalTokens, compressedTokens),
		Engine:           EngineCaveman,
	}, nil
}

// cavemanCompress 规则式压缩散文
func cavemanCompress(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 代码块保护
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			result = append(result, line)
			continue
		}
		if inCodeBlock {
			result = append(result, line)
			continue
		}

		// 空行跳过
		if trimmed == "" {
			continue
		}

		// 移除填充词
		trimmed = removeFillerWords(trimmed)
		// 压缩重复标点
		trimmed = compressPunctuation(trimmed)

		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return strings.Join(result, "\n")
}

// cavemanCompressParts 压缩 content parts
func cavemanCompressParts(parts []any) []any {
	for i, p := range parts {
		if m, ok := p.(map[string]any); ok {
			if t, ok := m["type"].(string); ok && t == "text" {
				if text, ok := m["text"].(string); ok {
					m["text"] = cavemanCompress(text)
				}
			}
			parts[i] = m
		}
	}
	return parts
}

var fillerWords = []string{
	"basically", "actually", "really", "very", "quite",
	"just", "simply", "literally", "honestly",
	"you know", "i mean", "in terms of", "when it comes to",
	"at the end of the day", "for what it's worth",
	"in order to", "due to the fact that", "in the event that",
	"a large number of", "the majority of",
	"well,", "so,", "now,", "then,",
}

// removeFillerWords 移除填充词（不区分大小写）
func removeFillerWords(s string) string {
	for _, w := range fillerWords {
		lowerW := strings.ToLower(w)
		for {
			lower := strings.ToLower(s)
			idx := strings.Index(lower, lowerW)
			if idx < 0 {
				break
			}
			s = s[:idx] + s[idx+len(w):]
		}
	}
	return s
}

// compressPunctuation 压缩重复标点
func compressPunctuation(s string) string {
	s = strings.ReplaceAll(s, "!!!", "!")
	s = strings.ReplaceAll(s, "??", "?")
	s = strings.ReplaceAll(s, "...", ".")
	s = strings.ReplaceAll(s, "  ", " ")
	return strings.TrimSpace(s)
}
