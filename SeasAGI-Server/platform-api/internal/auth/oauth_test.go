package auth

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

// TestMain 将 locales 目录固定到一个空目录：测试环境下 i18n.T 确定性地回退到键名，
// 断言不依赖开发者机器上的 SEASAGI_LOCALES_DIR 或工作目录。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "seasagi-locales-empty")
	if err == nil {
		_ = os.Setenv("SEASAGI_LOCALES_DIR", dir)
		code := m.Run()
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// performOAuthRequest 构造 gin 测试请求并调用 handler，返回响应记录器。
func performOAuthRequest(handler gin.HandlerFunc, method, target string, body io.Reader) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, body)
	if body != nil {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	handler(c)
	return w
}

// authorizeViaHandler 走 AuthorizeOAuth 创建一个会话并返回 session_id。
func authorizeViaHandler(t *testing.T, provider string) (sessionID, authorizeURL string) {
	t.Helper()
	w := performOAuthRequest(AuthorizeOAuth, http.MethodPost, "/api/v1/auth/oauth/authorize",
		strings.NewReader(`{"provider":"`+provider+`"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("authorize failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		SessionID    string `json:"session_id"`
		AuthorizeURL string `json:"authorize_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal authorize response failed: %v", err)
	}
	if resp.SessionID == "" || resp.AuthorizeURL == "" {
		t.Fatalf("unexpected authorize response: %s", w.Body.String())
	}
	return resp.SessionID, resp.AuthorizeURL
}

func TestOAuthProviderInfoJSON(t *testing.T) {
	info := OAuthProviderInfo{Name: "google", ClientID: "cid-123"}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var got OAuthProviderInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Name != "google" || got.ClientID != "cid-123" {
		t.Fatalf("unexpected round-trip result: %+v", got)
	}
	if string(data) != `{"name":"google","client_id":"cid-123"}` {
		t.Fatalf("unexpected JSON: %s", data)
	}
}

func TestListConfiguredProviders(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "g-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "g-secret")
	t.Setenv("GITHUB_CLIENT_ID", "h-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "h-secret")

	providers := listConfiguredProviders()
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d: %+v", len(providers), providers)
	}
	if providers[0].Name != "google" || providers[0].ClientID != "g-id" {
		t.Fatalf("unexpected google provider: %+v", providers[0])
	}
	if providers[1].Name != "github" || providers[1].ClientID != "h-id" {
		t.Fatalf("unexpected github provider: %+v", providers[1])
	}

	// 缺少 secret 的 provider 不应下发。
	t.Setenv("GITHUB_CLIENT_SECRET", "")
	providers = listConfiguredProviders()
	if len(providers) != 1 || providers[0].Name != "google" {
		t.Fatalf("expected only google, got %+v", providers)
	}
}

func TestAuthorizeOAuthCreatesSession(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "g-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "g-secret")
	t.Setenv("PUBLIC_BASE_URL", "http://127.0.0.1:9318")

	sessionID, authorizeURL := authorizeViaHandler(t, "google")
	if !strings.HasPrefix(authorizeURL, "http://127.0.0.1:9318/api/v1/auth/oauth/start?session_id=") {
		t.Fatalf("unexpected authorize_url: %q", authorizeURL)
	}

	sess, ok := getOAuthSession(sessionID)
	if !ok {
		t.Fatal("session not saved")
	}
	if sess.Provider != "google" || sess.Status != "pending" {
		t.Fatalf("unexpected session: %+v", sess)
	}
	if sess.State == "" || sess.CodeVerifier == "" {
		t.Fatalf("expected state and google PKCE verifier: %+v", sess)
	}
}

