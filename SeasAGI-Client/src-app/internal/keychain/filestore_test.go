package keychain

import (
	"os"
	"path/filepath"
	"testing"
)

// setupKeychainDir 将加密文件存储隔离到临时目录，避免污染真实用户目录。
func setupKeychainDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SEASAGI_KEYCHAIN_DIR", dir)
	return dir
}

func TestFileStoreRoundTrip(t *testing.T) {
	dir := setupKeychainDir(t)

	if err := fileStoreSave("channel_demo", "sk-plain-secret"); err != nil {
		t.Fatalf("fileStoreSave: %v", err)
	}

	got, err := fileStoreGet("channel_demo")
	if err != nil {
		t.Fatalf("fileStoreGet: %v", err)
	}
	if got != "sk-plain-secret" {
		t.Fatalf("fileStoreGet = %q, want %q", got, "sk-plain-secret")
	}

	// 密文文件不得包含明文
	path, err := entryFilePath("channel_demo")
	if err != nil {
		t.Fatalf("entryFilePath: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if string(raw) == "sk-plain-secret" {
		t.Fatal("凭证以明文落盘")
	}

	// 主密钥文件权限必须为 0600
	keyPath := filepath.Join(dir, "master.key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat master key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != keychainFileMode {
		t.Fatalf("master key perm = %o, want %o", perm, keychainFileMode)
	}

	// 条目文件权限同理
	entryInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat entry: %v", err)
	}
	if perm := entryInfo.Mode().Perm(); perm != keychainFileMode {
		t.Fatalf("entry perm = %o, want %o", perm, keychainFileMode)
	}
}

func TestFileStoreRejectsTamperedEntry(t *testing.T) {
	setupKeychainDir(t)

	if err := fileStoreSave("secret_x", "value"); err != nil {
		t.Fatalf("fileStoreSave: %v", err)
	}
	path, _ := entryFilePath("secret_x")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	// 篡改密文最后一个字节
	raw[len(raw)-1] ^= 0xff
	if err := os.WriteFile(path, raw, keychainFileMode); err != nil {
		t.Fatalf("write tampered entry: %v", err)
	}

	if _, err := fileStoreGet("secret_x"); err == nil {
		t.Fatal("篡改后的条目必须读取失败")
	}
}

func TestFileStoreMissingEntryReturnsError(t *testing.T) {
	setupKeychainDir(t)

	if _, err := fileStoreGet("does_not_exist"); err == nil {
		t.Fatal("不存在的条目应返回错误")
	}
}

func TestFileStoreDelete(t *testing.T) {
	setupKeychainDir(t)

	if err := fileStoreSave("tmp", "v"); err != nil {
		t.Fatalf("fileStoreSave: %v", err)
	}
	if err := fileStoreDelete("tmp"); err != nil {
		t.Fatalf("fileStoreDelete: %v", err)
	}
	if _, err := fileStoreGet("tmp"); err == nil {
		t.Fatal("删除后仍可读取")
	}
	// 重复删除应幂等
	if err := fileStoreDelete("tmp"); err != nil {
		t.Fatalf("second fileStoreDelete: %v", err)
	}
}
