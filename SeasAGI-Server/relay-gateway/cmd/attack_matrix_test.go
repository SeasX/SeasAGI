package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/auth"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/channel"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/trace"
	"github.com/gin-gonic/gin"
)

// TestRelayAttackMatrix 覆盖社区版中继数据面的越权、参数篡改与重放：
//   a) 无凭证/伪造凭证调用 /relay/chat/completions → 401；
//   b) 伪造 X-Upstream-Model 不改变实际上游模型；
//   c) 复用同一 X-Request-Id 连续两次请求：社区版 relay 不按客户端 request_id
//      做计量抑制，服务端为每次请求生成独立 trace 记录，两次请求都会打到上游。
func TestRelayAttackMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("JWT_SECRET", "relay-attack-matrix-jwt-secret-0123456789")
	t.Setenv("RATE_LIMIT_RPM", "600")

	upstreamModels := make(chan string, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(raw, &payload)
		upstreamModels <- payload.Model

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-test","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	defer upstream.Close()

	router, err := BuildRouter()
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}
	store := routerStore
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})

	store.AddChannel(channel.Channel{
		ChannelID:    "chan-test",
		ProviderType: "openai",
		DisplayName:  "Test Upstream",
		BaseURL:      upstream.URL,
		APIKey:       "sk-test",
		Enabled:      true,
		Models:       []string{"gpt-4o-mini"},
		Weight:       100,
		Health:       channel.HealthStatus{Healthy: true},
	})

	trace.InitStore(store.DBPath())

	token, err := auth.GenerateToken("user-attack", "chan-test")
	if err != nil {
		t.Fatalf("generate token failed: %v", err)
	}

	post := func(authHeader string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/relay/chat/completions",
			strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	t.Run("missing_or_forged_credentials_are_rejected", func(t *testing.T) {
		anonymous := post("", nil)
		if anonymous.Code != http.StatusUnauthorized && anonymous.Code != http.StatusForbidden {
			t.Fatalf("anonymous relay request returned %d, want 401/403: %s", anonymous.Code, anonymous.Body.String())
		}

		forged := post("Bearer forged-token", nil)
		if forged.Code != http.StatusUnauthorized && forged.Code != http.StatusForbidden {
			t.Fatalf("forged relay credential returned %d, want 401/403: %s", forged.Code, forged.Body.String())
		}
	})

	t.Run("upstream_model_header_is_ignored", func(t *testing.T) {
		rec := post("Bearer "+token, map[string]string{"X-Upstream-Model": "claude-3-opus"})
		if rec.Code != http.StatusOK {
			t.Fatalf("relay request failed: %d %s", rec.Code, rec.Body.String())
		}
		select {
		case model := <-upstreamModels:
			if model != "gpt-4o-mini" {
				t.Fatalf("upstream model was rewritten by client header: %q", model)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for upstream request")
		}
	})

	t.Run("replayed_request_id_does_not_suppress_records", func(t *testing.T) {
		headers := map[string]string{"X-Request-Id": "client-fixed-request-id"}
		first := post("Bearer "+token, headers)
		if first.Code != http.StatusOK {
			t.Fatalf("first relay request failed: %d %s", first.Code, first.Body.String())
		}
		second := post("Bearer "+token, headers)
		if second.Code != http.StatusOK {
			t.Fatalf("second relay request failed: %d %s", second.Code, second.Body.String())
		}

		// 两次请求都必须打到上游，说明客户端 request_id 未用于去重/抑制。
		for i := 0; i < 2; i++ {
			select {
			case model := <-upstreamModels:
				if model != "gpt-4o-mini" {
					t.Fatalf("unexpected upstream model: %q", model)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for upstream request")
			}
		}

		// trace 记录由服务端异步写入；等待两条独立记录出现。
		var records []trace.Record
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			records, err = trace.GetStore().QueryByUserID("user-attack", 10)
			if err == nil && len(records) >= 2 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if len(records) < 2 {
			t.Fatalf("expected two independent trace records, got %d", len(records))
		}
		seen := make(map[string]bool)
		for _, rec := range records {
			if rec.TraceID == "client-fixed-request-id" {
				t.Fatalf("client X-Request-Id leaked into server trace key: %q", rec.TraceID)
			}
			seen[rec.TraceID] = true
		}
		if len(seen) < 2 {
			t.Fatalf("replayed client request id suppressed server records: %v", seen)
		}
	})
}
