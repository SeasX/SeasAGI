package channel

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testRelayKey  = "relay-test-key-0123456789abcdef-0123456789"
	otherRelayKey = "relay-other-key-0123456789abcdef-012345678"
)

func setRelayKey(t *testing.T, key string) {
	t.Helper()
	t.Setenv("RELAY_DATA_KEY", key)
	t.Setenv("SEASAGI_DATA_KEY", "")
}

func rawDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := openSQLite(filepath.Join(dir, dbFileName))
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func rawAPIKey(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var v string
	if err := db.QueryRow(`SELECT api_key FROM relay_channels WHERE channel_id = ?`, id).Scan(&v); err != nil {
		t.Fatalf("read api_key for %s: %v", id, err)
	}
	return v
}

func countRows(t *testing.T, db *sql.DB, where string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM relay_channels `+where, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// 凭证落库必须是密文，且重启后可解密还原。
func TestCredentialsEncryptedAtRest(t *testing.T) {
	setRelayKey(t, testRelayKey)
	dir := t.TempDir()
	s := mustStore(t, dir)
	s.AddChannel(Channel{ChannelID: "c1", ProviderType: "openai", BaseURL: "https://u", APIKey: "sk-live-secret", Enabled: true})
	if err := s.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	stored := rawAPIKey(t, rawDB(t, dir), "c1")
	if !strings.HasPrefix(stored, relayCipherPrefix) || strings.Contains(stored, "sk-live-secret") {
		t.Fatalf("api_key stored as %q, want relay ciphertext", stored)
	}

	re := mustStore(t, dir)
	defer re.Close()
	ch, ok := re.GetChannel("c1")
	if !ok || ch.APIKey != "sk-live-secret" || ch.CredentialError != "" {
		t.Fatalf("reloaded channel = %+v", ch)
	}
}

// 升级前的老库（无新列、明文凭证）在有密钥时启动即迁移为密文，且不丢数据。
func TestLegacyPlaintextMigratedOnStartup(t *testing.T) {
	setRelayKey(t, testRelayKey)
	dir := t.TempDir()
	legacy := rawDB(t, dir)
	for _, stmt := range []string{
		`CREATE TABLE relay_channels (channel_id TEXT PRIMARY KEY, provider_type TEXT NOT NULL, display_name TEXT NOT NULL, base_url TEXT NOT NULL, api_key TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1, models_json TEXT NOT NULL DEFAULT '[]', weight INTEGER NOT NULL DEFAULT 100, priority INTEGER NOT NULL DEFAULT 0, gray_percent INTEGER NOT NULL DEFAULT 0, read_only INTEGER NOT NULL DEFAULT 0, extra_json TEXT NOT NULL DEFAULT '{}', healthy INTEGER NOT NULL DEFAULT 1, status_code INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '', consec_fail INTEGER NOT NULL DEFAULT 0, last_check DATETIME)`,
		`INSERT INTO relay_channels (channel_id, provider_type, display_name, base_url, api_key, models_json) VALUES ('old', 'openai', 'Old', 'https://old', 'sk-plain-legacy', '["gpt-4o"]')`,
	} {
		if _, err := legacy.Exec(stmt); err != nil {
			t.Fatalf("seed legacy db: %v", err)
		}
	}
	_ = legacy.Close()

	s := mustStore(t, dir)
	ch, ok := s.GetChannel("old")
	if !ok || ch.APIKey != "sk-plain-legacy" || len(ch.Models) != 1 {
		t.Fatalf("legacy channel not loaded intact: %+v", ch)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	stored := rawAPIKey(t, rawDB(t, dir), "old")
	if !strings.HasPrefix(stored, relayCipherPrefix) {
		t.Fatalf("legacy credential not migrated, stored = %q", stored)
	}
}

// 没有加密密钥时：历史明文保持原样（不删除用户数据），新凭证绝不以明文落盘。
func TestNoKeyNeverWritesPlaintext(t *testing.T) {
	t.Setenv("RELAY_DATA_KEY", "")
	t.Setenv("SEASAGI_DATA_KEY", "")
	dir := t.TempDir()
	s := mustStore(t, dir)
	s.AddChannel(Channel{ChannelID: "n1", ProviderType: "openai", BaseURL: "https://u", APIKey: "sk-new-secret", Enabled: true})
	if err := s.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	ch, _ := s.GetChannel("n1")
	if ch.APIKey != "sk-new-secret" {
		t.Fatal("credential must stay usable in memory")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if v := rawAPIKey(t, rawDB(t, dir), "n1"); v != "" {
		t.Fatalf("credential written without encryption key: %q", v)
	}
}

// 密钥错误时：渠道不参与路由、原密文不被覆盖；换回正确密钥后恢复。
func TestDecryptFailureFailsSafe(t *testing.T) {
	setRelayKey(t, testRelayKey)
	dir := t.TempDir()
	s := mustStore(t, dir)
	s.AddChannel(Channel{ChannelID: "c1", ProviderType: "openai", BaseURL: "https://u", APIKey: "sk-secret", Enabled: true, Models: []string{"m"}, Health: HealthStatus{Healthy: true}})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	original := rawAPIKey(t, rawDB(t, dir), "c1")

	setRelayKey(t, otherRelayKey)
	wrong := mustStore(t, dir)
	ch, ok := wrong.GetChannel("c1")
	if !ok || ch.CredentialError == "" || ch.APIKey != "" || ch.Routable() {
		t.Fatalf("channel with undecryptable credential must be unroutable: %+v", ch)
	}
	if got, _ := wrong.ResolveChannel("m"); got != nil {
		t.Fatal("ResolveChannel returned a channel whose credential cannot be decrypted")
	}
	if len(wrong.ListEnabledChannels()) != 0 {
		t.Fatal("credential-broken channel must not be health-checked")
	}
	// 其他字段变化触发重写，密文必须原样保留。
	wrong.UpdateHealth("c1", HealthStatus{Healthy: false, ConsecFail: 1})
	if err := wrong.Close(); err != nil {
		t.Fatal(err)
	}
	if got := rawAPIKey(t, rawDB(t, dir), "c1"); got != original {
		t.Fatal("ciphertext was overwritten while the key was wrong")
	}

	setRelayKey(t, testRelayKey)
	fixed := mustStore(t, dir)
	defer fixed.Close()
	if ch, _ := fixed.GetChannel("c1"); ch.APIKey != "sk-secret" || ch.CredentialError != "" {
		t.Fatalf("channel not recovered after key fix: %+v", ch)
	}
}

// 事务中途失败必须整体回滚，库中保持上次提交的完整状态；故障排除后重试成功。
func TestPersistRollbackAndRecovery(t *testing.T) {
	setRelayKey(t, testRelayKey)
	dir := t.TempDir()
	s := mustStore(t, dir)
	defer s.Close()
	s.AddChannel(Channel{ChannelID: "a", ProviderType: "openai", BaseURL: "https://a", Enabled: true, Weight: 10})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	if _, err := s.db.Exec(`CREATE TRIGGER inject_fail BEFORE INSERT ON relay_channels WHEN NEW.channel_id = 'boom' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	// 同一批次：更新 a、新增 ok 与 boom、新增映射。boom 失败后全部回滚。
	s.UpdateChannelPolicy("a", 99, -1, -1, nil, nil)
	s.AddChannel(Channel{ChannelID: "boom", ProviderType: "openai", BaseURL: "https://b", Enabled: true})
	s.AddChannel(Channel{ChannelID: "ok", ProviderType: "openai", BaseURL: "https://o", Enabled: true})
	s.AddMapping(ModelMapping{LogicalModel: "m", ChannelID: "ok", UpstreamModel: "m-up"})

	err := s.Flush()
	if err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("flush error = %v, want injected failure", err)
	}
	db := rawDB(t, dir)
	if n := countRows(t, db, `WHERE channel_id IN ('ok', 'boom')`); n != 0 {
		t.Fatalf("partial write survived rollback: %d rows", n)
	}
	var weight int
	if err := db.QueryRow(`SELECT weight FROM relay_channels WHERE channel_id = 'a'`).Scan(&weight); err != nil || weight != 10 {
		t.Fatalf("existing row changed by failed tx: weight=%d err=%v", weight, err)
	}
	var mappings int
	_ = db.QueryRow(`SELECT COUNT(*) FROM relay_model_mappings`).Scan(&mappings)
	if mappings != 0 {
		t.Fatal("mapping written by failed tx")
	}

	if _, err := s.db.Exec(`DROP TRIGGER inject_fail`); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatalf("retry after recovery failed: %v", err)
	}
	if n := countRows(t, db, ``); n != 3 {
		t.Fatalf("after recovery want 3 rows, got %d", n)
	}
	_ = db.QueryRow(`SELECT weight FROM relay_channels WHERE channel_id = 'a'`).Scan(&weight)
	_ = db.QueryRow(`SELECT COUNT(*) FROM relay_model_mappings`).Scan(&mappings)
	if weight != 99 || mappings != 1 {
		t.Fatalf("after recovery weight=%d mappings=%d", weight, mappings)
	}
}

