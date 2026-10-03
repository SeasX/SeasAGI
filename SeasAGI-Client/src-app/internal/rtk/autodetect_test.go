package rtk

import (
	"fmt"
	"strings"
	"testing"
)

func TestDetectOutputTypeEmpty(t *testing.T) {
	if got := DetectOutputType(""); got != TypeUnknown {
		t.Errorf("empty content should be %s, got %s", TypeUnknown, got)
	}
}

func TestDetectOutputTypeGitDiff(t *testing.T) {
	content := strings.Join([]string{
		"diff --git a/main.go b/main.go",
		"index 83db48f..bf269f4 100644",
		"--- a/main.go",
		"+++ b/main.go",
		"@@ -1,3 +1,4 @@",
		" package main",
		"+func main() {}",
	}, "\n")
	if got := DetectOutputType(content); got != TypeGitDiff {
		t.Errorf("want %s, got %s", TypeGitDiff, got)
	}
}

func TestDetectOutputTypeGitStatus(t *testing.T) {
	content := strings.Join([]string{
		"M  cmd/main.go",
		"A  internal/util.go",
		"D  old/legacy.go",
	}, "\n")
	if got := DetectOutputType(content); got != TypeGitStatus {
		t.Errorf("want %s, got %s", TypeGitStatus, got)
	}
}

func TestDetectOutputTypeGrep(t *testing.T) {
	content := strings.Join([]string{
		"src/main.go:10: func main() {",
		"src/util.go:22: helper()",
		"internal/app.go:5: init()",
	}, "\n")
	if got := DetectOutputType(content); got != TypeGrep {
		t.Errorf("want %s, got %s", TypeGrep, got)
	}
}

func TestDetectOutputTypeFind(t *testing.T) {
	content := strings.Join([]string{
		"src/a/main.go",
		"src/b/util.go",
		"src/c/app.go",
	}, "\n")
	if got := DetectOutputType(content); got != TypeFind {
		t.Errorf("want %s, got %s", TypeFind, got)
	}
}

func TestDetectOutputTypeLs(t *testing.T) {
	content := strings.Join([]string{
		"README.md",
		"Makefile",
		"docs",
		"notes.txt",
	}, "\n")
	if got := DetectOutputType(content); got != TypeLs {
		t.Errorf("want %s, got %s", TypeLs, got)
	}
}

func TestDetectOutputTypeReadNumbered(t *testing.T) {
	content := strings.Join([]string{
		"1  package main",
		"2  import \"fmt\"",
		"3  func main() {}",
		"4  var x = 1",
	}, "\n")
	if got := DetectOutputType(content); got != TypeReadNumbered {
		t.Errorf("want %s, got %s", TypeReadNumbered, got)
	}
}

func TestDetectOutputTypeDedupLog(t *testing.T) {
	lines := make([]string, 0, 210)
	for i := 0; i < 70; i++ {
		lines = append(lines,
			"2026-09-30 10:00:00 ERROR connection refused",
			"2026-09-30 10:00:00 ERROR connection refused",
			"2026-09-30 10:00:00 ERROR connection refused")
	}
	content := strings.Join(lines, "\n")
	if got := DetectOutputType(content); got != TypeDedupLog {
		t.Errorf("want %s, got %s", TypeDedupLog, got)
	}
}

func TestDetectOutputTypeSmartTruncate(t *testing.T) {
	lines := make([]string, 0, 330)
	for i := 0; i < 30; i++ {
		lines = append(lines, "")
	}
	for i := 0; i < 300; i++ {
		lines = append(lines, fmt.Sprintf("entry-%d value", i))
	}
	content := strings.Join(lines, "\n")
	if got := DetectOutputType(content); got != TypeSmartTruncate {
		t.Errorf("want %s, got %s", TypeSmartTruncate, got)
	}
}

func TestDetectOutputTypeUnknown(t *testing.T) {
	content := "hello world\nsecond line"
	if got := DetectOutputType(content); got != TypeUnknown {
		t.Errorf("want %s, got %s", TypeUnknown, got)
	}
}
