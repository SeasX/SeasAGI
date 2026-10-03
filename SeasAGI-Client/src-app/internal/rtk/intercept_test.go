package rtk

import (
	"reflect"
	"strings"
	"testing"
)

func bigToolOutput() string {
	lines := []string{"diff --git a/big.go b/big.go"}
	for i := 0; i < 60; i++ {
		lines = append(lines, "+"+strings.Repeat("data", 40))
	}
	return strings.Join(lines, "\n")
}

func TestApplyPipelineToMessagesNilPipeline(t *testing.T) {
	msgs := []map[string]any{{"role": "tool", "content": bigToolOutput()}}
	if got := ApplyPipelineToMessages(msgs, nil); !reflect.DeepEqual(got, msgs) {
		t.Error("nil pipeline must return messages unchanged")
	}
}

func TestApplyPipelineToMessagesDisabled(t *testing.T) {
	p := NewPipeline(false, 8000)
	original := bigToolOutput()
	msgs := []map[string]any{{"role": "tool", "content": original}}
	ApplyPipelineToMessages(msgs, p)
	if msgs[0]["content"] != original {
		t.Error("disabled pipeline must not modify tool content")
	}
}

func TestApplyPipelineToMessagesOnlyToolRolesCompressed(t *testing.T) {
	p := NewPipeline(true, 1000)
	original := bigToolOutput()
	assistant := "thinking " + strings.Repeat("x", 300)
	system := strings.Repeat("y", 300)
	msgs := []map[string]any{
		{"role": "user", "content": "run it"},
		{"role": "tool", "content": original},
		{"role": "function", "content": original},
		{"role": "assistant", "content": assistant},
		{"role": "system", "content": system},
	}
	ApplyPipelineToMessages(msgs, p)

	if got := msgs[1]["content"].(string); len(got) >= len(original) {
		t.Errorf("tool content should be compressed: got %d chars, want < %d", len(got), len(original))
	}
	if got := msgs[2]["content"].(string); len(got) >= len(original) {
		t.Errorf("function content should be compressed: got %d chars, want < %d", len(got), len(original))
	}
	if msgs[0]["content"] != "run it" {
		t.Error("user content must be preserved")
	}
	if msgs[3]["content"] != assistant {
		t.Error("assistant content must be preserved (model output direction)")
	}
	if msgs[4]["content"] != system {
		t.Error("system content must be preserved")
	}
}

func TestApplyPipelineToMessagesNonStringContent(t *testing.T) {
	p := NewPipeline(true, 1000)
	arr := []any{map[string]any{"type": "text", "text": "output"}}
	msgs := []map[string]any{{"role": "tool", "content": arr}}
	ApplyPipelineToMessages(msgs, p)
	if !reflect.DeepEqual(msgs[0]["content"], arr) {
		t.Error("non-string tool content must be preserved")
	}
}
