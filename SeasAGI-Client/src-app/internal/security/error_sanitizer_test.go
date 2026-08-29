package security

import (
	"strings"
	"testing"
)

func TestErrorSanitizerBasic(t *testing.T) {
	s := NewErrorSanitizer(true)

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unix path",
			input: "open /home/user/project/config.yaml: no such file or directory",
		},
		{
			name:  "windows path",
			input: "Cannot find C:\\Users\\admin\\secrets\\keys.txt",
		},
		{
			name:  "go package path",
			input: "error in src-app/internal/gateway/handler.go:42",
		},
		{
			name:  "env var reference",
			input: "Missing $HOME directory or ${API_KEY} not set",
		},
		{
			name:  "go package",
			input: "panic in github.com/SeasAGI/SeasAGI-Client/internal/gateway/service.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Sanitize(tt.input)
			// Result should not contain the original sensitive substring patterns
			if strings.Contains(result, "/home/") || strings.Contains(result, "C:\\Users") {
				t.Errorf("Sanitize(%q) = %q, still contains path", tt.input, result)
			}
		})
	}
}

func TestErrorSanitizerStackTrace(t *testing.T) {
	s := NewErrorSanitizer(true)

	input := `error: something failed
goroutine 1 [running]:
main.main()
	/usr/local/go/src/main.go:10 +0x100
created by main.init
	/usr/local/go/src/main.go:5 +0x50`

	result := s.Sanitize(input)
	if strings.Contains(result, "goroutine") {
		t.Errorf("Sanitize should remove stack trace, got: %q", result)
	}
}

func TestErrorSanitizerDisabled(t *testing.T) {
	s := NewErrorSanitizer(false)

	input := "open /home/user/secret.txt: no such file"
	result := s.Sanitize(input)
	if result != input {
		t.Errorf("Disabled sanitizer should return original, got %q", result)
	}
}

func TestErrorSanitizerSanitizeIfEnabled(t *testing.T) {
	s := NewErrorSanitizer(false)

	input := "path /home/user/data"
	if s.SanitizeIfEnabled(input) != input {
		t.Error("SanitizeIfEnabled should not sanitize when disabled")
	}

	s.SetEnabled(true)
	result := s.SanitizeIfEnabled(input)
	if strings.Contains(result, "/home/user/data") {
		t.Error("SanitizeIfEnabled should sanitize when enabled")
	}
}

func TestErrorSanitizerInternalHost(t *testing.T) {
	s := NewErrorSanitizer(true)

	input := "Failed to connect to 192.168.1.100:8080"
	result := s.Sanitize(input)
	if strings.Contains(result, "192.168.1.100") {
		t.Errorf("Sanitize should mask internal host, got %q", result)
	}
}
