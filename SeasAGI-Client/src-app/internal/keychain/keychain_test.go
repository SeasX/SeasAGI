package keychain

import "testing"

func TestEnvKeyName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"local_access_token", "SEASAGI_LOCAL_ACCESS_TOKEN"},
		{"platform_access_token", "SEASAGI_PLATFORM_ACCESS_TOKEN"},
		{"channel_abc", "SEASAGI_CHANNEL_ABC"},
		{"a.b-c", "SEASAGI_A_B_C"},
	}
	for _, c := range cases {
		if got := envKeyName(c.in); got != c.want {
			t.Errorf("envKeyName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

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
