package rtk

import (
	"strings"
)

func FilterGrep(content string, maxChars int) string {
	lines := strings.Split(content, "\n")
	seen := make(map[string]struct{})
	var deduped []string

	for _, line := range lines {
		if _, exists := seen[line]; !exists {
			seen[line] = struct{}{}
			deduped = append(deduped, line)
		}
	}

	headTail := 100
	if len(deduped) > headTail*2 {
		result := strings.Join(deduped[:headTail], "\n")
		result += "\n... (" + itoa(len(deduped)-headTail*2) + " more results omitted) ...\n"
		result += strings.Join(deduped[len(deduped)-headTail:], "\n")
		return truncateToMax(result, maxChars)
	}

	result := strings.Join(deduped, "\n")
	return truncateToMax(result, maxChars)
}
