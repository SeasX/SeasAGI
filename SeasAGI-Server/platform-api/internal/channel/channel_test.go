package channel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

func setupRepo(t *testing.T) (*Repository, *sql.DB) {
	t.Helper()
	t.Setenv("SEASAGI_DATA_KEY", "channel-test-data-key-0123456789abcdef")
	if err := database.Init(filepath.Join(t.TempDir(), "channel.db")); err != nil {
		t.Fatalf("database.Init: %v", err)
	}
	t.Cleanup(database.Close)
	db := database.DB
	return NewRepository(func() *sql.DB { return db }), db
}

func newTestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/channels", h.ListChannels)
	r.GET("/channels/:id", h.GetChannel)
	r.POST("/channels", h.CreateChannel)
	r.PUT("/channels/:id", h.UpdateChannel)
	r.DELETE("/channels/:id", h.DeleteChannel)
	r.GET("/internal/relay/config", h.InternalRelayConfig)
	return r
}

func do(t *testing.T, r http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 任何渠道接口响应都不得包含凭证明文或密文（此前 Create/Update 会回显 encrypted_api_key）。
func TestChannelResponsesNeverExposeCredentials(t *testing.T) {
	repo, _ := setupRepo(t)
	r := newTestRouter(NewHandler(repo))

	create := do(t, r, http.MethodPost, "/channels", `{"channel_id":"c1","provider_type":"openai","display_name":"C1","base_url":"https://u","api_key":"sk-top-secret","models":[{"model_id":"gpt-4o","model_name":"GPT-4o"}]}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", create.Code, create.Body)
	}
	update := do(t, r, http.MethodPut, "/channels/c1", `{"api_key":"sk-rotated-secret"}`)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"create": create,
		"update": update,
		"get":    do(t, r, http.MethodGet, "/channels/c1", ""),
		"list":   do(t, r, http.MethodGet, "/channels", ""),
	} {
		body := w.Body.String()
		if w.Code >= 300 || strings.Contains(body, "sk-") || strings.Contains(body, "encrypted_api_key") || strings.Contains(body, "v2:") {
			t.Fatalf("%s response leaks credential material (%d): %s", name, w.Code, body)
		}
	}
}

func TestCreateConflictAndValidation(t *testing.T) {
	repo, _ := setupRepo(t)
	r := newTestRouter(NewHandler(repo))
	body := `{"channel_id":"dup","provider_type":"openai","display_name":"D","base_url":"https://u"}`
	if w := do(t, r, http.MethodPost, "/channels", body); w.Code != http.StatusCreated {
		t.Fatalf("first create = %d", w.Code)
	}
	if w := do(t, r, http.MethodPost, "/channels", body); w.Code != http.StatusConflict {
		t.Fatalf("duplicate create = %d, want 409", w.Code)
	}
	if w := do(t, r, http.MethodPut, "/channels/missing", `{"display_name":"x"}`); w.Code != http.StatusNotFound {
		t.Fatalf("update missing = %d, want 404", w.Code)
	}
	if w := do(t, r, http.MethodDelete, "/channels/missing", ""); w.Code != http.StatusNotFound {
		t.Fatalf("delete missing = %d, want 404", w.Code)
	}
}

// 加密失败时拒绝创建，而不是静默丢弃凭证。
func TestCreateFailsWhenCredentialCannotBeEncrypted(t *testing.T) {
	repo, db := setupRepo(t)
	h := NewHandler(repo)
	h.encrypt = func(string) (string, error) { return "", errors.New("no key") }
	r := newTestRouter(h)
	w := do(t, r, http.MethodPost, "/channels", `{"channel_id":"c1","provider_type":"openai","display_name":"C","base_url":"https://u","api_key":"sk-x"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("create = %d, want 500", w.Code)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM channels`).Scan(&n)
	if n != 0 {
		t.Fatal("channel row written although the credential was rejected")
	}
}

// 多表写入原子化：模型写入失败时渠道行也必须回滚。
func TestRepositoryCreateIsAtomic(t *testing.T) {
	repo, db := setupRepo(t)
	if _, err := db.Exec(`CREATE TRIGGER fail_model BEFORE INSERT ON channel_models BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	err := repo.Create(context.Background(), ChannelRecord{ChannelID: "c1", ChannelType: "platform", ProviderType: "openai", DisplayName: "C", BaseURL: "https://u", Enabled: true},
		"v2:cipher", []ModelInfo{{ModelID: "m"}})
	if err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("Create err = %v", err)
	}
	for _, table := range []string{"channels", "channel_strategies", "channel_models"} {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
		if n != 0 {
			t.Fatalf("%s has %d rows after rolled-back create", table, n)
		}
	}
}

// 删除渠道同时删除其凭证，避免同 ID 重建时复活旧凭证。
func TestRepositoryDeleteRemovesCredential(t *testing.T) {
	repo, db := setupRepo(t)
	ctx := context.Background()
	rec := ChannelRecord{ChannelID: "c1", ChannelType: "platform", ProviderType: "openai", DisplayName: "C", BaseURL: "https://u", Enabled: true}
	if err := repo.Create(ctx, rec, "v2:old", nil); err != nil {
		t.Fatal(err)
	}
	if found, err := repo.Delete(ctx, "c1"); err != nil || !found {
		t.Fatalf("Delete = %v, %v", found, err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM channel_strategies`).Scan(&n)
	if n != 0 {
		t.Fatal("credential survived channel deletion")
	}
	if err := repo.Create(ctx, rec, "", nil); err != nil {
		t.Fatal(err)
	}
	ch, _ := repo.Get(ctx, "c1")
	if ch.EncryptedAPIKey != "" {
		t.Fatal("re-created channel inherited the deleted channel's credential")
	}
}

func TestInternalRelayConfigContentAndETag(t *testing.T) {
	repo, db := setupRepo(t)
	r := newTestRouter(NewHandler(repo))
	do(t, r, http.MethodPost, "/channels", `{"channel_id":"a","provider_type":"openai","display_name":"A","base_url":"https://a","api_key":"sk-a","models":[{"model_id":"gpt-4o","model_name":"GPT-4o"}]}`)
	do(t, r, http.MethodPost, "/channels", `{"channel_id":"b","provider_type":"openai","display_name":"B","base_url":"https://b","enabled":false}`)
	// 策略级关闭也应体现为 enabled=false。
	if _, err := db.Exec(`UPDATE channel_strategies SET enabled = 0, weight = 5 WHERE channel_id = 'a'`); err != nil {
		t.Fatal(err)
	}

	w := do(t, r, http.MethodGet, "/internal/relay/config", "")
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("relay config = %d cache=%q", w.Code, w.Header().Get("Cache-Control"))
	}
	var cfg RelayConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SchemaVersion != RelayConfigSchemaVersion || cfg.Version == "" || len(cfg.Channels) != 2 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	byID := map[string]RelayChannelConfig{}
	for _, ch := range cfg.Channels {
		byID[ch.ChannelID] = ch
	}
	if a := byID["a"]; a.APIKey != "sk-a" || a.Enabled || a.Weight != 5 || len(a.Models) != 1 || a.Models[0] != "gpt-4o" {
		t.Fatalf("channel a = %+v", a)
	}
	if b := byID["b"]; b.APIKey != "" || b.Enabled || b.Weight != 100 {
		t.Fatalf("channel b = %+v", b)
	}
	if strings.Contains(w.Header().Get("ETag"), "sk-a") {
		t.Fatal("ETag derived from plaintext")
	}

	notModified := do(t, r, http.MethodGet, "/internal/relay/config", "", "If-None-Match", w.Header().Get("ETag"))
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("If-None-Match = %d body=%q", notModified.Code, notModified.Body)
	}
	do(t, r, http.MethodPut, "/channels/a", `{"base_url":"https://a2"}`)
	changed := do(t, r, http.MethodGet, "/internal/relay/config", "", "If-None-Match", w.Header().Get("ETag"))
	if changed.Code != http.StatusOK {
		t.Fatalf("config change not reflected: %d", changed.Code)
	}
}

func TestInternalRelayConfigMarksUndecryptableCredential(t *testing.T) {
	repo, db := setupRepo(t)
	r := newTestRouter(NewHandler(repo))
	do(t, r, http.MethodPost, "/channels", `{"channel_id":"a","provider_type":"openai","display_name":"A","base_url":"https://a"}`)
	if _, err := db.Exec(`INSERT INTO channel_strategies (channel_id, encrypted_api_key) VALUES ('a', 'v2:not-valid-ciphertext')`); err != nil {
		t.Fatal(err)
	}
	w := do(t, r, http.MethodGet, "/internal/relay/config", "")
	var cfg RelayConfig
	_ = json.Unmarshal(w.Body.Bytes(), &cfg)
	if len(cfg.Channels) != 1 || cfg.Channels[0].CredentialError != credentialUnavailable || cfg.Channels[0].APIKey != "" {
		t.Fatalf("undecryptable credential not flagged: %+v", cfg.Channels)
	}
}
