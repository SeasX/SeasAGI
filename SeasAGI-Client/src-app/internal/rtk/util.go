package rtk

import (
	"strconv"
)

func truncateToMax(s string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 8000
	}
	// 按 rune 计数与截断，避免在多字节 UTF-8 字符中间切断产生乱码。
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars]) + "\n... (truncated) ..."
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
