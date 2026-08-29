package security

import (
	"regexp"
	"strings"
)

// PIIMasker detects and masks Personally Identifiable Information (PII).
// All masking is opt-in — the caller decides whether to enable it.
type PIIMasker struct {
	enabled  bool
	patterns []PIIPattern
}

// PIIPattern defines a PII type and its masking rule.
type PIIPattern struct {
	Name    string
	Pattern *regexp.Regexp
	Mask    string // replacement template, uses $1 for capture group if needed
}

// NewPIIMasker creates a masker with built-in patterns for common PII types.
func NewPIIMasker(enabled bool) *PIIMasker {
	m := &PIIMasker{
		enabled: enabled,
		patterns: []PIIPattern{
			{
				Name:    "email",
				Pattern: regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
				Mask:    "[EMAIL]",
			},
			{
				Name:    "phone_intl",
				Pattern: regexp.MustCompile(`\+\d{1,3}[\s.-]?\d{1,4}[\s.-]?\d{3,4}[\s.-]?\d{3,4}`),
				Mask:    "[PHONE]",
			},
			{
				Name:    "phone_us",
				Pattern: regexp.MustCompile(`\b\d{3}[\s.-]\d{3}[\s.-]\d{4}\b`),
				Mask:    "[PHONE]",
			},
			{
				Name:    "credit_card_visa",
				Pattern: regexp.MustCompile(`\b4\d{3}[\s.-]?\d{4}[\s.-]?\d{4}[\s.-]?\d{4}\b`),
				Mask:    "[CREDIT_CARD]",
			},
			{
				Name:    "credit_card_mc",
				Pattern: regexp.MustCompile(`\b5[1-5]\d{2}[\s.-]?\d{4}[\s.-]?\d{4}[\s.-]?\d{4}\b`),
				Mask:    "[CREDIT_CARD]",
			},
			{
				Name:    "credit_card_amex",
				Pattern: regexp.MustCompile(`\b3[47]\d{2}[\s.-]?\d{6}[\s.-]?\d{5}\b`),
				Mask:    "[CREDIT_CARD]",
			},
			{
				Name:    "ssn_us",
				Pattern: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
				Mask:    "[SSN]",
			},
			{
				Name:    "id_cn",
				Pattern: regexp.MustCompile(`\b\d{15}(\d{2}[\dXx])?\b`),
				Mask:    "[ID_NUMBER]",
			},
			{
				Name:    "ipv4",
				Pattern: regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
				Mask:    "[IP]",
			},
			{
				Name:    "api_key_sk",
				Pattern: regexp.MustCompile(`\bsk-[a-zA-Z0-9]{20,}\b`),
				Mask:    "[API_KEY]",
			},
		},
	}
	return m
}

// Mask applies all PII patterns to the input text and returns the masked result.
// If the masker is disabled, the original text is returned unchanged.
func (m *PIIMasker) Mask(input string) string {
	if !m.enabled {
		return input
	}
	result := input
	for _, p := range m.patterns {
		result = p.Pattern.ReplaceAllString(result, p.Mask)
	}
	return result
}

// MaskMessages masks PII in all message contents.
func (m *PIIMasker) MaskMessages(messages []map[string]interface{}) {
	if !m.enabled {
		return
	}
	for _, msg := range messages {
		if content, ok := msg["content"].(string); ok {
			msg["content"] = m.Mask(content)
		}
	}
}

// IsEnabled returns whether the masker is active.
func (m *PIIMasker) IsEnabled() bool {
	return m.enabled
}

// SetEnabled enables or disables the masker.
func (m *PIIMasker) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// AddPattern adds a custom PII pattern.
func (m *PIIMasker) AddPattern(name, pattern, mask string) error {
	p, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	m.patterns = append(m.patterns, PIIPattern{Name: name, Pattern: p, Mask: mask})
	return nil
}

// DetectedPII returns the types of PII found in the input without masking.
type DetectedPII struct {
	Type  string
	Value string
}

// Detect returns a list of detected PII items in the input.
func (m *PIIMasker) Detect(input string) []DetectedPII {
	var detections []DetectedPII
	for _, p := range m.patterns {
		matches := p.Pattern.FindAllString(input, -1)
		for _, match := range matches {
			detections = append(detections, DetectedPII{Type: p.Name, Value: match})
		}
	}
	return detections
}

// DLPPattern defines a regex-based sensitive content pattern for the DLP matcher.
type DLPPattern struct {
	Name    string
	Pattern *regexp.Regexp
	Mask    string
}

// DLPMatcher extends PIIMasker with advanced sensitive-content detection:
// regex-based PII, sensitive phrase matching, and PEM private key block detection.
type DLPMatcher struct {
	patterns            map[string]*DLPPattern
	sensitivePhrases    []string
	similarityThreshold float64
}

