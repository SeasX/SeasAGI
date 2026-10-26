package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CertificateAuthority 管理 MITM CA 证书和按域名动态签发的叶子证书。
type CertificateAuthority struct {
	caCert   *x509.Certificate
	caKey    *rsa.PrivateKey
	certPath string // ~/.seasagi/mitm/ca.pem
	keyPath  string // ~/.seasagi/mitm/ca-key.pem
	cache    map[string]*cachedCert
	cacheMu  sync.RWMutex

	policyMu sync.RWMutex
	policy   func(string) bool
}

type cachedCert struct {
	cert *tls.Certificate
	exp  time.Time
}

const (
	caMaxAge     = 10 * 365 * 24 * time.Hour // CA 证书有效期 10 年
	leafMaxAge   = 23 * time.Hour            // 叶子证书缓存 TTL 23 小时
	caKeyBits    = 2048
	caCommonName = "SeasAGI MITM CA"
	caOrgName    = "SeasAGI"
	// maxLeafCache 限制叶子证书缓存条目数，避免被任意 SNI 撑爆内存。
	maxLeafCache = 512
)

// SetDomainPolicy 安装可签发域名判定（通常为 MITM 拦截规则 Rules.Match）。
// 传 nil 恢复默认策略：仅允许内置厂商 API 域名与已支持目标。
func (ca *CertificateAuthority) SetDomainPolicy(fn func(string) bool) {
	ca.policyMu.Lock()
	ca.policy = fn
	ca.policyMu.Unlock()
}

// isSignableDomain 仅允许为命中 MITM 规则的公网域名签发叶子证书（CL-S11）。
// 拒绝 IP、localhost、.local/.internal，避免受信根被用于任意站点的中间人。
func (ca *CertificateAuthority) isSignableDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	if d == "" || net.ParseIP(d) != nil {
		return false
	}
	if d == "localhost" || strings.HasSuffix(d, ".local") || strings.HasSuffix(d, ".internal") {
		return false
	}
	ca.policyMu.RLock()
	policy := ca.policy
	ca.policyMu.RUnlock()
	if policy != nil {
		return policy(d)
	}
	if ResolveTarget(d) != nil {
		return true
	}
	for _, base := range defaultRuleDomains {
		if base == d {
			return true
		}
	}
	return false
}

// NewCA 创建 CertificateAuthority 实例。certDir 通常为 ~/.seasagi/mitm/。
func NewCA(certDir string) (*CertificateAuthority, error) {
	if certDir == "" {
		return nil, fmt.Errorf("certDir is empty")
	}
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir certDir: %w", err)
	}
	// 既有目录可能是早期版本以更宽权限创建的，启动时统一收紧（CL-S22）。
	_ = os.Chmod(certDir, 0o700)
	return &CertificateAuthority{
		certPath: filepath.Join(certDir, "ca.pem"),
		keyPath:  filepath.Join(certDir, "ca-key.pem"),
		cache:    make(map[string]*cachedCert),
	}, nil
}

// EnsureCA 首次生成 CA 或从磁盘加载已有 CA。幂等操作。
func (ca *CertificateAuthority) EnsureCA() error {
	if ca.caCert != nil && ca.caKey != nil {
		return nil // 已加载
	}

	// 尝试从磁盘加载
	if _, err := os.Stat(ca.certPath); err == nil {
		return ca.loadFromDisk()
	}

	// 生成新 CA
	return ca.generateAndSave()
}

// loadFromDisk 从磁盘加载 CA 证书和私钥。
func (ca *CertificateAuthority) loadFromDisk() error {
	certPEM, err := os.ReadFile(ca.certPath)
	if err != nil {
		return fmt.Errorf("read ca cert: %w", err)
	}
	keyPEM, err := os.ReadFile(ca.keyPath)
	if err != nil {
		return fmt.Errorf("read ca key: %w", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("failed to decode CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse CA cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("failed to decode CA key PEM")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse CA key: %w", err)
	}

	ca.caCert = caCert
	ca.caKey = caKey
	// CA 私钥是全机 TLS 信任的根，加载后再次收紧权限（CL-S22）。
	_ = os.Chmod(ca.keyPath, 0o600)
	_ = os.Chmod(ca.certPath, 0o644)
	return nil
}

// generateAndSave 生成新的 CA 证书和私钥并保存到磁盘。
func (ca *CertificateAuthority) generateAndSave() error {
	key, err := rsa.GenerateKey(rand.Reader, caKeyBits)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   caCommonName,
			Organization: []string{caOrgName},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caMaxAge),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("parse CA cert: %w", err)
	}

	// 保存到磁盘
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(ca.certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("write CA cert: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(ca.keyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write CA key: %w", err)
	}

	ca.caCert = cert
	ca.caKey = key
	return nil
}

