package security

import (
	"net"
	"net/url"
	"strings"
)

// SSRFFilter prevents Server-Side Request Forgery by blocking requests to
// internal, loopback, and link-local addresses.
type SSRFFilter struct {
	// allowList contains CIDRs that are explicitly allowed even if they
	// would otherwise be blocked (e.g., for local development).
	allowList []*net.IPNet

	// blockList contains additional CIDRs to block beyond the defaults.
	blockList []*net.IPNet

	// blockLoopback blocks 127.0.0.0/8 and ::1/128.
	blockLoopback bool

	// blockPrivate blocks RFC 1918 private ranges and RFC 4193 unique local.
	blockPrivate bool

	// blockLinkLocal blocks 169.254.0.0/16 and fe80::/10.
	blockLinkLocal bool

	// blockMulticast blocks 224.0.0.0/4 and ff00::/8.
	blockMulticast bool

	// blockReserved blocks 0.0.0.0/8, 240.0.0.0/4, and other reserved ranges.
	blockReserved bool
}

// NewSSRFFilter creates a filter with sensible defaults:
// all internal ranges blocked.
func NewSSRFFilter() *SSRFFilter {
	return &SSRFFilter{
		blockLoopback:  true,
		blockPrivate:   true,
		blockLinkLocal: true,
		blockMulticast: true,
		blockReserved:  true,
	}
}

// AllowCIDR adds a CIDR to the allow list (overrides blocks).
func (f *SSRFFilter) AllowCIDR(cidr string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	f.allowList = append(f.allowList, ipNet)
	return nil
}

// BlockCIDR adds a CIDR to the block list.
func (f *SSRFFilter) BlockCIDR(cidr string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	f.blockList = append(f.blockList, ipNet)
	return nil
}

// IsBlocked checks whether a URL should be blocked.
// Returns true if the URL points to a blocked address.
func (f *SSRFFilter) IsBlocked(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true // Invalid URL = block
	}

	// Check scheme — only allow http/https
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return true
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return true
	}

	// Resolve hostname to IP(s)
	// For numeric IPs, parse directly
	if ip := net.ParseIP(hostname); ip != nil {
		return f.isIPBlocked(ip)
	}

	// For hostnames, check against known internal hostnames first
	if f.isInternalHostname(hostname) {
		return true
	}

	// DNS resolution
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return true // Can't resolve = block
	}

	for _, ip := range ips {
		if f.isIPBlocked(ip) {
			return true
		}
	}

	return false
}

// IsAllowed is the inverse of IsBlocked for convenience.
func (f *SSRFFilter) IsAllowed(rawURL string) bool {
	return !f.IsBlocked(rawURL)
}

func (f *SSRFFilter) isIPBlocked(ip net.IP) bool {
	// Check allow list first (overrides everything)
	for _, cidr := range f.allowList {
		if cidr.Contains(ip) {
			return false
		}
	}

	// Check custom block list
	for _, cidr := range f.blockList {
		if cidr.Contains(ip) {
			return true
		}
	}

	// Check loopback
	if f.blockLoopback {
		if ip.IsLoopback() {
			return true
		}
	}

	// Check private
	if f.blockPrivate {
		if ip.IsPrivate() {
			return true
		}
	}

	// Check link-local
	if f.blockLinkLocal {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
	}

	// Check multicast
	if f.blockMulticast {
		if ip.IsMulticast() {
			return true
		}
	}

	// Check reserved / unspecified
	if f.blockReserved {
		if ip.IsUnspecified() {
			return true
		}
		// 0.0.0.0/8 range
		if ip4 := ip.To4(); ip4 != nil {
			if ip4[0] == 0 {
				return true
			}
			// 240.0.0.0/4 reserved
			if ip4[0] >= 240 {
				return true
			}
		}
	}

	return false
}

func (f *SSRFFilter) isInternalHostname(hostname string) bool {
	internalHosts := []string{
		"localhost",
		"ip6-localhost",
		"ip6-loopback",
		"broadcasthost",
	}
	lower := strings.ToLower(hostname)
	for _, h := range internalHosts {
		if lower == h {
			return true
		}
	}

	// Check for .local, .internal, .localhost TLDs
	internalTLDs := []string{".local", ".internal", ".localhost"}
	for _, tld := range internalTLDs {
		if strings.HasSuffix(lower, tld) {
			return true
		}
	}

	return false
}

// GetBlockedReason returns a human-readable reason for why a URL is blocked.
func (f *SSRFFilter) GetBlockedReason(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "invalid URL"
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "non-HTTP scheme"
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return "empty hostname"
	}

	if f.isInternalHostname(hostname) {
		return "internal hostname"
	}

	if ip := net.ParseIP(hostname); ip != nil {
		if f.isIPBlocked(ip) {
			if ip.IsLoopback() {
				return "loopback address"
			}
			if ip.IsPrivate() {
				return "private address range"
			}
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				return "link-local address"
			}
			if ip.IsMulticast() {
				return "multicast address"
			}
			if ip.IsUnspecified() {
				return "unspecified address"
			}
			return "reserved address"
		}
	}

	return "allowed"
}
