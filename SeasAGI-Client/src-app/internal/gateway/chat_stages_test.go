package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
)

// 任务类型优先级（显式 task_type > tools）、约束合并（MITM 规则覆盖请求体）、
// 调试开关不透传下游。
func TestExtractRouteHints(t *testing.T) {
	body := &chatBody{raw: map[string]any{
		"task_type":      "code",
		"tools":          []any{map[string]any{"type": "function"}},
		"max_price":      1.5,
		"data_policy":    "no-train",
		"_seasagi_debug": true,
	}}
	req := &protocol.CanonicalRequest{Model: "m", Messages: []map[string]any{{"role": "user", "content": "hi"}}}
	hints := extractRouteHints(body, req, mitmDecision{Constraints: map[string]any{"data_policy": "zdr"}})

	if hints.taskType != "code" || req.Extra["_task_type"] != "code" {
		t.Fatalf("taskType = %q extra=%v", hints.taskType, req.Extra)
	}
	if hints.constraints["max_price"] != 1.5 || hints.constraints["data_policy"] != "zdr" {
		t.Fatalf("constraints = %v", hints.constraints)
	}
	if !hints.debugTrace {
		t.Fatal("debug flag not detected")
	}
	if _, ok := body.raw["_seasagi_debug"]; ok {
		t.Fatal("debug flag must not be forwarded upstream")
	}

	// 无显式 task_type 但带 tools → tools；无 raw body 时默认 chat。
	tools := extractRouteHints(&chatBody{raw: map[string]any{"tools": []any{1}}}, &protocol.CanonicalRequest{}, mitmDecision{})
	if tools.taskType != "tools" {
		t.Fatalf("tools taskType = %q", tools.taskType)
	}
	if plain := extractRouteHints(&chatBody{}, &protocol.CanonicalRequest{}, mitmDecision{}); plain.taskType != "chat" || plain.debugTrace {
		t.Fatalf("default hints = %+v", plain)
	}
}

// Gemini 原生路径：model/stream 注入 raw，并同步到转发字节。
func TestReadChatBodyInjectsGeminiPathModel(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-x:streamGenerateContent", strings.NewReader(`{"contents":[]}`))
	r = r.WithContext(context.WithValue(r.Context(), geminiPathCtxKey{}, geminiPathInfo{Model: "gemini-x", Action: "streamGenerateContent"}))
	body, ok := (&Service{}).readChatBody(httptest.NewRecorder(), r)
	if !ok {
		t.Fatal("readChatBody failed")
	}
	var forwarded map[string]any
	if err := json.Unmarshal(body.bytes, &forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded["model"] != "gemini-x" || forwarded["stream"] != true || body.raw["model"] != "gemini-x" {
		t.Fatalf("forwarded = %v raw = %v", forwarded, body.raw)
	}
}
