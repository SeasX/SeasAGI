package configio

import "testing"

func TestNewServiceResolvesHomeDir(t *testing.T) {
	s := NewService(nil)
	if s == nil {
		t.Fatal("expected non-nil service")
	}
	if s.homeDir == "" {
		t.Fatal("expected home directory to be resolved")
	}
}

func TestImportRejectsMalformedJSON(t *testing.T) {
	s := NewService(nil)
	if err := s.Import([]byte("not json")); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestImportRejectsMissingVersion(t *testing.T) {
	s := NewService(nil)
	if err := s.Import([]byte(`{"channels":[]}`)); err == nil {
		t.Fatal("expected error for missing version")
	}
}
