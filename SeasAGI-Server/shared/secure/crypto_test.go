package secure

import (
	"errors"
	"strings"
	"testing"
)

const testKey = "0123456789abcdef0123456789abcdef-test"

func testBox(envs ...string) Box {
	if len(envs) == 0 {
		envs = []string{"SHARED_TEST_KEY"}
	}
	return Box{
		KeyEnv: envs,
		Salt:   []byte("test/salt"),
		Info:   []byte("test/info"),
		Prefix: "tst:v1:",
	}
}

func TestBoxRoundTripAndPrefix(t *testing.T) {
	t.Setenv("SHARED_TEST_KEY", testKey)
	b := testBox()
	ct, err := b.Encrypt("sk-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !b.IsCiphertext(ct) || strings.Contains(ct, "sk-secret") {
		t.Fatalf("ciphertext %q missing prefix or leaks plaintext", ct)
	}
	pt, err := b.Decrypt(ct)
	if err != nil || pt != "sk-secret" {
		t.Fatalf("Decrypt = %q, %v", pt, err)
	}
}

func TestBoxKeyFallbackOrder(t *testing.T) {
	t.Setenv("PRIMARY_KEY", "")
	t.Setenv("FALLBACK_KEY", testKey)
	b := testBox("PRIMARY_KEY", "FALLBACK_KEY")
	if got := b.KeySource(); got != "FALLBACK_KEY" {
		t.Fatalf("KeySource = %q, want FALLBACK_KEY", got)
	}
	ct, err := b.Encrypt("v")
	if err != nil {
		t.Fatal(err)
	}
	// 主密钥后来配置成另一个值时，旧密文不可再解开（不会静默用错密钥）。
	t.Setenv("PRIMARY_KEY", testKey+"-other")
	if _, err := b.Decrypt(ct); err == nil {
		t.Fatal("expected decrypt failure after key source changed")
	}
}

func TestBoxDomainSeparation(t *testing.T) {
	t.Setenv("SHARED_TEST_KEY", testKey)
	a := testBox()
	other := testBox()
	other.Info = []byte("another/service")
	ct, err := a.Encrypt("v")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Decrypt(ct); err == nil {
		t.Fatal("ciphertext from one service must not decrypt under another service's derivation")
	}
}

func TestBoxCheckAndMissingKey(t *testing.T) {
	t.Setenv("SHARED_TEST_KEY", "")
	b := testBox()
	if err := b.Check(); !errors.Is(err, ErrKeyNotConfigured) {
		t.Fatalf("Check() = %v, want ErrKeyNotConfigured", err)
	}
	if _, err := b.Encrypt("v"); err == nil {
		t.Fatal("Encrypt must fail without a key")
	}
	t.Setenv("SHARED_TEST_KEY", "short")
	if err := b.Check(); err == nil || strings.Contains(err.Error(), "short") {
		t.Fatalf("weak key must be rejected without echoing the key: %v", err)
	}
}

func TestBoxRejectsUnprefixedWithoutLegacy(t *testing.T) {
	t.Setenv("SHARED_TEST_KEY", testKey)
	if _, err := testBox().Decrypt("plain-api-key"); err == nil {
		t.Fatal("unprefixed value must not be treated as ciphertext")
	}
}

func TestBoxTamperedCiphertext(t *testing.T) {
	t.Setenv("SHARED_TEST_KEY", testKey)
	b := testBox()
	ct, _ := b.Encrypt("v")
	tampered := ct[:len(ct)-2] + "AA"
	if _, err := b.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext must fail authentication")
	}
}
