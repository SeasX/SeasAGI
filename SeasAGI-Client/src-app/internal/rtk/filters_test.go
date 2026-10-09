package rtk

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFilterSmartTruncate(t *testing.T) {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}
	content := strings.Join(lines, "\n")
	got := FilterSmartTruncate(content, 8000)

	if !strings.Contains(got, "(100 lines omitted)") {
		t.Errorf("output should report omitted count, got:\n%s", got)
	}
	if !strings.Contains(got, "line-0") || !strings.Contains(got, "line-49") {
		t.Error("head lines must be preserved")
	}
	if !strings.Contains(got, "line-150") || !strings.Contains(got, "line-199") {
		t.Error("tail lines must be preserved")
	}
	if strings.Contains(got, "line-75") || strings.Contains(got, "line-120") {
		t.Error("middle lines must be omitted")
	}
}

func TestFilterSmartTruncateShortInput(t *testing.T) {
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}
	content := strings.Join(lines, "\n")
	if got := FilterSmartTruncate(content, 8000); got != content {
		t.Error("input with <= 100 lines should only be char-truncated, not head/tail-cut")
	}
}

func TestFilterDedupLog(t *testing.T) {
	content := strings.Join([]string{"A", "A", "A", "A", "B", "B", "C"}, "\n")
	got := FilterDedupLog(content, 8000)
	want := "A\n... (3 repeated lines) ...\nB\nB\nC"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterGitDiff(t *testing.T) {
	content := strings.Join([]string{
		"diff --git a/f.go b/f.go",
		"--- a/f.go",
		"+++ b/f.go",
		"@@ -1,3 +1,3 @@",
		" context line",
		"-removed",
		"+added",
	}, "\n")
	got := FilterGitDiff(content, 8000)

	if strings.Contains(got, "context line") {
		t.Error("context lines should be dropped")
	}
	if !strings.Contains(got, "-removed") || !strings.Contains(got, "+added") {
		t.Error("add/remove lines must be preserved")
	}
	if !strings.Contains(got, "@@ -1,3 +1,3 @@") {
		t.Error("hunk header must be preserved")
	}
	if !strings.Contains(got, "diff --git a/f.go b/f.go") {
		t.Error("diff header must be preserved")
	}
}

func TestFilterGitStatus(t *testing.T) {
	content := strings.Join([]string{
		"On branch main",
		"Your branch is up to date",
		"M  modified.go",
		"?? untracked.go",
		"",
		"nothing to commit",
	}, "\n")
	got := FilterGitStatus(content, 8000)
	want := "M  modified.go\n?? untracked.go"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterGitStatusNoMatches(t *testing.T) {
	content := "On branch main\nnothing to commit"
	if got := FilterGitStatus(content, 8000); got != content {
		t.Errorf("no status lines should return original content, got %q", got)
	}
}

func TestFilterGrepDedup(t *testing.T) {
	content := strings.Join([]string{
		"a.go:1: x",
		"a.go:1: x",
		"b.go:2: y",
		"c.go:3: z",
	}, "\n")
	got := FilterGrep(content, 8000)
	want := "a.go:1: x\nb.go:2: y\nc.go:3: z"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterGrepHeadTail(t *testing.T) {
	lines := make([]string, 250)
	for i := range lines {
		lines[i] = fmt.Sprintf("f%d.go:1: unique %d", i, i)
	}
	got := FilterGrep(strings.Join(lines, "\n"), 8000)

	if !strings.Contains(got, "(50 more results omitted)") {
		t.Errorf("output should report omitted count, got:\n%s", got)
	}
	if !strings.Contains(got, "f0.go") || !strings.Contains(got, "f249.go") {
		t.Error("head and tail results must be preserved")
	}
	if strings.Contains(got, "f120.go") {
		t.Error("middle results must be omitted")
	}
}

func TestFilterPathCommonPrefix(t *testing.T) {
	content := strings.Join([]string{
		"/project/src/main.go",
		"/project/src/util.go",
		"/project/src/app/main.go",
	}, "\n")
	got := FilterPath(content, 8000)
	want := "main.go\nutil.go\napp/main.go"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterPathHeadTail(t *testing.T) {
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = fmt.Sprintf("/root/dir/file%d.txt", i)
	}
	got := FilterPath(strings.Join(lines, "\n"), 8000)

	if !strings.Contains(got, "(100 more paths omitted)") {
		t.Errorf("output should report omitted count, got:\n%s", got)
	}
	if !strings.Contains(got, "file0.txt") || !strings.Contains(got, "file399.txt") {
		t.Error("head and tail paths must be preserved")
	}
}

func TestTruncateToMax(t *testing.T) {
	if got := truncateToMax("short", 100); got != "short" {
		t.Errorf("short content must be unchanged, got %q", got)
	}
	long := strings.Repeat("a", 200)
	want := strings.Repeat("a", 50) + "\n... (truncated) ..."
	if got := truncateToMax(long, 50); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := truncateToMax(long, 0); got != long {
		t.Error("non-positive maxChars should default to 8000 and keep content")
	}
}

// 多字节字符必须按 rune 截断，不得产生非法 UTF-8。
func TestTruncateToMaxRuneSafe(t *testing.T) {
	content := strings.Repeat("汉", 10) // 10 runes, 30 bytes
	got := truncateToMax(content, 5)
	if !utf8.ValidString(got) {
		t.Errorf("truncated output is not valid UTF-8: %q", got)
	}
	want := strings.Repeat("汉", 5) + "\n... (truncated) ..."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
