package gateway

// 端到端集成测试：用 httptest 假上游/假 Grant 中继驱动真实的 handleChatCompletions
// 主链路（鉴权 → 限流 → DLP → Caveman 注入 → RTK 压缩 → 路由 → 转发 → 响应透传），
// 验证 Caveman/RTK 的修改真实到达上游，而不只是单元测试层面生效。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/auth"
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
)

const (
	integrationAccessToken = "e2e-access-token"
	integrationChannelID   = "chan-e2e"
	integrationComboName   = "e2e-combo"
	integrationModel       = "test-model"

	cavemanConciseMarker = "Be concise. Use fewer words."
	rtkTruncateMarker    = "... (truncated) ..."
)

// wireMessage 用于断言到达上游的 messages 结构。
type wireMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type wireChatBody struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
}

// recordingUpstream 记录收到的请求体并返回固定 OpenAI 非流式补全。
type recordingUpstream struct {
	mu      sync.Mutex
	bodies  []wireChatBody
	handler http.HandlerFunc
}

func newRecordingUpstream(t *testing.T, path, reply string) (*httptest.Server, *recordingUpstream) {
	t.Helper()
	rec := &recordingUpstream{}
	rec.handler = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		var body wireChatBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
			return
		}
		rec.mu.Lock()
		rec.bodies = append(rec.bodies, body)
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chatcmpl-e2e","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, reply)
	}
	srv := httptest.NewServer(rec.handler)
	t.Cleanup(srv.Close)
	return srv, rec
}

func (r *recordingUpstream) last(t *testing.T) wireChatBody {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		t.Fatal("upstream received no requests")
	}
	return r.bodies[len(r.bodies)-1]
}

// buildLongGitDiff 构造一个可被 RTK 自动检测为 git-diff 的超长工具结果（约 4KB）。
func buildLongGitDiff() string {
	var sb strings.Builder
	sb.WriteString("diff --git a/internal/app/app.go b/internal/app/app.go\n")
	sb.WriteString("index 1a2b3c4..5d6e7f8 100644\n")
	sb.WriteString("--- a/internal/app/app.go\n")
	sb.WriteString("+++ b/internal/app/app.go\n")
	sb.WriteString("@@ -10,6 +10,40 @@\n")
	for i := 0; i < 60; i++ {
		sb.WriteString(fmt.Sprintf("+\tnewlyAddedLine_%02d := computeValue(%d) // padded padding padding\n", i, i))
	}
	return sb.String()
}

