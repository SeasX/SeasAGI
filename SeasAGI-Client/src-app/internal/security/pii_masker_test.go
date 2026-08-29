package security

import (
	"testing"
)

func TestPIIMaskerEmail(t *testing.T) {
	masker := NewPIIMasker(true)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple email", "Contact me at alice@example.com", "Contact me at [EMAIL]"},
		{"multiple emails", "Send to bob@test.org and charlie@demo.io", "Send to [EMAIL] and [EMAIL]"},
		{"no email", "No email here", "No email here"},
		{"email with dots", "first.last@sub.domain.com", "[EMAIL]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := masker.Mask(tt.input)
			if got != tt.want {
				t.Errorf("Mask(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPIIMaskerPhone(t *testing.T) {
	masker := NewPIIMasker(true)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"US phone with dashes", "Call 555-123-4567", "Call [PHONE]"},
		{"US phone with dots", "Call 555.123.4567", "Call [PHONE]"},
		{"US phone with spaces", "Call 555 123 4567", "Call [PHONE]"},
		{"international phone", "Call +1-800-555-1234", "Call [PHONE]"},
		{"no phone", "No phone here", "No phone here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := masker.Mask(tt.input)
			if got != tt.want {
				t.Errorf("Mask(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPIIMaskerCreditCard(t *testing.T) {
	masker := NewPIIMasker(true)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Visa", "Card: 4111111111111111", "Card: [CREDIT_CARD]"},
		{"Visa with dashes", "4111-1111-1111-1111", "[CREDIT_CARD]"},
		{"MasterCard", "5500000000000004", "[CREDIT_CARD]"},
		{"Amex", "378282246310005", "[CREDIT_CARD]"},
		{"non-card number", "My favorite number is 42", "My favorite number is 42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := masker.Mask(tt.input)
			if got != tt.want {
				t.Errorf("Mask(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPIIMaskerDisabled(t *testing.T) {
	masker := NewPIIMasker(false)

	input := "Email: alice@example.com, Phone: 555-123-4567"
	got := masker.Mask(input)
	if got != input {
		t.Errorf("Disabled masker should return original, got %q", got)
	}
}

func TestPIIMaskerSSN(t *testing.T) {
	masker := NewPIIMasker(true)

	input := "SSN: 123-45-6789"
	got := masker.Mask(input)
	if got != "SSN: [SSN]" {
		t.Errorf("Mask SSN: got %q, want 'SSN: [SSN]'", got)
	}
}

func TestPIIMaskerAPIKey(t *testing.T) {
	masker := NewPIIMasker(true)

	input := "Key: sk-abcdefghijklmnopqrstuvwxyz123456"
	got := masker.Mask(input)
	if got != "Key: [API_KEY]" {
		t.Errorf("Mask API key: got %q, want 'Key: [API_KEY]'", got)
	}
}

func TestPIIMaskerDetect(t *testing.T) {
	masker := NewPIIMasker(true)

	input := "Email alice@test.com and call 555-123-4567, card 4111111111111111"
	detections := masker.Detect(input)
	if len(detections) < 3 {
		t.Errorf("Expected at least 3 detections, got %d", len(detections))
	}

	types := map[string]bool{}
	for _, d := range detections {
		types[d.Type] = true
	}
	if !types["email"] {
		t.Error("Expected email detection")
	}
	if !types["phone_us"] && !types["phone_intl"] {
		t.Error("Expected phone detection")
	}
	if !types["credit_card_visa"] {
		t.Error("Expected credit card detection")
	}
}

func TestPIIMaskerMaskMessages(t *testing.T) {
	masker := NewPIIMasker(true)

	messages := []map[string]interface{}{
		{"role": "user", "content": "My email is alice@example.com"},
		{"role": "assistant", "content": "Noted"},
	}
	masker.MaskMessages(messages)

	content := messages[0]["content"].(string)
	if content != "My email is [EMAIL]" {
		t.Errorf("Masked content = %q, want 'My email is [EMAIL]'", content)
	}
}
