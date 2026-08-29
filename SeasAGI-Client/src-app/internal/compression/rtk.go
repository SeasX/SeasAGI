package compression

import (
	"strings"
)

// RTKEngine 智能 tool-result 过滤
type RTKEngine struct{}

func (e *RTKEngine) Name() EngineName { return EngineRTK }

func (e *RTKEngine) Compress(messages []map[string]any, cfg CompressionConfig) ([]map[string]any, CompressionResult, error) {
	originalTokens := estimateTokens(messages)

	for i, msg := range messages {
		role, _ := msg["content"].(string)
		if msg["role"] == "tool" || msg["role"] == "function" {
			// tool-result 压缩：保留首尾，截断中间
			if len(role) > 500 {
				msg["content"] = rtkCompressToolResult(role)
			}
		} else if content, ok := msg["content"].(string); ok {
			// assistant 消息中的 tool_result 内容
			if len(content) > 1000 {
				msg["content"] = rtkCompressLongContent(content)
			}
		}
		messages[i] = msg
	}

	compressedTokens := estimateTokens(messages)
	return messages, CompressionResult{
		OriginalTokens:   originalTokens,
		CompressedTokens: compressedTokens,
		SavingsPct:       savingsPct(originalTokens, compressedTokens),
		Engine:           EngineRTK,
	}, nil
}

// rtkCompressToolResult 压缩 tool-result：保留首 200 + 尾 100 字符
func rtkCompressToolResult(s string) string {
	if len(s) <= 500 {
		return s
	}
	head := s[:200]
	tail := s[len(s)-100:]
	return head + "\n...[truncated " + itoa(len(s)-300) + " chars]...\n" + tail
}

// rtkCompressLongContent 压缩长内容
func rtkCompressLongContent(s string) string {
	if len(s) <= 1000 {
		return s
	}
	// 保留代码块，压缩散文
	lines := strings.Split(s, "\n")
	var result []string
	inCodeBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			result = append(result, line)
			continue
		}
		if inCodeBlock {
			result = append(result, line)
			continue
		}
		// 散文：去除空行和多余空白
		if trimmed == "" {
			continue
		}
		result = append(result, strings.TrimSpace(line))
	}
	return strings.Join(result, "\n")
}

// itoa 简易整数转字符串
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