// pemKeyBlockRegex matches PEM-encoded private key headers/blocks.
var pemKeyBlockRegex = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`)

// sensitiveValueRegex matches common "key = value" assignment lines for secrets.
var sensitiveValueRegex = regexp.MustCompile(`(?im)^(\s*(password|secret|api_key|token)\s*=\s*).*$`)

// NewDLPMatcher initializes a DLPMatcher with the built-in PII regex patterns
// plus a list of common sensitive phrases. The similarity threshold defaults to 0.6.
func NewDLPMatcher() *DLPMatcher {
	builtin := []PIIPattern{
		{
			Name:    "email",
			Pattern: regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Mask:    "[EMAIL]",
		},
		{
			Name:    "phone_intl",
			Pattern: regexp.MustCompile(`\+\d{1,3}[\s.-]?\d{1,4}[\s.-]?\d{3,4}[\s.-]?\d{3,4}`),
			Mask:    "[PHONE]",
		},
		{
			Name:    "phone_us",
			Pattern: regexp.MustCompile(`\b\d{3}[\s.-]\d{3}[\s.-]\d{4}\b`),
			Mask:    "[PHONE]",
		},
		{
			Name:    "credit_card_visa",
			Pattern: regexp.MustCompile(`\b4\d{3}[\s.-]?\d{4}[\s.-]?\d{4}[\s.-]?\d{4}\b`),
			Mask:    "[CREDIT_CARD]",
		},
		{
			Name:    "credit_card_mc",
			Pattern: regexp.MustCompile(`\b5[1-5]\d{2}[\s.-]?\d{4}[\s.-]?\d{4}[\s.-]?\d{4}\b`),
			Mask:    "[CREDIT_CARD]",
		},
		{
			Name:    "credit_card_amex",
			Pattern: regexp.MustCompile(`\b3[47]\d{2}[\s.-]?\d{6}[\s.-]?\d{5}\b`),
			Mask:    "[CREDIT_CARD]",
		},
		{
			Name:    "ssn_us",
			Pattern: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
			Mask:    "[SSN]",
		},
		{
			Name:    "id_cn",
			Pattern: regexp.MustCompile(`\b\d{15}(\d{2}[\dXx])?\b`),
			Mask:    "[ID_NUMBER]",
		},
		{
			Name:    "ipv4",
			Pattern: regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
			Mask:    "[IP]",
		},
		{
			Name:    "api_key_sk",
			Pattern: regexp.MustCompile(`\bsk-[a-zA-Z0-9]{20,}\b`),
			Mask:    "[API_KEY]",
		},
	}

	patterns := make(map[string]*DLPPattern, len(builtin))
	for i := range builtin {
		patterns[builtin[i].Name] = &DLPPattern{
			Name:    builtin[i].Name,
			Pattern: builtin[i].Pattern,
			Mask:    builtin[i].Mask,
		}
	}

	return &DLPMatcher{
		patterns: patterns,
		sensitivePhrases: []string{
			"private key",
			"password",
			"secret",
			"BEGIN PRIVATE KEY",
			"BEGIN RSA PRIVATE KEY",
			"BEGIN EC PRIVATE KEY",
			"BEGIN OPENSSH PRIVATE KEY",
			"aws_secret_access_key",
			"api_key",
			"authorization",
		},
		similarityThreshold: 0.6,
	}
}

// DetectAdvanced runs the existing regex-based PII detection, then checks for
// sensitive phrases (case-insensitive substring) and PEM private key blocks
// (regex). Returns the combined list of detections.
func (d *DLPMatcher) DetectAdvanced(input string) []DetectedPII {
	var detections []DetectedPII

	// 1. Existing regex detection
	lower := strings.ToLower(input)
	for _, p := range d.patterns {
		matches := p.Pattern.FindAllString(input, -1)
		for _, match := range matches {
			detections = append(detections, DetectedPII{Type: p.Name, Value: match})
		}
	}

	// 2. Sensitive phrase detection (case-insensitive substring)
	for _, phrase := range d.sensitivePhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			detections = append(detections, DetectedPII{Type: "sensitive_phrase", Value: phrase})
		}
	}

	// 3. PEM private key block detection (regex)
	pemMatches := pemKeyBlockRegex.FindAllString(input, -1)
	for _, match := range pemMatches {
		detections = append(detections, DetectedPII{Type: "private_key_block", Value: match})
	}

	return detections
}

// MaskAdvanced first runs the existing regex masking (PII patterns), then masks
// PEM private key blocks with [PRIVATE_KEY], then redacts the value portion of
// lines containing "password =", "secret =", "api_key =", or "token =" (case-insensitive).
func (d *DLPMatcher) MaskAdvanced(input string) string {
	result := input

	// 1. Existing regex masking
	for _, p := range d.patterns {
		result = p.Pattern.ReplaceAllString(result, p.Mask)
	}

	// 2. Mask PEM key blocks
	result = pemKeyBlockRegex.ReplaceAllString(result, "[PRIVATE_KEY]")

	// 3. Redact secret assignment values
	result = sensitiveValueRegex.ReplaceAllString(result, "${1}[REDACTED]")

	return result
}

// MaskMessagesAdvanced applies MaskAdvanced to the content of each message.
func (d *DLPMatcher) MaskMessagesAdvanced(messages []map[string]interface{}) {
	for _, msg := range messages {
		if content, ok := msg["content"].(string); ok {
			msg["content"] = d.MaskAdvanced(content)
		}
	}
}
