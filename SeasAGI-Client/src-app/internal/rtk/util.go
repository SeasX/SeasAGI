package rtk

import (
	"strconv"
)

func truncateToMax(s string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 8000
	}
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "\n... (truncated) ..."
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
