// Package security provides security enhancement middleware for SeasAGI.
package security

import (
	"regexp"
	"strings"
)

// PromptInjectionDetector detects common prompt injection attack patterns.
type PromptInjectionDetector struct {
	patterns []*regexp.Regexp
}

// NewPromptInjectionDetector creates a detector with built-in attack patterns.
func NewPromptInjectionDetector() *PromptInjectionDetector {
	patterns := []*regexp.Regexp{
		// Jailbreak patterns
		regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous\s+(instructions|prompts|rules)`),
		regexp.MustCompile(`(?i)disregard\s+(all\s+)?prior\s+(instructions|prompts|rules)`),
		regexp.MustCompile(`(?i)forget\s+(all\s+)?(your|previous)\s+(instructions|rules|prompts)`),
		regexp.MustCompile(`(?i)you\s+are\s+(now|no\s+longer)\s+`),
		regexp.MustCompile(`(?i)act\s+as\s+(if|a|an)\s+`),
		regexp.MustCompile(`(?i)pretend\s+(you\s+are|to\s+be)\s+`),
		regexp.MustCompile(`(?i)simulate\s+(being|you\s+are)\s+`),

		// Role hijack patterns
		regexp.MustCompile(`(?i)your\s+new\s+(role|instruction|task)\s+is`),
		regexp.MustCompile(`(?i)override\s+(your|the)\s+(system|safety|content)\s+(prompt|filter|rules)`),
		regexp.MustCompile(`(?i)enter\s+(developer|jailbreak|unrestricted)\s+mode`),
		regexp.MustCompile(`(?i)bypass\s+(your|the|all)\s+(safety|content|filter)\s+(checks?|rules?|filters?)`),

		// Instruction leak patterns
		regexp.MustCompile(`(?i)(show|reveal|print|output|display)\s+(me\s+)?(your|the)\s+((?:system|initial|original)\s+)*((?:system|initial|original)\s+)?(prompt|instructions?|messages?)`),
		regexp.MustCompile(`(?i)what\s+(are|is)\s+your\s+(system|initial|original)\s+(prompt|instructions?)`),

		// DAN and variants
		regexp.MustCompile(`(?i)DAN\s+mode`),
		regexp.MustCompile(`(?i)do\s+anything\s+now`),
		regexp.MustCompile(`(?i)AI\s+freedom\s+mode`),
		regexp.MustCompile(`(?i)unrestricted\s+AI`),

		// Encoding-based bypass attempts
		regexp.MustCompile(`(?i)\\x[0-9a-f]{2}`),
		regexp.MustCompile(`(?i)base64\s*(decode|encoded)`),

		// Direct command injection
		regexp.MustCompile(`(?i)system\s*\(.*\)`),
		regexp.MustCompile(`(?i)exec\s*\(.*\)`),
		regexp.MustCompile(`(?i)eval\s*\(.*\)`),
	}

	return &PromptInjectionDetector{patterns: patterns}
}

// InjectionResult holds the detection result.
type InjectionResult struct {
	Detected bool
	Pattern  string
	Severity string // "high", "medium", "low"
	Input    string
}

// Check checks the input text for prompt injection patterns.
func (d *PromptInjectionDetector) Check(input string) InjectionResult {
	for _, p := range d.patterns {
		if loc := p.FindStringIndex(input); loc != nil {
			matched := input[loc[0]:loc[1]]
			severity := classifySeverity(p.String(), matched)
			return InjectionResult{
				Detected: true,
				Pattern:  p.String(),
				Severity: severity,
				Input:    matched,
			}
		}
	}
	return InjectionResult{Detected: false}
}

// CheckMessages checks all message contents for injection.
func (d *PromptInjectionDetector) CheckMessages(messages []map[string]interface{}) InjectionResult {
	for _, msg := range messages {
		if content, ok := msg["content"].(string); ok {
			result := d.Check(content)
			if result.Detected {
				return result
			}
		}
	}
	return InjectionResult{Detected: false}
}

func classifySeverity(pattern, matched string) string {
	lower := strings.ToLower(matched)

	// High severity: direct jailbreak / override / system prompt leak
	highKeywords := []string{"ignore all previous", "ignore previous", "disregard all prior", "disregard prior",
		"forget your instructions", "forget all your instructions",
		"override", "bypass", "system prompt", "developer mode", "jailbreak", "dan mode",
		"do anything now", "unrestricted ai", "exec", "eval", "system("}
	for _, kw := range highKeywords {
		if strings.Contains(lower, kw) {
			return "high"
		}
	}

	// Medium severity: role manipulation
	mediumKeywords := []string{"you are now", "act as", "pretend", "simulate", "new role", "ai freedom"}
	for _, kw := range mediumKeywords {
		if strings.Contains(lower, kw) {
			return "medium"
		}
	}

	return "low"
}

// AddPattern allows adding custom detection patterns.
func (d *PromptInjectionDetector) AddPattern(pattern string) error {
	p, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	d.patterns = append(d.patterns, p)
	return nil
}