// GetCertificate 实现 tls.Config.GetCertificate 回调，根据 SNI 动态签发叶子证书。
func (ca *CertificateAuthority) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello == nil || hello.ServerName == "" {
		return nil, fmt.Errorf("no SNI in ClientHello")
	}
	if !ca.isSignableDomain(hello.ServerName) {
		return nil, fmt.Errorf("refusing to sign certificate for unmanaged host: %s", hello.ServerName)
	}
	return ca.SignLeafCert(hello.ServerName)
}

// SignLeafCert 为指定域名签发叶子证书，带缓存。
func (ca *CertificateAuthority) SignLeafCert(domain string) (*tls.Certificate, error) {
	if ca.caCert == nil || ca.caKey == nil {
		return nil, fmt.Errorf("CA not initialized, call EnsureCA first")
	}
	domain = strings.TrimSpace(strings.ToLower(domain))
	if !ca.isSignableDomain(domain) {
		return nil, fmt.Errorf("refusing to sign certificate for unmanaged host: %s", domain)
	}

	// 检查缓存
	ca.cacheMu.RLock()
	if cached, ok := ca.cache[domain]; ok && time.Now().Before(cached.exp) {
		ca.cacheMu.RUnlock()
		return cached.cert, nil
	}
	ca.cacheMu.RUnlock()

	// 签发新证书
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate leaf key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   domain,
			Organization: []string{caOrgName},
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(leafMaxAge),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:    []string{domain},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.caCert, &key.PublicKey, ca.caKey)
	if err != nil {
		return nil, fmt.Errorf("create leaf cert: %w", err)
	}

	leaf := &tls.Certificate{
		Certificate: [][]byte{certDER, ca.caCert.Raw},
		PrivateKey:  key,
		Leaf:        mustParseCert(certDER),
	}

	// 写入缓存（有上限：先清理过期项，仍超限则淘汰任意一项）
	ca.cacheMu.Lock()
	if len(ca.cache) >= maxLeafCache {
		now := time.Now()
		for name, entry := range ca.cache {
			if now.After(entry.exp) {
				delete(ca.cache, name)
			}
		}
		if len(ca.cache) >= maxLeafCache {
			for name := range ca.cache {
				delete(ca.cache, name)
				break
			}
		}
	}
	ca.cache[domain] = &cachedCert{cert: leaf, exp: time.Now().Add(leafMaxAge)}
	ca.cacheMu.Unlock()

	return leaf, nil
}

// PEMBytes 导出 CA 证书 PEM 格式（供 TrustInstaller 使用）。
func (ca *CertificateAuthority) PEMBytes() ([]byte, error) {
	if ca.caCert == nil {
		return nil, fmt.Errorf("CA not initialized")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.caCert.Raw}), nil
}

// CACert 返回 CA 证书对象（用于构建 TLS 信任池）。
func (ca *CertificateAuthority) CACert() *x509.Certificate {
	return ca.caCert
}

// CertPool 返回包含 MITM CA 的证书池，用于客户端验证 MITM 签发的叶子证书。
func (ca *CertificateAuthority) CertPool() *x509.CertPool {
	if ca.caCert == nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.caCert)
	return pool
}

// Cleanup 清理证书缓存。
func (ca *CertificateAuthority) Cleanup() {
	ca.cacheMu.Lock()
	ca.cache = make(map[string]*cachedCert)
	ca.cacheMu.Unlock()
}

func mustParseCert(certDER []byte) *x509.Certificate {
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil
	}
	return cert
}
