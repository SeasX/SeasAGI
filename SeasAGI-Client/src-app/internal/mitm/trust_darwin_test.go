//go:build darwin

package mitm

import (
	"strings"
	"testing"
)

func TestPEMSHA256Fingerprint(t *testing.T) {
	dir := t.TempDir()
	ca, caErr := NewCA(dir)
	if caErr != nil {
		t.Fatalf("NewCA: %v", caErr)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	pemBytes, pemErr := ca.PEMBytes()
	if pemErr != nil {
		t.Fatalf("PEMBytes: %v", pemErr)
	}

	fp, err := pemSHA256Fingerprint(pemBytes)
	if err != nil {
		t.Fatalf("pemSHA256Fingerprint: %v", err)
	}
	if fp == "" {
		t.Fatal("fingerprint should not be empty")
	}
	if strings.Contains(fp, ":") {
		t.Fatalf("fingerprint should use security CLI format without colons, got %q", fp)
	}
	if fp != strings.ToUpper(fp) {
		t.Fatalf("fingerprint should be uppercase, got %q", fp)
	}
}

func TestPEMSHA256FingerprintRejectsInvalidPEM(t *testing.T) {
	if _, err := pemSHA256Fingerprint([]byte("not-a-cert")); err == nil {
		t.Fatal("expected invalid PEM to fail")
	}
}
