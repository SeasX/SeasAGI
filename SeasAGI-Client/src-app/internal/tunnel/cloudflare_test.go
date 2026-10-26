package tunnel

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudflaredAssetsArePinned(t *testing.T) {
	for platform, asset := range cloudflaredAssets {
		if asset.Name == "" {
			t.Errorf("%s: empty asset name", platform)
		}
		if !isHexDigest(asset.SHA256) {
			t.Errorf("%s: invalid pinned SHA-256 %q", platform, asset.SHA256)
		}
		if strings.Contains(asset.Name, "latest") {
			t.Errorf("%s: asset must not use latest", platform)
		}
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin")
	data := []byte("cloudflared-binary")
	if err := os.WriteFile(path, data, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])

	if err := verifyFileSHA256(path, good); err != nil {
		t.Fatalf("verify good digest: %v", err)
	}
	if err := verifyFileSHA256(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("错误摘要必须校验失败")
	}
}

func TestExtractCloudflaredFromTGZ(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	payload := []byte("binary-content")
	if err := tw.WriteHeader(&tar.Header{Name: "cloudflared", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractCloudflared(buf.Bytes())
	if err != nil {
		t.Fatalf("extractCloudflared: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("extracted = %q, want %q", got, payload)
	}
}

func TestCloudflaredCacheDirPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := cloudflaredCacheDir()
	if err != nil {
		t.Fatalf("cloudflaredCacheDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("cache dir perm = %o, want 700", perm)
	}
}
