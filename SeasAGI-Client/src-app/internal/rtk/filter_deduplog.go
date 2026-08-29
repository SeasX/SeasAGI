package rtk

import (
	"fmt"
	"strings"
)

func FilterDedupLog(content string, maxChars int) string {
	lines := strings.Split(content, "\n")
	var result []string
	i := 0

	for i < len(lines) {
		if lines[i] == "" {
			result = append(result, "")
			i++
			continue
		}

		j := i + 1
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}

		count := j - i
		if count > 2 {
			result = append(result, lines[i])
			result = append(result, fmt.Sprintf("... (%d repeated lines) ...", count-1))
		} else {
			for k := i; k < j; k++ {
				result = append(result, lines[k])
			}
		}
		i = j
	}

	out := strings.Join(result, "\n")
	return truncateToMax(out, maxChars)
}
