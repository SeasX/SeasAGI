package rtk

import (
	"fmt"
	"strings"
)

const defaultHeadTailLines = 50

func FilterSmartTruncate(content string, maxChars int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= defaultHeadTailLines*2 {
		return truncateToMax(content, maxChars)
	}

	head := lines[:defaultHeadTailLines]
	tail := lines[len(lines)-defaultHeadTailLines:]
	omitted := len(lines) - defaultHeadTailLines*2

	var sb strings.Builder
	sb.WriteString(strings.Join(head, "\n"))
	sb.WriteString(fmt.Sprintf("\n\n... (%d lines omitted) ...\n\n", omitted))
	sb.WriteString(strings.Join(tail, "\n"))

	return truncateToMax(sb.String(), maxChars)
}
