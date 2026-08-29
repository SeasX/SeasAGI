package security

import (
	"testing"
)

func TestSSRFFilterInternal(t *testing.T) {
	filter := NewSSRFFilter()

	internalURLs := []string{
		"http://10.0.0.1/path",
		"http://172.16.0.1/path",
		"http://192.168.1.1/path",
		"http://10.255.255.255/path",
		"http://172.31.255.255/path",
		"http://192.168.0.0/path",
	}

	for _, u := range internalURLs {
		t.Run(u, func(t *testing.T) {
			if !filter.IsBlocked(u) {
				t.Errorf("IsBlocked(%q) = false, want true (internal address)", u)
			}
		})
	}
}

func TestSSRFFilterLocalhost(t *testing.T) {
	filter := NewSSRFFilter()

	localhostURLs := []string{
		"http://127.0.0.1/path",
		"http://localhost/path",
		"http://localhost:8080/path",
		"http://127.0.0.1:8080/path",
		"http://[::1]/path",
		"http://ip6-localhost/path",
	}

	for _, u := range localhostURLs {
		t.Run(u, func(t *testing.T) {
			if !filter.IsBlocked(u) {
				t.Errorf("IsBlocked(%q) = false, want true (localhost)", u)
			}
		})
	}
}

func TestSSRFFilterPublic(t *testing.T) {
	filter := NewSSRFFilter()

	publicURLs := []string{
		"http://8.8.8.8/path",
		"http://1.1.1.1/path",
		"http://93.184.216.34/path",
		"https://api.openai.com/v1/chat",
	}

	for _, u := range publicURLs {
		t.Run(u, func(t *testing.T) {
			if filter.IsBlocked(u) {
				t.Errorf("IsBlocked(%q) = true, want false (public address)", u)
			}
		})
	}
}

func TestSSRFFilterNonHTTP(t *testing.T) {
	filter := NewSSRFFilter()

	nonHTTP := []string{
		"file:///etc/passwd",
		"ftp://example.com/file",
		"gopher://localhost:1234",
		"dict://localhost:11111",
	}

	for _, u := range nonHTTP {
		t.Run(u, func(t *testing.T) {
			if !filter.IsBlocked(u) {
				t.Errorf("IsBlocked(%q) = false, want true (non-HTTP scheme)", u)
			}
		})
	}
}

func TestSSRFFilterLinkLocal(t *testing.T) {
	filter := NewSSRFFilter()

	linkLocal := []string{
		"http://169.254.1.1/path",
		"http://169.254.169.254/latest/meta-data/",
	}

	for _, u := range linkLocal {
		t.Run(u, func(t *testing.T) {
			if !filter.IsBlocked(u) {
				t.Errorf("IsBlocked(%q) = false, want true (link-local)", u)
			}
		})
	}
}

func TestSSRFFilterAllowCIDR(t *testing.T) {
	filter := NewSSRFFilter()

	// 10.0.0.1 is blocked by default
	if !filter.IsBlocked("http://10.0.0.1/path") {
		t.Error("10.0.0.1 should be blocked by default")
	}

	// Allow 10.0.0.0/24
	err := filter.AllowCIDR("10.0.0.0/24")
	if err != nil {
		t.Fatalf("AllowCIDR failed: %v", err)
	}

	if filter.IsBlocked("http://10.0.0.1/path") {
		t.Error("10.0.0.1 should be allowed after AllowCIDR")
	}

	// 10.0.1.1 should still be blocked (outside /24)
	if !filter.IsBlocked("http://10.0.1.1/path") {
		t.Error("10.0.1.1 should still be blocked")
	}
}

func TestSSRFFilterBlockCIDR(t *testing.T) {
	filter := NewSSRFFilter()

	// 8.8.8.8 is allowed by default
	if filter.IsBlocked("http://8.8.8.8/path") {
		t.Error("8.8.8.8 should be allowed by default")
	}

	err := filter.BlockCIDR("8.0.0.0/8")
	if err != nil {
		t.Fatalf("BlockCIDR failed: %v", err)
	}

	if !filter.IsBlocked("http://8.8.8.8/path") {
		t.Error("8.8.8.8 should be blocked after BlockCIDR")
	}
}

func TestSSRFFilterGetBlockedReason(t *testing.T) {
	filter := NewSSRFFilter()

	tests := []struct {
		url    string
		expect string
	}{
		{"http://127.0.0.1/path", "loopback address"},
		{"http://10.0.0.1/path", "private address range"},
		{"http://169.254.1.1/path", "link-local address"},
		{"http://localhost/path", "internal hostname"},
		{"file:///etc/passwd", "non-HTTP scheme"},
		{"http://8.8.8.8/path", "allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			reason := filter.GetBlockedReason(tt.url)
			if reason != tt.expect {
				t.Errorf("GetBlockedReason(%q) = %q, want %q", tt.url, reason, tt.expect)
			}
		})
	}
}

func TestSSRFFilterInvalidURL(t *testing.T) {
	filter := NewSSRFFilter()

	// Invalid URL should be blocked
	if !filter.IsBlocked("://invalid") {
		t.Error("Invalid URL should be blocked")
	}
}

func TestSSRFFilterInternalTLD(t *testing.T) {
	filter := NewSSRFFilter()

	internalTLDs := []string{
		"http://service.local/path",
		"http://app.internal/path",
		"http://dev.localhost/path",
	}

	for _, u := range internalTLDs {
		t.Run(u, func(t *testing.T) {
			if !filter.IsBlocked(u) {
				t.Errorf("IsBlocked(%q) = false, want true (internal TLD)", u)
			}
		})
	}
}
