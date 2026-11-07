package gateway

// A6：四源协议入口 smoke test。用同一个 OpenAI 上游，验证
// Chat / Responses / Anthropic / Gemini 四种入站协议都能 200，
// 且关键字段（model、用户文本）完整到达上游、响应按源协议返回。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postProtocol 以指定路径与方法直调网关处理函数。
func postProtocol(t *testing.T, svc *Service, path, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+integrationAccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func lastUserText(t *testing.T, body wireChatBody) string {
	t.Helper()
	for i := len(body.Messages) - 1; i >= 0; i-- {
		if body.Messages[i].Role == "user" {
			return contentString(t, body.Messages[i])
		}
	}
	t.Fatalf("upstream request has no user message: %+v", body.Messages)
	return ""
}

func TestProtocolEntryPointsSmoke(t *testing.T) {
	upstream, rec := newRecordingUpstream(t, "/v1/chat/completions", "pong")
	svc := newIntegrationGateway(t, writeIntegrationConfig(t, upstream.URL, map[string]any{
		"rtk_enabled":     false,
		"caveman_enabled": false,
	}))

	cases := []struct {
		name     string
		path     string
		body     string
		wantText string
		handler  func(*Service) http.HandlerFunc
	}{
		{
			name:     "openai-chat",
			path:     "/v1/chat/completions",
			body:     `{"model":"test-model","messages":[{"role":"user","content":"ping-chat"}]}`,
			wantText: "ping-chat",
			handler:  func(s *Service) http.HandlerFunc { return s.handleChatCompletions },
		},
		{
			name:     "openai-responses",
			path:     "/v1/responses",
			body:     `{"model":"test-model","input":"ping-responses"}`,
			wantText: "ping-responses",
			handler:  func(s *Service) http.HandlerFunc { return s.handleResponses },
		},
		{
			name:     "anthropic-messages",
			path:     "/v1/messages",
			body:     `{"model":"test-model","max_tokens":64,"messages":[{"role":"user","content":"ping-anthropic"}]}`,
			wantText: "ping-anthropic",
			handler:  func(s *Service) http.HandlerFunc { return s.handleAnthropicMessages },
		},
		{
			name:     "gemini-generate-content",
			path:     "/v1beta/models/test-model:generateContent",
			body:     `{"contents":[{"role":"user","parts":[{"text":"ping-gemini"}]}]}`,
			wantText: "ping-gemini",
			handler:  func(s *Service) http.HandlerFunc { return s.handleGemini },
		},
	}

	for _, tc := range cases {
		resp := postProtocol(t, svc, tc.path, tc.body, tc.handler(svc))
		if resp.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", tc.name, resp.Code, resp.Body.String())
		}
		if strings.TrimSpace(resp.Body.String()) == "" {
			t.Fatalf("%s: empty response body", tc.name)
		}

		sent := rec.last(t)
		if sent.Model != integrationModel {
			t.Fatalf("%s: upstream model = %q, want %q", tc.name, sent.Model, integrationModel)
		}
		if got := lastUserText(t, sent); !strings.Contains(got, tc.wantText) {
			t.Fatalf("%s: upstream user text = %q, want contains %q", tc.name, got, tc.wantText)
		}
	}
}