// 落库失败（如关闭期间）必须通过 Close 返回，而不是静默吞掉。
func TestCloseReportsPersistFailure(t *testing.T) {
	setRelayKey(t, testRelayKey)
	s := mustStore(t, t.TempDir())
	if _, err := s.db.Exec(`CREATE TRIGGER inject_fail BEFORE INSERT ON relay_channels BEGIN SELECT RAISE(ABORT, 'disk gone'); END`); err != nil {
		t.Fatal(err)
	}
	s.AddChannel(Channel{ChannelID: "x", ProviderType: "openai", BaseURL: "https://x"})
	if err := s.Close(); err == nil || !strings.Contains(err.Error(), "disk gone") {
		t.Fatalf("Close() = %v, want persist error", err)
	}
	if err := s.Flush(); err != errStoreClosed {
		t.Fatalf("Flush after close = %v", err)
	}
}

// 只重写有变化的行：一次健康更新不应触碰其他渠道。
func TestPersistOnlyTouchesChangedRows(t *testing.T) {
	setRelayKey(t, testRelayKey)
	s := mustStore(t, t.TempDir())
	defer s.Close()
	for _, id := range []string{"a", "b", "c"} {
		s.AddChannel(Channel{ChannelID: id, ProviderType: "openai", BaseURL: "https://" + id, APIKey: "sk-" + id, Enabled: true})
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE write_audit (channel_id TEXT)`,
		`CREATE TRIGGER audit_update AFTER UPDATE ON relay_channels BEGIN INSERT INTO write_audit VALUES (NEW.channel_id); END`,
		`CREATE TRIGGER audit_delete AFTER DELETE ON relay_channels BEGIN INSERT INTO write_audit VALUES ('deleted:' || OLD.channel_id); END`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	s.UpdateHealth("b", HealthStatus{Healthy: false, StatusCode: 503, ConsecFail: 1})
	s.RemoveChannel("c")
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil { // 无变化时为空操作
		t.Fatal(err)
	}
	rows, err := s.db.Query(`SELECT channel_id FROM write_audit ORDER BY channel_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var touched []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		touched = append(touched, id)
	}
	if strings.Join(touched, ",") != "b,deleted:c" {
		t.Fatalf("rows touched = %v, want only b updated and c deleted", touched)
	}
}

