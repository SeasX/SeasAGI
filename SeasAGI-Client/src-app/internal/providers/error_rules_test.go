package providers

import (
	"net/http"
	"testing"
)

func TestErrorClassifierStatusCodes(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name      string
		status    int
		message   string
		wantClass ErrorClass
		wantRetry bool
	}{
		{"429 rate limit", 429, "too many requests", ErrorClassRateLimit, true},
		{"401 auth", 401, "unauthorized", ErrorClassAuth, false},
		{"403 auth", 403, "forbidden", ErrorClassAuth, false},
		{"404 model", 404, "model not found", ErrorClassModel, false},
		{"400 context", 400, "context length exceeded", ErrorClassContext, false},
		{"400 generic", 400, "bad request", ErrorClassPermanent, false},
		{"500 server", 500, "internal error", ErrorClassServer, true},
		{"503 server", 503, "unavailable", ErrorClassServer, true},
		{"599 fallback server", 599, "weird", ErrorClassServer, true},
		{"418 unknown", 418, "I'm a teapot", ErrorClassUnknown, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			class, retry := ec.Classify(tc.status, tc.message)
			if class != tc.wantClass || retry != tc.wantRetry {
				t.Fatalf("Classify(%d, %q) = (%s, %v), want (%s, %v)",
					tc.status, tc.message, class, retry, tc.wantClass, tc.wantRetry)
			}
		})
	}
}

func TestErrorClassifierSubstringMatching(t *testing.T) {
	ec := NewErrorClassifier()

	// 404 仅当消息包含 model/not found 时归类为 model；否则落到 unknown。
	if class, _ := ec.Classify(404, "gateway route missing"); class != ErrorClassUnknown {
		t.Fatalf("404 without model keyword = %s, want unknown", class)
	}
	if class, _ := ec.Classify(400, "TOKENS LIMIT reached"); class != ErrorClassContext {
		t.Fatalf("400 with token keyword = %s, want context", class)
	}
}

func TestErrorClassifierAddRule(t *testing.T) {
	ec := NewErrorClassifier()
	ec.AddRule(ErrorRule{
		StatusCodes: []int{418},
		Class:       ErrorClassPermanent,
		Retryable:   false,
	})
	if class, _ := ec.Classify(418, "teapot"); class != ErrorClassPermanent {
		t.Fatalf("custom rule not applied, got %s", class)
	}
}

func TestClassifyHelpers(t *testing.T) {
	if !IsRetryableHTTPError(429, "") {
		t.Error("429 should be retryable")
	}
	if IsRetryableHTTPError(401, "") {
		t.Error("401 should not be retryable")
	}
	if _, retryable := ClassifyHTTPError(500, ""); !retryable {
		t.Error("500 should be retryable")
	}
}

func TestShouldRotateKey(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests} {
		if !ShouldRotateKey(code) {
			t.Errorf("status %d should rotate key", code)
		}
	}
	for _, code := range []int{http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError} {
		if ShouldRotateKey(code) {
			t.Errorf("status %d should not rotate key", code)
		}
	}
}
