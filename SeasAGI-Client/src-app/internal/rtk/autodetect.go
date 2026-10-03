package rtk

import (
	"strings"
)

type OutputType string

const (
	TypeGitDiff       OutputType = "git-diff"
	TypeGitStatus     OutputType = "git-status"
	TypeGrep          OutputType = "grep"
	TypeFind          OutputType = "find"
	TypeLs            OutputType = "ls"
	TypeReadNumbered  OutputType = "read-numbered"
	TypeSearchList    OutputType = "search-list"
	TypeDedupLog      OutputType = "dedup-log"
	TypeSmartTruncate OutputType = "smart-truncate"
	TypeUnknown       OutputType = "unknown"
)

const detectionWindowLines = 20

func DetectOutputType(content string) OutputType {
	lines := headLines(content, detectionWindowLines)
	if len(lines) == 0 {
		return TypeUnknown
	}

	if isGitDiff(lines) {
		return TypeGitDiff
	}
	if isGitStatus(lines) {
		return TypeGitStatus
	}
	if isGrepResult(lines) {
		return TypeGrep
	}
	// read-numbered 需在 ls 之前判定：ls 对任意 ≥3 行非空内容都会命中，
	// 若 ls 先行，编号读取输出将永远被误判为 ls。
	if isReadNumbered(lines) {
		return TypeReadNumbered
	}
	if isFindResult(lines) {
		return TypeFind
	}
	if isSearchList(lines) {
		return TypeSearchList
	}

	totalLines := len(strings.Split(content, "\n"))
	// dedup-log/smart-truncate 是大内容兜底，需在 ls 之前判定，理由同上。
	if totalLines > 200 && hasConsecutiveDupes(content) {
		return TypeDedupLog
	}
	if isLsResult(lines) {
		return TypeLs
	}
	if totalLines > 300 {
		return TypeSmartTruncate
	}

	return TypeUnknown
}

func headLines(content string, n int) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		return lines[:n]
	}
	return lines
}

func isGitDiff(lines []string) bool {
	diffCount := 0
	hunkCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") || strings.HasPrefix(line, "diff -") {
			diffCount++
		}
		if strings.HasPrefix(line, "@@") {
			hunkCount++
		}
	}
	return diffCount >= 1 || hunkCount >= 1
}

func isGitStatus(lines []string) bool {
	statusCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) >= 3 {
			c := trimmed[0]
			if (c == 'M' || c == 'A' || c == 'D' || c == 'R' || c == '?' || c == '!' || c == 'C' || c == 'U') &&
				(trimmed[1] == ' ' || trimmed[1] == 'M' || trimmed[1] == 'A' || trimmed[1] == 'D') {
				statusCount++
			}
		}
	}
	return statusCount >= 2
}

func isGrepResult(lines []string) bool {
	matchCount := 0
	for _, line := range lines {
		if idx := strings.Index(line, ":"); idx > 0 {
			before := line[:idx]
			if looksLikeFilePath(before) {
				matchCount++
			}
		}
	}
	return matchCount >= 3
}

func isFindResult(lines []string) bool {
	pathCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if looksLikeFilePath(trimmed) && !strings.Contains(trimmed, ":") {
			pathCount++
		}
	}
	return pathCount >= 3
}

func isLsResult(lines []string) bool {
	entryCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "total") && !strings.HasPrefix(trimmed, "ls:") {
			entryCount++
		}
	}
	return entryCount >= 3
}

func isReadNumbered(lines []string) bool {
	numberedCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 4 {
			i := 0
			for i < len(trimmed) && (trimmed[i] >= '0' && trimmed[i] <= '9') {
				i++
			}
			if i > 0 && i < len(trimmed) && (trimmed[i] == ' ' || trimmed[i] == '|' || trimmed[i] == '\t') {
				numberedCount++
			}
		}
	}
	return numberedCount >= 3
}

func isSearchList(lines []string) bool {
	globCount := 0
	for _, line := range lines {
		if strings.Contains(line, "/") && (strings.Contains(line, ".ts") || strings.Contains(line, ".js") || strings.Contains(line, ".py") || strings.Contains(line, ".go")) {
			globCount++
		}
	}
	return globCount >= 3
}

func looksLikeFilePath(s string) bool {
	return strings.Contains(s, "/") || strings.Contains(s, "\\") ||
		strings.Contains(s, ".ts") || strings.Contains(s, ".js") ||
		strings.Contains(s, ".py") || strings.Contains(s, ".go") ||
		strings.Contains(s, ".rs") || strings.Contains(s, ".java")
}

func hasConsecutiveDupes(content string) bool {
	lines := strings.Split(content, "\n")
	dupeRuns := 0
	for i := 1; i < len(lines) && i < 100; i++ {
		if lines[i] == lines[i-1] && lines[i] != "" {
			dupeRuns++
		}
	}
	return dupeRuns >= 3
}
