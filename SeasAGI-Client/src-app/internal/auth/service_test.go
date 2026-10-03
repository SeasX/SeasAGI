package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

func TestNewService_NonNil(t *testing.T) {
	svc := NewService()
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
}

func TestNewService_NotLoggedInInitially(t *testing.T) {
	// NewService tries to load a token from the keychain/environment.
	// This test documents that the service is not logged in when no token is available.
	// For isolated state tests, construct the service directly.
	svc := &Service{}
	if svc.IsLoggedIn() {
		t.Fatal("expected service to not be logged in initially")
	}
}

func TestGetAuthState_ReturnsCorrectState(t *testing.T) {
	svc := &Service{
		info: AuthInfo{
			IsLoggedIn: true,
			UserID:     strPtr("user123"),
			Email:      strPtr("test@example.com"),
		},
		token: "some-token",
	}

	state := svc.GetAuthState()
	if !state.IsLoggedIn {
		t.Error("expected IsLoggedIn to be true")
	}
	if state.UserID == nil || *state.UserID != "user123" {
		t.Errorf("expected UserID 'user123', got %v", state.UserID)
	}
	if state.Email == nil || *state.Email != "test@example.com" {
		t.Errorf("expected Email 'test@example.com', got %v", state.Email)
	}
}

func TestIsLoggedIn_ReturnsFalseWhenNotLoggedIn(t *testing.T) {
	svc := &Service{
		info:  AuthInfo{IsLoggedIn: false},
		token: "",
	}

	if svc.IsLoggedIn() {
		t.Error("expected IsLoggedIn to return false")
	}
}

func TestGetPlatformToken_ReturnsEmptyWhenNotLoggedIn(t *testing.T) {
	svc := &Service{}
	token := svc.GetPlatformToken()
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestLogout_ClearsState(t *testing.T) {
	email := "test@example.com"
	svc := &Service{
		info: AuthInfo{
			IsLoggedIn: true,
			UserID:     strPtr("user123"),
			Email:      &email,
		},
		token: "some-token",
	}

	err := svc.Logout()
	if err != nil {
		t.Fatalf("Logout() returned error: %v", err)
	}

	if svc.IsLoggedIn() {
		t.Error("expected IsLoggedIn to be false after logout")
	}
	if svc.GetPlatformToken() != "" {
		t.Error("expected token to be empty after logout")
	}
	state := svc.GetAuthState()
	if state.IsLoggedIn {
		t.Error("expected AuthState.IsLoggedIn to be false after logout")
	}
}

func TestLogout_WhenNotLoggedIn(t *testing.T) {
	svc := &Service{
		info:  AuthInfo{IsLoggedIn: false},
		token: "",
	}

	err := svc.Logout()
	if err != nil {
		t.Fatalf("Logout() returned error: %v", err)
	}

	if svc.IsLoggedIn() {
		t.Error("expected IsLoggedIn to remain false")
	}
}

func TestExtractUserID_ReturnsFirst8Chars(t *testing.T) {
	svc := &Service{}
	token := "abcdefghijklmnop"
	result := svc.extractUserID(token)
	if result != "abcdefgh" {
		t.Errorf("expected 'abcdefgh', got %q", result)
	}
}

func TestExtractUserID_EmptyString(t *testing.T) {
	svc := &Service{}
	result := svc.extractUserID("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestExtractUserID_ShortString(t *testing.T) {
	svc := &Service{}
	token := "abc"
	result := svc.extractUserID(token)
	if result != "abc" {
		t.Errorf("expected 'abc', got %q", result)
	}
}

func TestExtractUserID_Exactly8Chars(t *testing.T) {
	svc := &Service{}
	token := "12345678"
	result := svc.extractUserID(token)
	if result != "12345678" {
		t.Errorf("expected '12345678', got %q", result)
	}
}

func TestConcurrentAccess(t *testing.T) {
	svc := &Service{
		info: AuthInfo{
			IsLoggedIn: true,
			UserID:     strPtr("concurrent"),
			Email:      strPtr("concurrent@test.com"),
		},
		token: "concurrent-token",
	}

	var wg sync.WaitGroup
	workers := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.GetAuthState()
			_ = svc.IsLoggedIn()
			_ = svc.GetPlatformToken()
		}()
	}

	wg.Wait()
}

func TestConcurrentAccess_WithLogout(t *testing.T) {
	email := "user@test.com"
	svc := &Service{
		info: AuthInfo{
			IsLoggedIn: true,
			UserID:     strPtr("user123"),
			Email:      &email,
		},
		token: "some-token",
	}

	var wg sync.WaitGroup
	workers := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%3 == 0 {
				_ = svc.Logout()
			} else {
				_ = svc.GetAuthState()
				_ = svc.IsLoggedIn()
				_ = svc.GetPlatformToken()
			}
		}(i)
	}

	wg.Wait()
}

