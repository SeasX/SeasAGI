package security

import (
	"net"
	"strings"
)

// IPFilter implements IP-based access control with whitelist and blacklist support.
// CIDR notation is supported for both lists. The blacklist takes precedence:
// if an IP is in both lists, it is denied.
type IPFilter struct {
	whitelist []*net.IPNet
	blacklist []*net.IPNet
}

// NewIPFilter creates a new filter. whitelist and blacklist are slices of
// CIDR strings (e.g., "10.0.0.0/8", "192.168.1.1/32").
func NewIPFilter(whitelist, blacklist []string) (*IPFilter, error) {
	f := &IPFilter{}
	for _, cidr := range whitelist {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		f.whitelist = append(f.whitelist, ipNet)
	}
	for _, cidr := range blacklist {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		f.blacklist = append(f.blacklist, ipNet)
	}
	return f, nil
}

// Allow checks whether an IP address should be allowed.
// Rules:
//  1. If the IP is in the blacklist, deny.
//  2. If the whitelist is empty, allow (no whitelist = allow all not blacklisted).
//  3. If the whitelist is non-empty, allow only if the IP is in the whitelist.
func (f *IPFilter) Allow(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Check blacklist first (takes precedence)
	for _, cidr := range f.blacklist {
		if cidr.Contains(ip) {
			return false
		}
	}

	// If no whitelist, allow
	if len(f.whitelist) == 0 {
		return true
	}

	// Check whitelist
	for _, cidr := range f.whitelist {
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}

// AddWhitelist adds a CIDR to the whitelist.
func (f *IPFilter) AddWhitelist(cidr string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	f.whitelist = append(f.whitelist, ipNet)
	return nil
}

// AddBlacklist adds a CIDR to the blacklist.
func (f *IPFilter) AddBlacklist(cidr string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	f.blacklist = append(f.blacklist, ipNet)
	return nil
}

// WhitelistCount returns the number of whitelist entries.
func (f *IPFilter) WhitelistCount() int {
	return len(f.whitelist)
}

// BlacklistCount returns the number of blacklist entries.
func (f *IPFilter) BlacklistCount() int {
	return len(f.blacklist)
}

// AllowAll is a convenience filter that allows all IPs (empty whitelist + blacklist).
func AllowAll() *IPFilter {
	f, _ := NewIPFilter(nil, nil)
	return f
}

// DenyAll is a convenience filter that denies all IPs (0.0.0.0/0 in blacklist).
func DenyAll() *IPFilter {
	f, _ := NewIPFilter(nil, []string{"0.0.0.0/0", "::/0"})
	return f
}

// ParseRemoteAddr extracts the client IP from common address formats.
// Handles "host:port", "[ipv6]:port", and bare IP strings.
func ParseRemoteAddr(addr string) string {
	// Try host:port format
	if strings.Contains(addr, ":") {
		host, _, err := net.SplitHostPort(addr)
		if err == nil {
			return host
		}
	}
	// Bare IP or hostname
	return strings.Split(addr, ":")[0]
}
