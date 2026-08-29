package backup

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

// BackupRecord represents a database backup record.
type BackupRecord struct {
	BackupID    string `json:"backup_id"`
	Filename    string `json:"filename"`
	FilePath    string `json:"file_path"`
	FileSize    int64  `json:"file_size"`
	SHA256      string `json:"sha256_checksum"`
	Status      string `json:"status"`
	TriggeredBy string `json:"triggered_by"`
	CreatedAt   string `json:"created_at"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	RestoredAt  string `json:"restored_at,omitempty"`
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func backupDir() string {
	dir := os.Getenv("BACKUP_DIR")
	if dir == "" {
		dir = filepath.Join(filepath.Dir(database.Path()), "backups")
	}
	os.MkdirAll(dir, 0755)
	return dir
}

// CreateBackup creates a new SQLite database backup.
func CreateBackup(c *gin.Context) {
	dbPath := database.Path()
	if dbPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database path not available"})
		return
	}

	backupID := generateID()
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("platform-api_%s_%s.db", timestamp, backupID[:8])
	backupPath := filepath.Join(backupDir(), filename)

	// Use SQLite online backup API via VACUUM INTO
	_, err := database.DB.Exec(fmt.Sprintf(`VACUUM INTO '%s'`, backupPath))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("backup failed: %v", err)})
		return
	}

	// Calculate SHA256 checksum
	checksum, fileSize, err := computeChecksum(backupPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("checksum failed: %v", err)})
		os.Remove(backupPath)
		return
	}

	// Calculate expiry (default 30 days)
	retentionDays := 30
	expiry := time.Now().Add(time.Duration(retentionDays) * 24 * time.Hour)

	_, err = database.DB.Exec(
		`INSERT INTO seasagi_backups (backup_id, filename, file_path, file_size, sha256_checksum, status, triggered_by, expires_at)
		 VALUES (?,?,?,?,?, 'completed', 'admin', ?)`,
		backupID, filename, backupPath, fileSize, checksum, expiry.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	record, _ := fetchBackup(backupID)
	c.JSON(http.StatusCreated, record)
}

// ListBackups returns all backup records.
func ListBackups(c *gin.Context) {
	rows, err := database.DB.Query(
		`SELECT backup_id, filename, file_path, file_size, sha256_checksum, status, triggered_by, created_at, expires_at, restored_at
		 FROM seasagi_backups ORDER BY created_at DESC`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	backups := make([]BackupRecord, 0)
	for rows.Next() {
		var b BackupRecord
		var expiresAt, restoredAt sql.NullString
		if err := rows.Scan(&b.BackupID, &b.Filename, &b.FilePath, &b.FileSize, &b.SHA256, &b.Status, &b.TriggeredBy, &b.CreatedAt, &expiresAt, &restoredAt); err != nil {
			continue
		}
		if expiresAt.Valid {
			b.ExpiresAt = expiresAt.String
		}
		if restoredAt.Valid {
			b.RestoredAt = restoredAt.String
		}
		backups = append(backups, b)
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": backups})
}

// GetBackup returns a single backup record.
func GetBackup(c *gin.Context) {
	backupID := c.Param("id")
	record, err := fetchBackup(backupID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	c.JSON(http.StatusOK, record)
}

// DeleteBackup deletes a backup record and its file.
func DeleteBackup(c *gin.Context) {
	backupID := c.Param("id")
	record, err := fetchBackup(backupID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	// Remove the file
	os.Remove(record.FilePath)

	// Delete the record
	_, err = database.DB.Exec(`DELETE FROM seasagi_backups WHERE backup_id = ?`, backupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// RestoreBackup restores the database from a backup file.
func RestoreBackup(c *gin.Context) {
	backupID := c.Param("id")
	record, err := fetchBackup(backupID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	// Verify checksum
	currentChecksum, _, err := computeChecksum(record.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "backup file not accessible"})
		return
	}
	if currentChecksum != record.SHA256 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "checksum mismatch, backup file may be corrupted"})
		return
	}

	// Copy backup file to the current database path
	dbPath := database.Path()
	if dbPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database path not available"})
		return
	}

	// Read backup file
	data, err := os.ReadFile(record.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read backup: %v", err)})
		return
	}

	// Write to database path
	if err := os.WriteFile(dbPath, data, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to restore: %v", err)})
		return
	}

	// Update restored_at timestamp
	database.DB.Exec(`UPDATE seasagi_backups SET restored_at = CURRENT_TIMESTAMP WHERE backup_id = ?`, backupID)

	c.JSON(http.StatusOK, gin.H{"restored": true, "backup_id": backupID, "note": "database restored, restart recommended"})
}

// VerifyBackup verifies a backup's checksum.
func VerifyBackup(c *gin.Context) {
	backupID := c.Param("id")
	record, err := fetchBackup(backupID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	currentChecksum, fileSize, err := computeChecksum(record.FilePath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"backup_id": backupID, "valid": false, "error": "file not accessible"})
		return
	}

	valid := currentChecksum == record.SHA256
	c.JSON(http.StatusOK, gin.H{
		"backup_id":       backupID,
		"valid":           valid,
		"expected_sha256": record.SHA256,
		"actual_sha256":   currentChecksum,
		"file_size":       fileSize,
	})
}

// fetchBackup retrieves a single backup record from the database.
func fetchBackup(backupID string) (BackupRecord, error) {
	var b BackupRecord
	var expiresAt, restoredAt sql.NullString
	err := database.DB.QueryRow(
		`SELECT backup_id, filename, file_path, file_size, sha256_checksum, status, triggered_by, created_at, expires_at, restored_at
		 FROM seasagi_backups WHERE backup_id = ?`,
		backupID,
	).Scan(&b.BackupID, &b.Filename, &b.FilePath, &b.FileSize, &b.SHA256, &b.Status, &b.TriggeredBy, &b.CreatedAt, &expiresAt, &restoredAt)
	if err != nil {
		return b, fmt.Errorf("backup not found")
	}
	if expiresAt.Valid {
		b.ExpiresAt = expiresAt.String
	}
	if restoredAt.Valid {
		b.RestoredAt = restoredAt.String
	}
	return b, nil
}

// computeChecksum calculates SHA256 checksum and file size.
func computeChecksum(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hasher := sha256.New()
	buf := make([]byte, 64*1024)
	var totalSize int64
	for {
		n, err := file.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
			totalSize += int64(n)
		}
		if err != nil {
			break
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), totalSize, nil
}
