package security

import (
	"regexp"
	"strings"
)

// ErrorSanitizer strips sensitive information from error messages before
// returning them to clients. It removes file paths, stack traces, internal
// hostnames, and environment variable references.
type ErrorSanitizer struct {
	enabled bool

	// pathPatterns matches file system paths on common OSes.
	pathPatterns []*regexp.Regexp

	// stackTracePatterns matches common stack trace formats.
	stackTracePatterns []*regexp.Regexp

	// internalHosts are hostnames considered internal.
	internalHosts []string
}

// NewErrorSanitizer creates a sanitizer with default rules.
func NewErrorSanitizer(enabled bool) *ErrorSanitizer {
	s := &ErrorSanitizer{
		enabled: enabled,
		pathPatterns: []*regexp.Regexp{
			// Unix absolute paths
			regexp.MustCompile(`/(?:home|usr|var|etc|opt|tmp|root|Users|Volumes|private)/[^\s:'"` + "`" + `]+`),
			// Windows paths
			regexp.MustCompile(`[A-Z]:\\[^\s:'"` + "`" + `]+`),
			// Relative paths with known source dirs
			regexp.MustCompile(`(?:src-app|internal|cmd|pkg|vendor)/[^\s:'"` + "`" + `]+`),
		},
		stackTracePatterns: []*regexp.Regexp{
			// Go panic / runtime stack
			regexp.MustCompile(`(?m)^goroutine \d+ \[.*\]:.*(\n\s+[\w./-]+:\d+.*|\n.*created by.*)*`),
			// Python traceback
			regexp.MustCompile(`(?s)Traceback \(most recent call last\):.*?Error:.*`),
			// Java stack trace
			regexp.MustCompile(`(?m)^\s*at\s+[\w.$]+\([^)]*\)`),
			// Node stack trace
			regexp.MustCompile(`(?m)^\s*at\s+[\w./<>-]+\s+\([^)]*\)`),
		},
		internalHosts: []string{
			"localhost", "127.0.0.1", "0.0.0.0",
			"::1", "10.", "172.16.", "172.17.", "172.18.",
			"172.19.", "172.20.", "172.21.", "172.22.", "172.23.",
			"172.24.", "172.25.", "172.26.", "172.27.", "172.28.",
			"172.29.", "172.30.", "172.31.", "192.168.",
		},
	}
	return s
}

// Sanitize cleans an error message, removing sensitive information.
func (s *ErrorSanitizer) Sanitize(errMsg string) string {
	if !s.enabled {
		return errMsg
	}

	result := errMsg

	// Remove stack traces
	for _, p := range s.stackTracePatterns {
		result = p.ReplaceAllString(result, "[stack-trace]")
	}

	// Remove file paths
	for _, p := range s.pathPatterns {
		result = p.ReplaceAllString(result, "[path]")
	}

	// Mask internal hostnames
	for _, h := range s.internalHosts {
		if strings.Contains(result, h) {
			// Replace standalone host or in URL context
			hostPattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(h))
			result = hostPattern.ReplaceAllString(result, "[internal-host]")
		}
	}

	// Remove environment variable references like $HOME, ${API_KEY}
	envPattern := regexp.MustCompile(`\$\{?\w+\}?`)
	result = envPattern.ReplaceAllString(result, "[env-var]")

	// Remove Go internal package references
	goPkgPattern := regexp.MustCompile(`github\.com/[\w-]+/[\w-]+/[\w./-]+`)
	result = goPkgPattern.ReplaceAllString(result, "[internal-pkg]")

	// Collapse multiple whitespace
	result = regexp.MustCompile(`\s{3,}`).ReplaceAllString(result, " ")

	return strings.TrimSpace(result)
}

// IsEnabled returns whether the sanitizer is active.
func (s *ErrorSanitizer) IsEnabled() bool {
	return s.enabled
}

// SetEnabled enables or disables the sanitizer.
func (s *ErrorSanitizer) SetEnabled(enabled bool) {
	s.enabled = enabled
}

// SanitizeIfEnabled is a convenience function that sanitizes only if enabled.
func (s *ErrorSanitizer) SanitizeIfEnabled(errMsg string) string {
	if s.enabled {
		return s.Sanitize(errMsg)
	}
	return errMsg
}
