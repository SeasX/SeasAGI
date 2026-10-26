package urlguard

import (
	"net/http"
	"net/url"
	"testing"
)

func TestValidateSecureURL(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		allowHTTP bool
		wantErr   bool
	}{
		{"https public", "https://api.openai.com/v1", false, false},
		{"https with port", "https://example.com:8443/v1", false, false},
		{"http public rejected", "http://api.openai.com/v1", true, true},
		{"http loopback allowed", "http://localhost:11434", true, false},
		{"http 127 allowed", "http://127.0.0.1:8080/v1", true, false},
		{"http ipv6 loopback allowed", "http://[::1]:8080", true, false},
		{"http loopback denied when disabled", "http://localhost:11434", false, true},
		{"unsupported scheme", "ftp://example.com", true, true},
		{"file scheme", "file:///etc/passwd", true, true},
		{"userinfo rejected", "https://user:pass@example.com/v1", false, true},
		{"empty", "   ", true, true},
		{"no host", "https://", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSecureURL(tc.raw, tc.allowHTTP)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.raw, err)
			}
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"localhost":      true,
		"localhost:8080": true,
		"LOCALHOST":      true,
		"127.0.0.1":      true,
		"127.0.0.1:4318": true,
		"[::1]:8080":     true,
		"::1":            true,
		"10.0.0.1":       false,
		"example.com":    false,
		"":               false,
	}
	for host, want := range cases {
		if got := IsLoopbackHost(host); got != want {
			t.Errorf("IsLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestHostOnly(t *testing.T) {
	cases := map[string]string{
		"localhost:8080": "localhost",
		"[::1]:8080":     "::1",
		"[::1]":          "::1",
		"example.com":    "example.com",
		"":               "",
	}
	for in, want := range cases {
		if got := HostOnly(in); got != want {
			t.Errorf("HostOnly(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRejectLinkLocalHost(t *testing.T) {
	rejected := []string{
		"",
		"169.254.169.254",
		"169.254.169.254:80",
		"[fe80::1]:8080",
		"fe80::1",
		"0.0.0.0",
		"0.0.0.0:4318",
		"::",
		"224.0.0.1",
		"[ff02::1]:53",
	}
	for _, host := range rejected {
		if err := RejectLinkLocalHost(host); err == nil {
			t.Errorf("expected %q to be rejected", host)
		}
	}

	allowed := []string{
		"127.0.0.1:4318",
		"[::1]:4318",
		"10.0.0.5:8080",
		"192.168.1.10",
		"api.example.com:443",
	}
	for _, host := range allowed {
		if err := RejectLinkLocalHost(host); err != nil {
			t.Errorf("expected %q to be allowed, got %v", host, err)
		}
	}
}

func TestValidateProxyTarget(t *testing.T) {
	rejected := []string{
		"",
		"localhost:4318",
		"127.0.0.1:4318",
		"127.0.0.1",
		"[::1]:8080",
		"169.254.169.254:80",
		"[fe80::1]:8080",
		"0.0.0.0:80",
		"224.0.0.1:443",
	}
	for _, host := range rejected {
		if err := ValidateProxyTarget(host); err == nil {
			t.Errorf("expected proxy target %q to be rejected", host)
		}
	}

	allowed := []string{
		"10.1.2.3:443",
		"172.16.0.9:8080",
		"api.openai.com:443",
	}
	for _, host := range allowed {
		if err := ValidateProxyTarget(host); err != nil {
			t.Errorf("expected proxy target %q to be allowed, got %v", host, err)
		}
	}
}

func newRedirectRequest(t *testing.T, raw string) *http.Request {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	req := &http.Request{URL: u, Header: http.Header{}}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Api-Key", "sk-secret")
	req.Header.Set("X-Goog-Api-Key", "AIza-secret")
	return req
}

func TestSecureRedirect(t *testing.T) {
	t.Run("first hop allowed", func(t *testing.T) {
		req := newRedirectRequest(t, "https://api.example.com/v2")
		if err := SecureRedirect(req, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("too many hops", func(t *testing.T) {
		req := newRedirectRequest(t, "https://api.example.com/v2")
		via := make([]*http.Request, 5)
		for i := range via {
			via[i] = newRedirectRequest(t, "https://api.example.com/v2")
		}
		if err := SecureRedirect(req, via); err == nil {
			t.Fatal("expected redirect limit error")
		}
	})

	t.Run("https to http rejected", func(t *testing.T) {
		prev := newRedirectRequest(t, "https://api.example.com/v1")
		req := newRedirectRequest(t, "http://127.0.0.1:8080/v1")
		if err := SecureRedirect(req, []*http.Request{prev}); err == nil {
			t.Fatal("expected downgrade to be rejected")
		}
	})

	t.Run("cross host strips credentials", func(t *testing.T) {
		prev := newRedirectRequest(t, "https://api.example.com/v1")
		req := newRedirectRequest(t, "https://evil.example.net/v1")
		if err := SecureRedirect(req, []*http.Request{prev}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, h := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key"} {
			if got := req.Header.Get(h); got != "" {
				t.Errorf("expected %s to be stripped, got %q", h, got)
			}
		}
	})

	t.Run("same host keeps credentials", func(t *testing.T) {
		prev := newRedirectRequest(t, "https://api.example.com/v1")
		req := newRedirectRequest(t, "https://api.example.com/v2")
		if err := SecureRedirect(req, []*http.Request{prev}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Header.Get("Authorization") == "" {
			t.Error("expected Authorization to be preserved for same-host redirect")
		}
	})
}