func TestAuthorizeOAuthGitHubHasNoVerifier(t *testing.T) {
	t.Setenv("GITHUB_CLIENT_ID", "h-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "h-secret")

	sessionID, _ := authorizeViaHandler(t, "github")
	sess, ok := getOAuthSession(sessionID)
	if !ok {
		t.Fatal("session not saved")
	}
	if sess.Provider != "github" || sess.CodeVerifier != "" {
		t.Fatalf("github OAuth App should not use PKCE: %+v", sess)
	}
}

func TestAuthorizeOAuthUnsupportedProvider(t *testing.T) {
	w := performOAuthRequest(AuthorizeOAuth, http.MethodPost, "/api/v1/auth/oauth/authorize",
		strings.NewReader(`{"provider":"microsoft"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	// 测试环境 locales 为空，i18n 回退返回键名。
	if !strings.Contains(w.Body.String(), "auth.oauthUnsupportedProvider") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestAuthorizeOAuthProviderNotConfigured(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("GOOGLE_CLIENT_SECRET", "")

	w := performOAuthRequest(AuthorizeOAuth, http.MethodPost, "/api/v1/auth/oauth/authorize",
		strings.NewReader(`{"provider":"google"}`))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "auth.oauthUnavailable") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestStartOAuthRedirectsToProvider(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "g-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "g-secret")
	t.Setenv("PUBLIC_BASE_URL", "http://127.0.0.1:9318")

	sessionID, _ := authorizeViaHandler(t, "google")
	sess, _ := getOAuthSession(sessionID)

	w := performOAuthRequest(StartOAuth, http.MethodGet, "/api/v1/auth/oauth/start?session_id="+sessionID, nil)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://accounts.google.com/o/oauth2/v2/auth?") {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
	for _, want := range []string{
		"client_id=g-id",
		"response_type=code",
		"scope=openid+email+profile",
		"code_challenge_method=S256",
		"code_challenge=" + oauthS256CodeChallenge(sess.CodeVerifier),
		"state=" + sess.State,
		"redirect_uri=" + url.QueryEscape("http://127.0.0.1:9318/api/v1/auth/oauth/callback"),
	} {
		if !strings.Contains(loc, want) {
			t.Fatalf("redirect missing %q: %q", want, loc)
		}
	}
}

func TestStartOAuthInvalidSession(t *testing.T) {
	w := performOAuthRequest(StartOAuth, http.MethodGet, "/api/v1/auth/oauth/start?session_id=nope", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "auth.oauthPageInvalidSession") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestOAuthCallbackInvalidState(t *testing.T) {
	w := performOAuthRequest(OAuthCallback, http.MethodGet, "/api/v1/auth/oauth/callback?code=x&state=bogus", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "auth.oauthPageInvalidSession") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestOAuthCallbackUserDenied(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "g-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "g-secret")

	sessionID, _ := authorizeViaHandler(t, "google")
	sess, _ := getOAuthSession(sessionID)

	w := performOAuthRequest(OAuthCallback, http.MethodGet,
		"/api/v1/auth/oauth/callback?error=access_denied&state="+sess.State, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 html, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "auth.oauthPageFailed") {
		t.Fatalf("expected failure page, got: %s", w.Body.String())
	}

	// 会话标记失败，客户端轮询应得到 failed 与原因。
	w = performOAuthRequest(PollOAuth, http.MethodPost, "/api/v1/auth/oauth/poll",
		strings.NewReader(`{"session_id":"`+sessionID+`"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("poll failed: %d %s", w.Code, w.Body.String())
	}
	var poll struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &poll); err != nil {
		t.Fatalf("unmarshal poll response failed: %v", err)
	}
	if poll.Status != "failed" || poll.Error != "auth.oauthDenied" {
		t.Fatalf("unexpected poll result: %+v", poll)
	}
}

func TestPollOAuthPending(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "g-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "g-secret")

	sessionID, _ := authorizeViaHandler(t, "google")
	w := performOAuthRequest(PollOAuth, http.MethodPost, "/api/v1/auth/oauth/poll",
		strings.NewReader(`{"session_id":"`+sessionID+`"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("poll failed: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"pending"`) {
		t.Fatalf("expected pending, got: %s", w.Body.String())
	}
}

func TestPollOAuthCompleted(t *testing.T) {
	saveOAuthSession(&oauthSession{
		SessionID: "sess-complete", Provider: "google", State: "st-complete",
		Status: "pending", CreatedAt: time.Now(),
	})
	completeOAuthSession("sess-complete", "access-1", "refresh-1")

	w := performOAuthRequest(PollOAuth, http.MethodPost, "/api/v1/auth/oauth/poll",
		strings.NewReader(`{"session_id":"sess-complete"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("poll failed: %d %s", w.Code, w.Body.String())
	}
	var poll struct {
		Status       string `json:"status"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &poll); err != nil {
		t.Fatalf("unmarshal poll response failed: %v", err)
	}
	if poll.Status != "completed" || poll.AccessToken != "access-1" || poll.RefreshToken != "refresh-1" || poll.ExpiresIn != 86400 {
		t.Fatalf("unexpected poll result: %+v", poll)
	}
}

func TestPollOAuthUnknownSession(t *testing.T) {
	w := performOAuthRequest(PollOAuth, http.MethodPost, "/api/v1/auth/oauth/poll",
		strings.NewReader(`{"session_id":"missing"}`))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "auth.oauthSessionNotFound") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

// TestPublicBaseURLFallback 验证社区版 publicBaseURL 的三级回退：
// PUBLIC_BASE_URL -> API_BASE_URL -> 默认本地地址。
func TestPublicBaseURLFallback(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "http://public.example.com")
	t.Setenv("API_BASE_URL", "http://api.example.com")
	if got := publicBaseURL(); got != "http://public.example.com" {
		t.Fatalf("expected PUBLIC_BASE_URL, got %q", got)
	}

	t.Setenv("PUBLIC_BASE_URL", "")
	if got := publicBaseURL(); got != "http://api.example.com" {
		t.Fatalf("expected API_BASE_URL fallback, got %q", got)
	}

	t.Setenv("API_BASE_URL", "")
	if got := publicBaseURL(); got != "http://localhost:9318" {
		t.Fatalf("expected default fallback, got %q", got)
	}

	t.Setenv("PUBLIC_BASE_URL", "http://trailingslash.example.com/")
	if got := publicBaseURL(); got != "http://trailingslash.example.com" {
		t.Fatalf("expected trailing slash trimmed, got %q", got)
	}
}

// RFC 7636 Appendix B 的官方 PKCE S256 测试向量。
func TestOAuthS256CodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := oauthS256CodeChallenge(verifier); got != want {
		t.Fatalf("unexpected challenge: got %q want %q", got, want)
	}
}

func TestExchangeAuthorizationCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept: application/json, got %q", r.Header.Get("Accept"))
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form failed: %v", err)
		}
		if r.Form.Get("code") != "auth-code" {
			t.Errorf("unexpected code: %q", r.Form.Get("code"))
		}
		if r.Form.Get("redirect_uri") != "http://127.0.0.1:55331/callback" {
			t.Errorf("unexpected redirect_uri: %q", r.Form.Get("redirect_uri"))
		}
		if r.Form.Get("code_verifier") != "verifier-1" {
			t.Errorf("unexpected code_verifier: %q", r.Form.Get("code_verifier"))
		}
		if r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "csecret" {
			t.Errorf("unexpected client credentials: %q/%q", r.Form.Get("client_id"), r.Form.Get("client_secret"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "provider-token"})
	}))
	defer ts.Close()

	cfg := &oauthProviderConfig{Name: "google", ClientID: "cid", ClientSecret: "csecret", TokenURL: ts.URL}
	token, err := exchangeAuthorizationCode(cfg, "auth-code", "http://127.0.0.1:55331/callback", "verifier-1")
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}
	if token != "provider-token" {
		t.Fatalf("unexpected token: %q", token)
	}
}

