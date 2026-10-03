package localtoken

import "testing"

func TestGenerateToken(t *testing.T) {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	tok := generateToken()
	if len(tok) != tokenLength {
		t.Fatalf("expected token length %d, got %d", tokenLength, len(tok))
	}
	for _, r := range tok {
		if !containsRune(charset, r) {
			t.Fatalf("token contains invalid character %q", r)
		}
	}
	if generateToken() == tok {
		t.Fatal("expected distinct tokens across calls")
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
