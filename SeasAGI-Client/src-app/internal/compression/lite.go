package compression

import (
	"regexp"
	"strings"
)

// LiteEngine 无损压缩：whitespace + image-URL trimming
type LiteEngine struct{}

func (e *LiteEngine) Name() EngineName { return EngineLite }

func (e *LiteEngine) Compress(messages []map[string]any, cfg CompressionConfig) ([]map[string]any, CompressionResult, error) {
	originalTokens := estimateTokens(messages)

	for i, msg := range messages {
		if content, ok := msg["content"].(string); ok {
			msg["content"] = liteCompressString(content)
		} else if parts, ok := msg["content"].([]any); ok {
			msg["content"] = liteCompressParts(parts)
		}
		messages[i] = msg
	}

	compressedTokens := estimateTokens(messages)
	return messages, CompressionResult{
		OriginalTokens:   originalTokens,
		CompressedTokens: compressedTokens,
		SavingsPct:       savingsPct(originalTokens, compressedTokens),
		Engine:           EngineLite,
	}, nil
}

var (
	multiSpaceRe   = regexp.MustCompile(`[ \t]+`)
	multiNewlineRe = regexp.MustCompile(`\n{3,}`)
	imageURLRe     = regexp.MustCompile(`https?://[^\s"]+\.(png|jpg|jpeg|gif|webp|bmp|svg)(\?[^\s"]*)?`)
)

// liteCompressString 压缩字符串内容
func liteCompressString(s string) string {
	// 去除多余空白
	s = strings.TrimSpace(s)
	s = multiSpaceRe.ReplaceAllString(s, " ")
	s = multiNewlineRe.ReplaceAllString(s, "\n\n")
	// image URL 裁剪为占位符
	s = imageURLRe.ReplaceAllString(s, "[image]")
	return s
}

// liteCompressParts 压缩 content parts 数组
func liteCompressParts(parts []any) []any {
	for i, p := range parts {
		if m, ok := p.(map[string]any); ok {
			if t, ok := m["type"].(string); ok && t == "text" {
				if text, ok := m["text"].(string); ok {
					m["text"] = liteCompressString(text)
				}
			}
			if t, ok := m["type"].(string); ok && t == "image_url" {
				if _, ok := m["image_url"].(map[string]any); ok {
					m["image_url"] = map[string]any{"url": "[image]"}
				}
			}
			parts[i] = m
		}
	}
	return parts
}