func TestExchangeAuthorizationCodeError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer ts.Close()

	cfg := &oauthProviderConfig{Name: "google", ClientID: "cid", ClientSecret: "csecret", TokenURL: ts.URL}
	if _, err := exchangeAuthorizationCode(cfg, "bad", "http://127.0.0.1:55331/callback", ""); err == nil {
		t.Fatal("expected error for non-200 token response")
	}
}

func TestFetchUserInfoGoogle(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer g-token" {
			t.Errorf("unexpected Authorization: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"sub-42","email":"User@Example.com","email_verified":true}`))
	}))
	defer ts.Close()

	cfg := &oauthProviderConfig{Name: "google", UserInfoURL: ts.URL}
	info, err := fetchUserInfo(cfg, "g-token")
	if err != nil {
		t.Fatalf("fetch user info failed: %v", err)
	}
	if info.ProviderUserID != "sub-42" || info.Email != "User@Example.com" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestFetchUserInfoGitHubEmailFallback(t *testing.T) {
	userTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":9001,"login":"octo","email":null}`))
	}))
	defer userTS.Close()

	emailTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"email":"private@x.com","primary":false,"verified":true},{"email":"main@x.com","primary":true,"verified":true}]`))
	}))
	defer emailTS.Close()

	cfg := &oauthProviderConfig{Name: "github", UserInfoURL: userTS.URL, EmailsURL: emailTS.URL}
	info, err := fetchUserInfo(cfg, "h-token")
	if err != nil {
		t.Fatalf("fetch user info failed: %v", err)
	}
	if info.ProviderUserID != "9001" {
		t.Fatalf("unexpected provider user id: %q", info.ProviderUserID)
	}
	if info.Email != "main@x.com" {
		t.Fatalf("expected primary verified email, got %q", info.Email)
	}
}

func TestFetchUserInfoMissingEmail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"sub-1"}`))
	}))
	defer ts.Close()

	cfg := &oauthProviderConfig{Name: "google", UserInfoURL: ts.URL}
	if _, err := fetchUserInfo(cfg, "g-token"); err == nil {
		t.Fatal("expected error when email missing")
	}
}

