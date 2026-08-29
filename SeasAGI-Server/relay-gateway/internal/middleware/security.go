// Package middleware provides HTTP security middleware for the SeasAGI Relay Gateway.
package middleware

import (
	"net"
	"net/http"
	"strings"
)

// SecurityMiddleware bundles all security checks into a single middleware chain.
type SecurityMiddleware struct {
	ipFilter       *IPFilter
	errorSanitizer *ErrorSanitizer
}

// IPFilter implements IP-based access control.
type IPFilter struct {
	whitelist []*net.IPNet
	blacklist []*net.IPNet
}

// NewIPFilter creates a new IP filter from CIDR lists.
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

// Allow checks if an IP is allowed.
func (f *IPFilter) Allow(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range f.blacklist {
		if cidr.Contains(ip) {
			return false
		}
	}
	if len(f.whitelist) == 0 {
		return true
	}
	for _, cidr := range f.whitelist {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// ErrorSanitizer strips sensitive info from error messages.
type ErrorSanitizer struct {
	enabled bool
}

// NewErrorSanitizer creates a new sanitizer.
func NewErrorSanitizer(enabled bool) *ErrorSanitizer {
	return &ErrorSanitizer{enabled: enabled}
}

// Sanitize cleans an error message.
func (s *ErrorSanitizer) Sanitize(errMsg string) string {
	if !s.enabled {
		return errMsg
	}
	result := errMsg
	// Remove file paths
	// Remove common path prefixes
	pathPrefixes := []string{"/home/", "/usr/", "/var/", "/etc/", "/opt/", "/Users/"}
	for _, p := range pathPrefixes {
		if strings.Contains(result, p) {
			result = strings.ReplaceAll(result, p, "[path]")
		}
	}
	return strings.TrimSpace(result)
}

// NewSecurityMiddleware creates a middleware with default configuration.
func NewSecurityMiddleware() *SecurityMiddleware {
	return &SecurityMiddleware{
		ipFilter:       &IPFilter{},
		errorSanitizer: NewErrorSanitizer(true),
	}
}

// IPFilterMiddleware wraps an http.Handler with IP filtering.
func (sm *SecurityMiddleware) IPFilterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := ParseRemoteAddr(r.RemoteAddr)
		if !sm.ipFilter.Allow(clientIP) {
			http.Error(w, `{"error":{"message":"Access denied","type":"forbidden"}}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware adds standard security headers to responses.
func (sm *SecurityMiddleware) SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// ParseRemoteAddr extracts client IP from address string.
func ParseRemoteAddr(addr string) string {
	if strings.Contains(addr, ":") {
		host, _, err := net.SplitHostPort(addr)
		if err == nil {
			return host
		}
	}
	return strings.Split(addr, ":")[0]
}

// SanitizeError cleans an error message before returning to client.
func (sm *SecurityMiddleware) SanitizeError(errMsg string) string {
	return sm.errorSanitizer.Sanitize(errMsg)
}