// TestFetchModelIndex_DecodesPublishedList 验证拉取链路对服务端
// {object:"list", data:{category, updated_at, entries[]}} 契约的解码（前端 types.ts 同构字段）。
func TestFetchModelIndex_DecodesPublishedList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/model-index" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("category"); got != "overall" {
			t.Errorf("expected category=overall, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":{"category":"overall","updated_at":"2026-09-15T00:00:00Z","entries":[`+
			`{"id":"e1","category":"overall","rank":1,"model_name":"Test Model","provider":"TestProvider","provider_logo":"",`+
			`"release_date":"","eval_count":0,"evidence_status":"sufficient","input_price":1.5,"output_price":6,`+
			`"consensus_score":99,"detail_url":"","source_key":"","created_at":"2026-09-15T00:00:00Z","updated_at":"2026-09-15T00:00:00Z"}]}}`)
	}))
	defer ts.Close()
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)

	svc := &Service{}
	data, err := svc.FetchModelIndex(context.Background(), "overall")
	if err != nil {
		t.Fatalf("FetchModelIndex() returned error: %v", err)
	}
	if data["category"] != "overall" {
		t.Errorf("expected category overall, got %v", data["category"])
	}
	entries, ok := data["entries"].([]interface{})
	if !ok || len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %v", data["entries"])
	}
	entry, _ := entries[0].(map[string]interface{})
	for _, key := range []string{"rank", "model_name", "provider", "input_price", "output_price", "consensus_score", "evidence_status"} {
		if _, ok := entry[key]; !ok {
			t.Errorf("entry missing field %q required by frontend types", key)
		}
	}
	if entry["model_name"] != "Test Model" {
		t.Errorf("expected model_name Test Model, got %v", entry["model_name"])
	}
}

// TestFetchModelIndex_ServerError 验证 4xx/5xx 时透传服务端 error 字段（客户端页面据此显示错误态）。
func TestFetchModelIndex_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":"model index is not enabled"}`)
	}))
	defer ts.Close()
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)

	svc := &Service{}
	_, err := svc.FetchModelIndex(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
	if err.Error() != "model index is not enabled" {
		t.Errorf("expected server error message, got %q", err.Error())
	}
}

// TestFetchModelIndex_NetworkError 验证网络不可达时返回错误（页面降级为错误态而非崩溃）。
func TestFetchModelIndex_NetworkError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // 立即关闭，模拟服务端停机
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)

	svc := &Service{}
	if _, err := svc.FetchModelIndex(context.Background(), "overall"); err == nil {
		t.Fatal("expected error when server unreachable")
	}
}

// TestFetchModelIndex_LiveE2E 实机端到端拉取验证（Phase 5）：
// 设置 SEASAGI_E2E_BASE_URL 指向本地企业服务端（如 http://127.0.0.1:9318/api/v1）后运行，
// 校验客户端 5 个主分类 Tab（与前端 ModelIndexPage / 服务端 validCategories 一致）均可拉取、
// 合法分类不被回退为 overall，且非空分类条目包含前端消费的全部字段；未设置时跳过。
func TestFetchModelIndex_LiveE2E(t *testing.T) {
	base := os.Getenv("SEASAGI_E2E_BASE_URL")
	if base == "" {
		t.Skip("SEASAGI_E2E_BASE_URL not set; skipping live model-index pull")
	}
	t.Setenv("ENTERPRISE_API_BASE_URL", base)

	nonEmpty := 0
	svc := &Service{}
	for _, category := range []string{"overall", "coding", "reasoning", "knowledge", "professional"} {
		data, err := svc.FetchModelIndex(context.Background(), category)
		if err != nil {
			t.Fatalf("FetchModelIndex(%q) returned error: %v", category, err)
		}
		if data["category"] != category {
			t.Errorf("category %q: server returned category %v (unexpected fallback)", category, data["category"])
		}
		entries, ok := data["entries"].([]interface{})
		if !ok {
			t.Errorf("category %q: entries is not a list, got %T", category, data["entries"])
			continue
		}
		if len(entries) > 0 {
			nonEmpty++
		} else {
			t.Logf("category %s: 0 entries (empty state)", category)
			continue
		}
		entry, _ := entries[0].(map[string]interface{})
		for _, key := range []string{"rank", "model_name", "provider", "input_price", "output_price", "consensus_score", "evidence_status"} {
			if _, ok := entry[key]; !ok {
				t.Errorf("category %q first entry missing field %q", category, key)
			}
		}
		t.Logf("category %s: %d entries, top = %v (%v)", category, len(entries), entry["model_name"], entry["provider"])
	}
	if nonEmpty == 0 {
		t.Error("expected at least one category with published entries")
	}
}

func strPtr(s string) *string {
	return &s
}
