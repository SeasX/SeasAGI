package updater

import (
	"strings"
	"testing"
)

func TestParseChecksumManifest(t *testing.T) {
	sum := strings.Repeat("a", 64)
	other := strings.Repeat("B", 64)
	data := []byte(`# comment
` + sum + `  SeasAGI-macos.dmg
` + strings.ToLower(other) + ` *SeasAGI-windows.exe
SHA256 (SeasAGI-linux.tar.gz) = ` + sum + `
garbage line
`)

	m := parseChecksumManifest(data)

	if m["SeasAGI-macos.dmg"] != sum {
		t.Errorf("macos entry = %q, want %q", m["SeasAGI-macos.dmg"], sum)
	}
	if m["SeasAGI-windows.exe"] != strings.ToLower(other) {
		t.Errorf("windows entry = %q, want %q", m["SeasAGI-windows.exe"], strings.ToLower(other))
	}
	if m["SeasAGI-linux.tar.gz"] != sum {
		t.Errorf("linux entry = %q, want %q", m["SeasAGI-linux.tar.gz"], sum)
	}
	if _, ok := m["garbage line"]; ok {
		t.Error("不应解析非法行")
	}
}

func TestLookupChecksumByBaseName(t *testing.T) {
	sum := strings.Repeat("c", 64)
	m := map[string]string{"dist/SeasAGI-macos.dmg": sum}

	if got, ok := lookupChecksum(m, "https://example.com/releases/SeasAGI-macos.dmg"); !ok || got != sum {
		t.Fatalf("lookup by base name = (%q, %v)", got, ok)
	}
}

func TestValidateHTTPSURLRejectsPlainHTTP(t *testing.T) {
	if err := validateHTTPSURL("http://example.com/pkg.dmg"); err == nil {
		t.Fatal("http url should be rejected")
	}
	if err := validateHTTPSURL("https://example.com/pkg.dmg"); err != nil {
		t.Fatalf("https url should be accepted: %v", err)
	}
	if err := validateHTTPSURL("file:///etc/passwd"); err == nil {
		t.Fatal("file url should be rejected")
	}
}

func TestDownloadDMGRefusesWithoutChecksum(t *testing.T) {
	if _, err := downloadDMG("https://example.com/pkg.dmg", ""); err == nil {
		t.Fatal("下载必须要求合法 SHA-256")
	}
	if _, err := downloadDMG("https://example.com/pkg.dmg", "not-a-hash"); err == nil {
		t.Fatal("非法 SHA-256 必须被拒绝")
	}
}

func TestResolveExpectedSHA256(t *testing.T) {
	// 有 GitHub digest 时直接使用
	status := &UpdateStatus{DownloadSHA256: strings.Repeat("d", 64)}
	got, err := resolveExpectedSHA256(status)
	if err != nil || got != status.DownloadSHA256 {
		t.Fatalf("resolveExpectedSHA256(digest) = (%q, %v)", got, err)
	}

	// 无 digest 且无清单地址 → 中止
	if _, err := resolveExpectedSHA256(&UpdateStatus{}); err == nil {
		t.Fatal("缺少校验来源时必须中止更新")
	}
}

func TestParseDigest(t *testing.T) {
	if got := parseDigest("sha256:ABCDEF"); got != "abcdef" {
		t.Fatalf("parseDigest = %q", got)
	}
	if got := parseDigest("md5:abc"); got != "" {
		t.Fatalf("非 sha256 digest 应返回空, got %q", got)
	}
}
