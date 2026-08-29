package mitm

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAndLoadCA(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	// 首次生成
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA first: %v", err)
	}
	if ca.caCert == nil || ca.caKey == nil {
		t.Fatal("CA cert or key is nil after EnsureCA")
	}

	// 验证文件存在
	if _, err := os.Stat(filepath.Join(dir, "ca.pem")); err != nil {
		t.Fatalf("ca.pem not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ca-key.pem")); err != nil {
		t.Fatalf("ca-key.pem not created: %v", err)
	}

	// 记录指纹
	origFingerprint := ca.caCert.SerialNumber

	// 重新加载
	ca2, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA second: %v", err)
	}
	if err := ca2.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA reload: %v", err)
	}

	// 验证 SerialNumber 一致（同一 CA）
	if ca2.caCert.SerialNumber.Cmp(origFingerprint) != 0 {
		t.Error("CA serial number mismatch after reload")
	}
}

func TestSignLeafCert(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	leaf, err := ca.SignLeafCert("api.openai.com")
	if err != nil {
		t.Fatalf("SignLeafCert: %v", err)
	}
	if leaf == nil {
		t.Fatal("leaf cert is nil")
	}

	// 验证证书链包含 CA
	if len(leaf.Certificate) < 2 {
		t.Fatalf("expected at least 2 certs in chain, got %d", len(leaf.Certificate))
	}

	// 解析叶子证书
	parsed, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf cert: %v", err)
	}

	// 验证 SAN 包含目标域名
	found := false
	for _, san := range parsed.DNSNames {
		if san == "api.openai.com" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("leaf cert SAN does not contain api.openai.com, got %v", parsed.DNSNames)
	}

	// 验证 CA 签名链
	if err := parsed.CheckSignatureFrom(ca.caCert); err != nil {
		t.Errorf("leaf cert not signed by CA: %v", err)
	}
}

func TestLeafCertCacheHit(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	// 第一次签发
	leaf1, err := ca.SignLeafCert("api.anthropic.com")
	if err != nil {
		t.Fatalf("first SignLeafCert: %v", err)
	}

	// 第二次应命中缓存
	leaf2, err := ca.SignLeafCert("api.anthropic.com")
	if err != nil {
		t.Fatalf("second SignLeafCert: %v", err)
	}

	// 验证是同一个证书对象（缓存命中）
	if leaf1 != leaf2 {
		t.Error("cache miss: second call returned different cert object")
	}
}

func TestGetCertificateSNI(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	hello := &tls.ClientHelloInfo{ServerName: "api.deepseek.com"}
	cert, err := ca.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cert == nil {
		t.Fatal("cert is nil")
	}
}

func TestGetCertificateEmptySNI(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	_, err = ca.GetCertificate(&tls.ClientHelloInfo{ServerName: ""})
	if err == nil {
		t.Error("expected error for empty SNI")
	}
}

func TestPEMBytes(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	pemBytes, err := ca.PEMBytes()
	if err != nil {
		t.Fatalf("PEMBytes: %v", err)
	}
	if len(pemBytes) == 0 {
		t.Fatal("PEMBytes returned empty")
	}

	// 验证可解析
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		t.Error("failed to parse PEM bytes as certificate")
	}
}
