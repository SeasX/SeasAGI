package rtk

import (
	"strings"
)

func FilterGitDiff(content string, maxChars int) string {
	lines := strings.Split(content, "\n")
	var kept []string
	inHunk := false

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") || strings.HasPrefix(line, "diff -") ||
			strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "+++") {
			kept = append(kept, line)
			inHunk = false
			continue
		}
		if strings.HasPrefix(line, "@@") {
			kept = append(kept, line)
			inHunk = true
			continue
		}
		if inHunk && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")) {
			kept = append(kept, line)
			continue
		}
		if inHunk && strings.HasPrefix(line, " ") {
			continue
		}
		kept = append(kept, line)
	}

	result := strings.Join(kept, "\n")
	return truncateToMax(result, maxChars)
}