// 数据库为空时一次性导入历史 JSON；原文件保持不变，且不再被写入。
func TestLegacyJSONImportedOnceAndLeftUntouched(t *testing.T) {
	setRelayKey(t, testRelayKey)
	dir := t.TempDir()
	legacyJSON := []byte(`{"channels":[{"channel_id":"j1","provider_type":"openai","display_name":"J","base_url":"https://j","api_key":"sk-json","enabled":true,"models":["gpt-4o"]}],"mappings":[{"logical_model":"gpt","channel_id":"j1","upstream_model":"gpt-4o"}]}`)
	jsonPath := filepath.Join(dir, "relay-channels.json")
	if err := os.WriteFile(jsonPath, legacyJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	s := mustStore(t, dir)
	ch, ok := s.GetChannel("j1")
	if !ok || ch.APIKey != "sk-json" || ch.Weight != 100 {
		t.Fatalf("json channel not imported: %+v", ch)
	}
	if got, up := s.ResolveChannel("gpt"); got != nil || up != "" {
		// j1 尚未健康探测（healthy=false），映射不应命中；这里只验证映射已导入。
		t.Fatalf("unexpected resolve before health check: %+v", got)
	}
	s.AddChannel(Channel{ChannelID: "later", ProviderType: "openai", BaseURL: "https://l"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	after, _ := os.ReadFile(jsonPath)
	if !bytes.Equal(after, legacyJSON) {
		t.Fatal("legacy JSON file was modified")
	}
	db := rawDB(t, dir)
	if v := rawAPIKey(t, db, "j1"); !strings.HasPrefix(v, relayCipherPrefix) {
		t.Fatalf("imported credential not encrypted: %q", v)
	}
	var mappings int
	_ = db.QueryRow(`SELECT COUNT(*) FROM relay_model_mappings`).Scan(&mappings)
	if mappings != 1 {
		t.Fatalf("mapping not imported, got %d", mappings)
	}
}

func TestResolveDataDirPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RELAY_DATA_DIR", "")
	t.Setenv("DATA_DIR", "")
	if got, err := ResolveDataDir(); err != nil || got != filepath.Join(home, legacyDataDirName) {
		t.Fatalf("default = %q, %v", got, err)
	}
	generic := t.TempDir()
	t.Setenv("DATA_DIR", generic)
	if got, _ := ResolveDataDir(); got != generic {
		t.Fatalf("DATA_DIR not honoured: %q", got)
	}
	specific := t.TempDir()
	t.Setenv("RELAY_DATA_DIR", specific)
	if got, _ := ResolveDataDir(); got != specific {
		t.Fatalf("RELAY_DATA_DIR must win: %q", got)
	}
}

// 配置新目录时不得移动/删除 $HOME/.seasagi 下的旧库。
func TestNewDataDirLeavesLegacyDatabaseAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacyDir := filepath.Join(home, legacyDataDirName)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyDB := filepath.Join(legacyDir, dbFileName)
	if err := os.WriteFile(legacyDB, []byte("not-touched"), 0o600); err != nil {
		t.Fatal(err)
	}
	newDir := t.TempDir()
	t.Setenv("RELAY_DATA_DIR", newDir)
	setRelayKey(t, testRelayKey)

	s, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if s.DataDir() != newDir {
		t.Fatalf("store dir = %s", s.DataDir())
	}
	_ = s.Close()
	if b, _ := os.ReadFile(legacyDB); string(b) != "not-touched" {
		t.Fatal("legacy database was modified")
	}
}