// openTestDB 打开内存 SQLite 并建立 OAuth 相关最小表结构（与社区版 users 表对齐，无 tenant_id）。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	for _, driverName := range []string{"sqlite", "sqlite3"} {
		db, err := sql.Open(driverName, ":memory:")
		if err != nil {
			continue
		}
		if _, err := db.Exec(`
			CREATE TABLE users (
				user_id TEXT PRIMARY KEY,
				email TEXT UNIQUE NOT NULL,
				hashed_password TEXT NOT NULL
			);
			CREATE TABLE oauth_identities (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id TEXT NOT NULL,
				provider TEXT NOT NULL,
				provider_user_id TEXT NOT NULL,
				email TEXT NOT NULL DEFAULT '',
				UNIQUE(provider, provider_user_id)
			);
		`); err == nil {
			return db
		}
		db.Close()
	}
	t.Skip("no sqlite driver available for tests")
	return nil
}

func TestFindOrCreateUserByIdentity(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	prev := database.DB
	database.DB = db
	defer func() { database.DB = prev }()

	// 1. 新建用户。
	userID1, err := findOrCreateUserByIdentity("google", "sub-1", "Alice@Example.com")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if userID1 == "" {
		t.Fatal("expected non-empty user id")
	}
	var email, hashed string
	if err := db.QueryRow(`SELECT email, hashed_password FROM users WHERE user_id = ?`, userID1).Scan(&email, &hashed); err != nil {
		t.Fatalf("user not persisted: %v", err)
	}
	if email != "alice@example.com" {
		t.Fatalf("expected lowercased email, got %q", email)
	}
	if hashed == "" {
		t.Fatal("expected non-empty placeholder hashed_password")
	}

	// 2. 同 identity 再次登录复用账号。
	userID2, err := findOrCreateUserByIdentity("google", "sub-1", "alice@example.com")
	if err != nil {
		t.Fatalf("reuse failed: %v", err)
	}
	if userID2 != userID1 {
		t.Fatalf("expected identity reuse, got %s vs %s", userID1, userID2)
	}

	// 3. 同邮箱不同 identity -> 绑定已有账号。
	userID3, err := findOrCreateUserByIdentity("github", "9001", "alice@example.com")
	if err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	if userID3 != userID1 {
		t.Fatalf("expected email binding to existing user, got %s vs %s", userID1, userID3)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user after binding, got %d", count)
	}

	// 4. 新邮箱新 identity -> 再建一个用户。
	if _, err := findOrCreateUserByIdentity("github", "9002", "bob@example.com"); err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 users, got %d", count)
	}
}
