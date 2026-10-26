package protocol

import "testing"

func TestParseGeminiPath(t *testing.T) {
	cases := []struct {
		path   string
		model  string
		action string
	}{
		{"/v1beta/models/gemini-2.5-pro:generateContent", "gemini-2.5-pro", "generateContent"},
		{"/v1beta/models/gemini-2.0-flash:streamGenerateContent", "gemini-2.0-flash", "streamGenerateContent"},
		{"/v1beta/models/", "", ""},
		{"/v1/chat/completions", "", ""},
	}
	for _, tc := range cases {
		model, action := ParseGeminiPath(tc.path)
		if model != tc.model || action != tc.action {
			t.Fatalf("ParseGeminiPath(%q) = (%q,%q), want (%q,%q)", tc.path, model, action, tc.model, tc.action)
		}
	}
}

// ParseRequestAs 必须按 Gemini 语义解析，不能落到自动探测（P0-10 回归）。
func TestParseRequestAsGemini(t *testing.T) {
	body := []byte(`{"model":"gemini-2.5-flash","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	req, err := ParseRequestAs(body, FormatGemini)
	if err != nil {
		t.Fatalf("ParseRequestAs failed: %v", err)
	}
	if req.Model != "gemini-2.5-flash" {
		t.Fatalf("unexpected model %q", req.Model)
	}
	if req.SourceFormat != FormatGemini {
		t.Fatalf("unexpected source format %v", req.SourceFormat)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
}
