package security

import (
	"testing"
)

func TestIPFilterCIDR(t *testing.T) {
	// Whitelist 10.0.0.0/8, blacklist 10.0.0.1/32
	filter, err := NewIPFilter(
		[]string{"10.0.0.0/8"},
		[]string{"10.0.0.1/32"},
	)
	if err != nil {
		t.Fatalf("NewIPFilter failed: %v", err)
	}

	tests := []struct {
		name    string
		ip      string
		allowed bool
	}{
		{"in whitelist not blacklist", "10.0.0.2", true},
		{"in whitelist and blacklist (blacklist wins)", "10.0.0.1", false},
		{"not in whitelist", "192.168.1.1", false},
		{"not in whitelist", "172.16.0.1", false},
		{"invalid IP", "not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Allow(tt.ip)
			if result != tt.allowed {
				t.Errorf("Allow(%q) = %v, want %v", tt.ip, result, tt.allowed)
			}
		})
	}
}

func TestIPFilterNoWhitelist(t *testing.T) {
	// Only blacklist, no whitelist — should allow everything not blacklisted
	filter, err := NewIPFilter(nil, []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("NewIPFilter failed: %v", err)
	}

	if !filter.Allow("192.168.1.1") {
		t.Error("192.168.1.1 should be allowed (not in blacklist)")
	}
	if filter.Allow("10.1.2.3") {
		t.Error("10.1.2.3 should be blocked (in blacklist)")
	}
}

func TestIPFilterEmptyLists(t *testing.T) {
	// No whitelist or blacklist — allow all
	filter, err := NewIPFilter(nil, nil)
	if err != nil {
		t.Fatalf("NewIPFilter failed: %v", err)
	}

	if !filter.Allow("1.2.3.4") {
		t.Error("1.2.3.4 should be allowed with empty filter")
	}
	if !filter.Allow("10.0.0.1") {
		t.Error("10.0.0.1 should be allowed with empty filter")
	}
}

func TestIPFilterIPv6(t *testing.T) {
	filter, err := NewIPFilter(
		[]string{"::1/128", "2001:db8::/32"},
		nil,
	)
	if err != nil {
		t.Fatalf("NewIPFilter failed: %v", err)
	}

	if !filter.Allow("::1") {
		t.Error("::1 should be allowed (in whitelist)")
	}
	if !filter.Allow("2001:db8::1") {
		t.Error("2001:db8::1 should be allowed (in whitelist)")
	}
	if filter.Allow("2001:dead::1") {
		t.Error("2001:dead::1 should be blocked (not in whitelist)")
	}
}

func TestIPFilterAddCIDR(t *testing.T) {
	filter, err := NewIPFilter(nil, nil)
	if err != nil {
		t.Fatalf("NewIPFilter failed: %v", err)
	}

	if !filter.Allow("10.0.0.1") {
		t.Error("10.0.0.1 should be allowed before blacklist")
	}

	err = filter.AddBlacklist("10.0.0.0/8")
	if err != nil {
		t.Fatalf("AddBlacklist failed: %v", err)
	}

	if filter.Allow("10.0.0.1") {
		t.Error("10.0.0.1 should be blocked after adding to blacklist")
	}

	err = filter.AddWhitelist("10.0.0.1/32")
	if err != nil {
		t.Fatalf("AddWhitelist failed: %v", err)
	}

	// Still blocked because blacklist takes precedence
	if filter.Allow("10.0.0.1") {
		t.Error("10.0.0.1 should still be blocked (blacklist wins)")
	}
}

func TestIPFilterAllowAll(t *testing.T) {
	filter := AllowAll()
	if !filter.Allow("8.8.8.8") {
		t.Error("AllowAll should allow 8.8.8.8")
	}
}

func TestIPFilterDenyAll(t *testing.T) {
	filter := DenyAll()
	if filter.Allow("8.8.8.8") {
		t.Error("DenyAll should block 8.8.8.8")
	}
}

func TestParseRemoteAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{"host:port", "192.168.1.1:8080", "192.168.1.1"},
		{"localhost:port", "localhost:3000", "localhost"},
		{"bare IP", "10.0.0.1", "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRemoteAddr(tt.addr)
			if got != tt.want {
				t.Errorf("ParseRemoteAddr(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}
