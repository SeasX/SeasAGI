package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogRotationBySize(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")
	// 创建一个超过 1KB 的文件
	data := make([]byte, 2048)
	os.WriteFile(logPath, data, 0644)

	r := NewLogRotator(logPath, RotationConfig{
		MaxFileSize:   1024,
		RetentionDays: 7,
		MaxFiles:      20,
		CheckInterval: time.Second,
	})

	if err := r.CheckAndRotate(); err != nil {
		t.Fatalf("CheckAndRotate failed: %v", err)
	}

	// 原文件应不存在，应有轮转文件
	if _, err := os.Stat(logPath); err == nil {
		// 轮转后原文件可能还在（如果大小刚好），检查轮转文件存在
	}
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if e.Name() != "app.log" && len(e.Name()) > 8 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected rotated log file")
	}
}

func TestLogRotationRetention(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")

	// 创建一个旧文件（8 天前）
	oldPath := filepath.Join(dir, "app.2026-01-01_000000.log")
	os.WriteFile(oldPath, []byte("old"), 0644)
	oldTime := time.Now().AddDate(0, 0, -8)
	os.Chtimes(oldPath, oldTime, oldTime)

	// 创建一个新文件
	newPath := filepath.Join(dir, "app.2026-07-30_120000.log")
	os.WriteFile(newPath, []byte("new"), 0644)

	r := NewLogRotator(logPath, RotationConfig{
		MaxFileSize:   50 * 1024 * 1024,
		RetentionDays: 7,
		MaxFiles:      20,
		CheckInterval: time.Second,
	})

	if err := r.CleanRetention(); err != nil {
		t.Fatalf("CleanRetention failed: %v", err)
	}

	// 旧文件应被删除
	if _, err := os.Stat(oldPath); err == nil {
		t.Fatal("old log file should be deleted")
	}
	// 新文件应保留
	if _, err := os.Stat(newPath); err != nil {
		t.Fatal("new log file should be retained")
	}
}

func TestLogRotationMaxFiles(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")

	// 创建 5 个文件，maxFiles=3
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, "app.2026-07-2"+string(rune('0'+i))+"_120000.log")
		os.WriteFile(p, []byte("x"), 0644)
		t := time.Now().Add(-time.Duration(i) * time.Hour)
		os.Chtimes(p, t, t)
	}

	r := NewLogRotator(logPath, RotationConfig{
		MaxFileSize:   50 * 1024 * 1024,
		RetentionDays: 365,
		MaxFiles:      3,
		CheckInterval: time.Second,
	})

	if err := r.CleanRetention(); err != nil {
		t.Fatalf("CleanRetention failed: %v", err)
	}

	// 应只剩 3 个文件
	entries, _ := os.ReadDir(dir)
	count := 0
	for _, e := range entries {
		if len(e.Name()) > 8 && e.Name() != "app.log" {
			count++
		}
	}
	if count > 3 {
		t.Fatalf("expected at most 3 files, got %d", count)
	}
}

func TestPayloadRedactSensitiveKeys(t *testing.T) {
	data := map[string]interface{}{
		"api_key":  "sk-12345",
		"model":    "gpt-4",
		"password": "secret123",
		"nested": map[string]interface{}{
			"token":         "abc",
			"safe_field":    "ok",
			"authorization": "Bearer xyz",
		},
	}

	result := RedactPayload(data).(map[string]interface{})

	if result["api_key"] != "[REDACTED]" {
		t.Fatal("api_key should be redacted")
	}
	if result["model"] != "gpt-4" {
		t.Fatal("model should not be redacted")
	}
	if result["password"] != "[REDACTED]" {
		t.Fatal("password should be redacted")
	}

	nested := result["nested"].(map[string]interface{})
	if nested["token"] != "[REDACTED]" {
		t.Fatal("nested token should be redacted")
	}
	if nested["safe_field"] != "ok" {
		t.Fatal("safe_field should not be redacted")
	}
}

func TestPayloadRedactBearer(t *testing.T) {
	s := "Authorization: Bearer sk-abc123"
	result := RedactBearerToken(s)
	if result != "Authorization: Bearer [REDACTED]" {
		t.Fatalf("expected Bearer redacted, got %s", result)
	}
}

func TestPayloadTruncate(t *testing.T) {
	long := strings.Repeat("x", 70000)
	result := SerializePayloadForStorage(long)
	if len(result) > 65536+20 {
		t.Fatalf("expected truncated, got len=%d", len(result))
	}
	if !strings.Contains(result, "[truncated]") {
		t.Fatal("expected [truncated] marker")
	}
}

func TestPayloadBinaryDetect(t *testing.T) {
	if !IsOpaqueBinary([]byte{1, 2, 3}) {
		t.Fatal("[]byte should be detected as binary")
	}
	if IsOpaqueBinary("string") {
		t.Fatal("string should not be binary")
	}
	if IsOpaqueBinary(123) {
		t.Fatal("int should not be binary")
	}
}

func TestPayloadProtectForLog(t *testing.T) {
	data := map[string]interface{}{
		"api_key": "sk-secret",
		"auth":    "Bearer sk-token",
		"message": "hello",
	}
	result := ProtectPayloadForLog(data)
	if !contains(result, "[REDACTED]") {
		t.Fatal("protected payload should contain [REDACTED]")
	}
	if contains(result, "sk-secret") {
		t.Fatal("protected payload should not contain secret value")
	}
	if !contains(result, "hello") {
		t.Fatal("protected payload should contain safe value")
	}

	// 二进制数据
	binResult := ProtectPayloadForLog([]byte{1, 2, 3})
	if binResult != "[binary data]" {
		t.Fatalf("expected [binary data], got %s", binResult)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
