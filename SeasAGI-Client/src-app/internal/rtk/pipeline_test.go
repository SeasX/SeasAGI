package rtk

import (
	"strings"
	"testing"
)

func TestPipelineDisabled(t *testing.T) {
	p := NewPipeline(false, 100)
	content := strings.Repeat("line\n", 100)
	if got := p.Process(content); got != content {
		t.Error("disabled pipeline must pass content through unchanged")
	}
}

func TestPipelineShortContentPassthrough(t *testing.T) {
	p := NewPipeline(true, 100)
	short := strings.Repeat("short line\n", 10) // < 200 chars
	if got := p.Process(short); got != short {
		t.Error("content under 200 chars must pass through unchanged")
	}
}

func TestPipelineDefaultMaxChars(t *testing.T) {
	if p := NewPipeline(true, 0); p.MaxOutputChars != 8000 {
		t.Errorf("zero MaxOutputChars should default to 8000, got %d", p.MaxOutputChars)
	}
	if p := NewPipeline(true, -5); p.MaxOutputChars != 8000 {
		t.Errorf("negative MaxOutputChars should default to 8000, got %d", p.MaxOutputChars)
	}
}

func TestPipelineUnknownTypePassthrough(t *testing.T) {
	p := NewPipeline(true, 8000)
	content := strings.Repeat("x", 300) // 单行长文本，无法归类
	if got := DetectOutputType(content); got != TypeUnknown {
		t.Fatalf("precondition: want %s, got %s", TypeUnknown, got)
	}
	if got := p.Process(content); got != content {
		t.Error("unknown type content must pass through unchanged")
	}
}

func TestPipelineCompressesToolResult(t *testing.T) {
	p := NewPipeline(true, 500)
	lines := []string{"diff --git a/big.go b/big.go"}
	for i := 0; i < 50; i++ {
		lines = append(lines, "+"+strings.Repeat("x", 50))
	}
	content := strings.Join(lines, "\n")
	got := p.Process(content)
	if len(got) >= len(content) {
		t.Errorf("output (%d chars) should be smaller than input (%d chars)", len(got), len(content))
	}
	if !strings.Contains(got, "... (truncated) ...") {
		t.Error("output should carry truncation marker")
	}
}
