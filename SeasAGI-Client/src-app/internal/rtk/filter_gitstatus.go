package rtk

import (
	"strings"
)

func FilterGitStatus(content string, maxChars int) string {
	lines := strings.Split(content, "\n")
	var kept []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "On branch") ||
			strings.HasPrefix(trimmed, "Your branch") || strings.HasPrefix(trimmed, "nothing to") ||
			strings.HasPrefix(trimmed, "Untracked") || strings.HasPrefix(trimmed, "no changes") {
			continue
		}

		if len(trimmed) >= 2 {
			c0 := trimmed[0]
			if c0 == 'M' || c0 == 'A' || c0 == 'D' || c0 == 'R' || c0 == '?' || c0 == '!' || c0 == 'C' {
				kept = append(kept, trimmed)
			}
		}
	}

	if len(kept) == 0 {
		return content
	}

	result := strings.Join(kept, "\n")
	return truncateToMax(result, maxChars)
}
