package gateway

// 端到端集成测试（多账号容错 + 流式记账）：驱动真实的网关主链路，验证 P0/P1 修复在
// 完整链路上真实生效，而不只是单元层面。
//
// 说明：多 key 轮询/冷却用例直接驱动 forwardRequest（生产转发主函数），候选步骤以内存
// 构造，避免在 macOS 上写入系统 keychain（config 载入明文多 key 会迁移进 keychain，
// 导致测试依赖外部状态且缓慢）；流式记账用例驱动完整的 handleChatCompletions 入口。

import (
	"context"
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
	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

// writeMultiKeyConfig 预写配置：单渠道（byok）；keys 非空时写入 api_keys。
func writeMultiKeyConfig(t *testing.T, upstreamURL string, keys []string, extraCfg map[string]any) string {
	t.Helper()
	cfg := map[string]any{
		"default_model":      integrationModel,
		"default_combo_name": integrationComboName,
		"routing_strategy":   "fallback",
		"rtk_enabled":        false,
		"caveman_enabled":    false,
		"model_combos": []map[string]any{{
			"name":        integrationComboName,
			"strategy":    "fallback",
			"sticky_uses": 1,
			"steps":       []map[string]any{{"channel_id": integrationChannelID, "model": integrationModel}},
		}},
	}
	for k, v := range extraCfg {
		cfg[k] = v
	}
	channel := map[string]any{
		"channel_id":    integrationChannelID,
		"channel_type":  "byok",
		"provider_type": "openai",
		"display_name":  "E2E MultiKey",
		"base_url":      upstreamURL,
		"enabled":       true,
		"health_status": "healthy",
		"weight":        1,
	}
	if len(keys) > 0 {
		channel["api_keys"] = keys
	}
	state := map[string]any{"config": cfg, "channels": []map[string]any{channel}}
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

// newForwardTestService 在隔离 HOME 下构建网关 Service（无渠道配置，不触碰 keychain）。
func newForwardTestService(t *testing.T) *Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfgSvc := config.NewServiceWithPath(config.AppConfig{}, filepath.Join(t.TempDir(), "config.json"))
	return NewService(0, integrationAccessToken, cfgSvc, auth.NewService(), logs.NewService())
}

// keyedUpstream 记录每个 key 的命中次数；仅 "good-key" 返回 200，其余返回 401。
type keyedUpstream struct {
	mu   sync.Mutex
	seen map[string]int
}

func newKeyedUpstream(t *testing.T) (*httptest.Server, *keyedUpstream) {
	t.Helper()
	u := &keyedUpstream{seen: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		u.mu.Lock()
		u.seen[key]++
		u.mu.Unlock()

		if key != "good-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chatcmpl-mk","object":"chat.completion","model":"%s","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`, integrationModel)
	}))
	t.Cleanup(srv.Close)
	return srv, u
}

func (u *keyedUpstream) count(key string) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.seen[key]
}

func multiKeySteps(upstreamURL string) []routing.PlanStep {
	return []routing.PlanStep{{
		Channel: config.Channel{
			ChannelID:    "chan-mk",
			ChannelType:  "byok",
			ProviderType: "openai",
			BaseURL:      upstreamURL,
			Enabled:      true,
			HealthStatus: "healthy",
			APIKeys:      []string{"bad-key", "good-key"},
		},
		UpstreamModel: integrationModel,
		StepRole:      "primary",
	}}
}

// TestMultiKeyRotationAndCooldown 验证：坏 key（401）触发按 key 冷却并被跳过，健康 key 接管；
// 后续请求不再浪费尝试被冷却的坏 key（修复前因 SetKeys 归零 + 整渠道跳过而失效）。
func TestMultiKeyRotationAndCooldown(t *testing.T) {
	upstream, rec := newKeyedUpstream(t)
	svc := newForwardTestService(t)
	steps := multiKeySteps(upstream.URL)
	req := &protocol.CanonicalRequest{
		Model:    integrationModel,
		Messages: []map[string]any{{"role": "user", "content": "hello"}},
	}

	resp, _, _, err := svc.forwardRequest(context.Background(), steps, req, "sess-1")
	if err != nil {
		t.Fatalf("first forward: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d, want 200", resp.StatusCode)
	}
	if rec.count("bad-key") != 1 {
		t.Fatalf("bad-key attempts = %d, want 1", rec.count("bad-key"))
	}
	if rec.count("good-key") != 1 {
		t.Fatalf("good-key attempts = %d, want 1", rec.count("good-key"))
	}
	if !svc.cooldownMgr.IsOnCooldown("bad-key") {
		t.Fatal("bad-key should be put on cooldown after 401")
	}

	// 第二次：坏 key 处于冷却 → 被跳过，只有好 key 命中上游
	resp2, _, _, err := svc.forwardRequest(context.Background(), steps, req, "sess-2")
	if err != nil {
		t.Fatalf("second forward: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second status = %d, want 200", resp2.StatusCode)
	}
	if rec.count("bad-key") != 1 {
		t.Fatalf("bad-key should be skipped on cooldown, attempts = %d", rec.count("bad-key"))
	}
	if rec.count("good-key") != 2 {
		t.Fatalf("good-key attempts = %d, want 2", rec.count("good-key"))
	}
}

func simpleChatBody(stream bool) string {
	b, _ := json.Marshal(map[string]any{
		"model":    integrationModel,
		"messages": []map[string]any{{"role": "user", "content": "hello"}},
		"stream":   stream,
	})
	return string(b)
}

// TestStreamingResponseAccounting 验证流式响应（SSE）经完整主链路后仍能正确采集并记账 token。
func TestStreamingResponseAccounting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeSSE := func(line string) {
			_, _ = fmt.Fprintln(w, line)
			_, _ = fmt.Fprintln(w)
			if flusher != nil {
				flusher.Flush()
			}
		}
		writeSSE(`data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hi"}}]}`)
		writeSSE(`data: {"id":"x","object":"chat.completion.chunk","usage":{"prompt_tokens":100,"completion_tokens":20}}`)
		writeSSE(`data: [DONE]`)
	}))
	defer upstream.Close()

	// newIntegrationGateway 会把 HOME 隔离到临时目录，用量服务随后在此 HOME 下创建。
	svc := newIntegrationGateway(t, writeMultiKeyConfig(t, upstream.URL, nil, nil))
	usageSvc := usage.NewService()
	svc.SetUsageService(usageSvc)

	resp := postChat(t, svc, simpleChatBody(true))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "[DONE]") {
		t.Fatalf("streamed body should pass through, got: %s", resp.Body.String())
	}

	records := usageSvc.GetAllRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(records))
	}
	r := records[0]
	if r.InputTokens != 100 || r.OutputTokens != 20 {
		t.Fatalf("streamed tokens = (in=%d out=%d), want (100, 20)", r.InputTokens, r.OutputTokens)
	}
}
