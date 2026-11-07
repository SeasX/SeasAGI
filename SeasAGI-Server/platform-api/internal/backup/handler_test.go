package backup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// initTestDB 在临时目录初始化数据库并返回库路径。
func initTestDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "platform-api.db")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database init failed: %v", err)
	}
	t.Cleanup(database.Close)
	return dbPath
}

func insertUser(t *testing.T, userID, email string) {
	t.Helper()
	if _, err := database.DB.Exec(
		`INSERT INTO users (user_id, email, hashed_password) VALUES (?, ?, ?)`,
		userID, email, "hashed",
	); err != nil {
		t.Fatalf("insert user %s failed: %v", userID, err)
	}
}

func userExists(t *testing.T, userID string) bool {
	t.Helper()
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatalf("count user failed: %v", err)
	}
	return count > 0
}

// 还原必须整体替换数据库：还原前的改动被回滚，备份内的数据被恢复。
func TestRestoreBackupRestoresDataAtomically(t *testing.T) {
	dbPath := initTestDB(t)
	insertUser(t, "user-in-backup", "backup@example.com")

	backupID := "test-backup-001"
	backupPath := filepath.Join(filepath.Dir(dbPath), "backup.db")
	if _, err := database.DB.Exec(fmt.Sprintf(`VACUUM INTO '%s'`, backupPath)); err != nil {
		t.Fatalf("create backup failed: %v", err)
	}
	checksum, size, err := computeChecksum(backupPath)
	if err != nil {
		t.Fatalf("checksum failed: %v", err)
	}
	if _, err := database.DB.Exec(
		`INSERT INTO seasagi_backups (backup_id, filename, file_path, file_size, sha256_checksum, status, triggered_by)
		 VALUES (?, ?, ?, ?, ?, 'completed', 'test')`,
		backupID, "backup.db", backupPath, size, checksum,
	); err != nil {
		t.Fatalf("insert backup record failed: %v", err)
	}

	// 还原前制造差异：删除备份内的用户，新增一个备份中不存在的用户。
	if _, err := database.DB.Exec(`DELETE FROM users WHERE user_id = ?`, "user-in-backup"); err != nil {
		t.Fatalf("delete user failed: %v", err)
	}
	insertUser(t, "user-after-backup", "after@example.com")

	router := gin.New()
	router.POST("/backups/:id/restore", RestoreBackup)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/backups/"+backupID+"/restore", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("restore status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !userExists(t, "user-in-backup") {
		t.Fatal("backup data was not restored")
	}
	if userExists(t, "user-after-backup") {
		t.Fatal("post-backup change should be rolled back")
	}
	for _, leftover := range []string{dbPath + ".restore.tmp", dbPath + ".restore.bak"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Fatalf("temporary file left behind: %s", leftover)
		}
	}
}

// 备份文件不可读时必须保留原库并可继续使用。
func TestRestoreDatabaseKeepsOriginalOnFailure(t *testing.T) {
	dbPath := initTestDB(t)
	insertUser(t, "user-keep", "keep@example.com")

	err := restoreDatabase(filepath.Join(filepath.Dir(dbPath), "missing.db"), dbPath)
	if err == nil {
		t.Fatal("expected restore to fail for missing backup")
	}
	if !userExists(t, "user-keep") {
		t.Fatal("original database must stay intact after failed restore")
	}
}

// 校验和不匹配时拒绝还原，且不修改现有数据库。
func TestRestoreBackupRejectsChecksumMismatch(t *testing.T) {
	dbPath := initTestDB(t)
	insertUser(t, "user-intact", "intact@example.com")

	backupPath := filepath.Join(filepath.Dir(dbPath), "corrupted.db")
	if _, err := database.DB.Exec(fmt.Sprintf(`VACUUM INTO '%s'`, backupPath)); err != nil {
		t.Fatalf("create backup failed: %v", err)
	}
	if _, err := database.DB.Exec(
		`INSERT INTO seasagi_backups (backup_id, filename, file_path, file_size, sha256_checksum, status, triggered_by)
		 VALUES ('test-backup-bad', 'corrupted.db', ?, 0, 'not-the-real-checksum', 'completed', 'test')`,
		backupPath,
	); err != nil {
		t.Fatalf("insert backup record failed: %v", err)
	}

	router := gin.New()
	router.POST("/backups/:id/restore", RestoreBackup)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/backups/test-backup-bad/restore", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if !userExists(t, "user-intact") {
		t.Fatal("database must not change when checksum mismatches")
	}
}
