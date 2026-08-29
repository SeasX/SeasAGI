package rtk

import (
	"path/filepath"
	"strings"
)

func FilterPath(content string, maxChars int) string {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return truncateToMax(content, maxChars)
	}

	commonPrefix := findCommonPrefix(lines)

	var simplified []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if commonPrefix != "" && strings.HasPrefix(trimmed, commonPrefix) {
			rel := strings.TrimPrefix(trimmed, commonPrefix)
			rel = strings.TrimPrefix(rel, "/")
			rel = strings.TrimPrefix(rel, string(filepath.Separator))
			simplified = append(simplified, rel)
		} else {
			simplified = append(simplified, trimmed)
		}
	}

	headTail := 150
	if len(simplified) > headTail*2 {
		result := strings.Join(simplified[:headTail], "\n")
		result += "\n... (" + itoa(len(simplified)-headTail*2) + " more paths omitted) ...\n"
		result += strings.Join(simplified[len(simplified)-headTail:], "\n")
		return truncateToMax(result, maxChars)
	}

	result := strings.Join(simplified, "\n")
	return truncateToMax(result, maxChars)
}

func findCommonPrefix(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	first := strings.TrimSpace(lines[0])
	prefix := first

	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		for !strings.HasPrefix(trimmed, prefix) && len(prefix) > 0 {
			idx := strings.LastIndex(prefix, "/")
			if idx <= 0 {
				prefix = ""
				break
			}
			prefix = prefix[:idx]
		}
		if prefix == "" {
			break
		}
	}

	return prefix
}