// writeIntegrationConfig 预写配置文件（NewServiceWithPath 会从文件加载 channels 与 config），
// 注册一个 channel_type=byok 的通道指向假上游，绕开 macOS Keychain（custom 类型会读真实钥匙串）。
func writeIntegrationConfig(t *testing.T, upstreamURL string, extra map[string]any) string {
	t.Helper()
	cfg := map[string]any{
		"default_model":        integrationModel,
		"default_combo_name":   integrationComboName,
		"routing_strategy":     "fallback",
		"rtk_enabled":          true,
		"rtk_max_output_chars": 500,
		"caveman_enabled":      true,
		"caveman_style":        "concise",
		"model_combos": []map[string]any{{
			"name":        integrationComboName,
			"strategy":    "fallback",
			"sticky_uses": 1,
			"steps":       []map[string]any{{"channel_id": integrationChannelID, "model": integrationModel}},
		}},
	}
	for k, v := range extra {
		cfg[k] = v
	}
	state := map[string]any{
		"config": cfg,
		"channels": []map[string]any{{
			"channel_id":    integrationChannelID,
			"channel_type":  "byok",
			"provider_type": "openai",
			"display_name":  "E2E Upstream",
			"base_url":      upstreamURL,
			"enabled":       true,
			"health_status": "healthy",
			"weight":        1,
		}},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// newIntegrationGateway 基于预写配置构建真实网关 Service。
func newIntegrationGateway(t *testing.T, configPath string) *Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // 隔离 logs / state store 的 HOME 写入
	cfgSvc := config.NewServiceWithPath(config.AppConfig{}, configPath)
	return NewService(0, integrationAccessToken, cfgSvc, auth.NewService(), logs.NewService())
}

// postChat 以合法令牌直调 handleChatCompletions，返回 recorder。
func postChat(t *testing.T, svc *Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+integrationAccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.handleChatCompletions(rec, req)
	return rec
}

func chatRequestBody(toolContent string) string {
	b, _ := json.Marshal(map[string]any{
		"model": integrationModel,
		"messages": []map[string]any{
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Summarize the changes."},
			{"role": "tool", "tool_call_id": "call_1", "content": toolContent},
		},
	})
	return string(b)
}

func contentString(t *testing.T, m wireMessage) string {
	t.Helper()
	s, ok := m.Content.(string)
	if !ok {
		t.Fatalf("message %q content is not a string: %T", m.Role, m.Content)
	}
	return s
}

// TestIntegrationBYOKComboAppliesCavemanAndRTK 验证 Combo/BYOK 路径：
// Caveman 系统提示与 RTK 压缩真实到达上游，响应原样透传，且 SetRTKConfig 热更新立即生效。
func TestIntegrationBYOKComboAppliesCavemanAndRTK(t *testing.T) {
	upstream, rec := newRecordingUpstream(t, "/v1/chat/completions", "upstream-ok")
	svc := newIntegrationGateway(t, writeIntegrationConfig(t, upstream.URL, nil))

	longDiff := buildLongGitDiff()
	resp := postChat(t, svc, chatRequestBody(longDiff))

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	// 响应方向原样透传：客户端拿到的就是假上游的补全
	if !strings.Contains(resp.Body.String(), "upstream-ok") {
		t.Errorf("client response should pass through upstream body, got: %s", resp.Body.String())
	}

	sent := rec.last(t)
	// Caveman：system 提示注入真实到达上游，且只注入一次（幂等）
	if len(sent.Messages) == 0 || sent.Messages[0].Role != "system" {
		t.Fatalf("first upstream message should be system, got %+v", sent.Messages)
	}
	sysContent := contentString(t, sent.Messages[0])
	if !strings.Contains(sysContent, cavemanConciseMarker) {
		t.Errorf("caveman prompt should reach upstream, got system content: %q", sysContent)
	}
	if strings.HasPrefix(sysContent, cavemanConciseMarker) {
		t.Errorf("caveman prompt should be appended to original system content, got: %q", sysContent)
	}
	if got := strings.Count(sysContent, cavemanConciseMarker); got != 1 {
		t.Errorf("caveman prompt should appear exactly once, got %d", got)
	}

	// RTK：超长工具结果被压缩到 500 字符上限（含截断标记）
	var toolMsg *wireMessage
	for i := range sent.Messages {
		if sent.Messages[i].Role == "tool" {
			toolMsg = &sent.Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("tool message missing from upstream request")
	}
	toolContent := contentString(t, *toolMsg)
	if len(toolContent) >= len(longDiff) {
		t.Errorf("tool result should be compressed: original=%d sent=%d", len(longDiff), len(toolContent))
	}
	if !strings.Contains(toolContent, rtkTruncateMarker) {
		t.Errorf("compressed tool result should contain truncation marker, got:\n%s", toolContent)
	}
	if !strings.HasPrefix(toolContent, "diff --git") {
		t.Errorf("compressed tool result should keep diff header, got:\n%s", toolContent)
	}
	if sent.Model != integrationModel {
		t.Errorf("upstream model = %q, want %q", sent.Model, integrationModel)
	}

	// 热更新：关闭 RTK 后同一入口不再压缩，但 Caveman 注入保持
	svc.SetRTKConfig(false, 0)
	resp2 := postChat(t, svc, chatRequestBody(longDiff))
	if resp2.Code != http.StatusOK {
		t.Fatalf("second request status = %d", resp2.Code)
	}
	sent2 := rec.last(t)
	var tool2 *wireMessage
	for i := range sent2.Messages {
		if sent2.Messages[i].Role == "tool" {
			tool2 = &sent2.Messages[i]
			break
		}
	}
	if tool2 == nil {
		t.Fatal("tool message missing after RTK disabled")
	}
	if got := contentString(t, *tool2); got != longDiff {
		t.Errorf("tool result should be untouched after RTK disabled")
	}
	if !strings.Contains(contentString(t, sent2.Messages[0]), cavemanConciseMarker) {
		t.Error("caveman prompt should still reach upstream after RTK disabled")
	}
}

// TestIntegrationGrantRelayCarriesCavemanAndRTK 验证 Grant 中继路径：
// 请求体（bodyBytes）同样携带 Caveman 注入与 RTK 压缩后的 messages。
func TestIntegrationGrantRelayCarriesCavemanAndRTK(t *testing.T) {
	relay, rec := newRecordingUpstream(t, "/relay/chat/completions", "relay-ok")
	upstream, _ := newRecordingUpstream(t, "/v1/chat/completions", "upstream-ok")
	svc := newIntegrationGateway(t, writeIntegrationConfig(t, upstream.URL, map[string]any{
		"selected_grant_id":        "grant-e2e",
		"selected_grant_relay_url": relay.URL,
	}))

	longDiff := buildLongGitDiff()
	resp := postChat(t, svc, chatRequestBody(longDiff))

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "relay-ok") {
		t.Errorf("client response should pass through relay body, got: %s", resp.Body.String())
	}

	// Grant 命中后直接转发中继响应；client 已拿到 relay-ok 即代表命中 Grant 路径。

	sent := rec.last(t)
	if len(sent.Messages) == 0 || sent.Messages[0].Role != "system" {
		t.Fatalf("relay first message should be system, got %+v", sent.Messages)
	}
	if !strings.Contains(contentString(t, sent.Messages[0]), cavemanConciseMarker) {
		t.Errorf("caveman prompt should reach grant relay, got system content: %q", contentString(t, sent.Messages[0]))
	}
	var toolMsg *wireMessage
	for i := range sent.Messages {
		if sent.Messages[i].Role == "tool" {
			toolMsg = &sent.Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("tool message missing from relay request")
	}
	toolContent := contentString(t, *toolMsg)
	if len(toolContent) >= len(longDiff) {
		t.Errorf("tool result should be compressed on grant path: original=%d sent=%d", len(longDiff), len(toolContent))
	}
	if !strings.Contains(toolContent, rtkTruncateMarker) {
		t.Errorf("compressed tool result should contain truncation marker on grant path, got:\n%s", toolContent)
	}
}
