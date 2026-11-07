package httpguard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsLoopbackRemoteAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:54321": true,
		"[::1]:54321":     true,
		"127.0.0.1":       true,
		"192.168.1.5:80":  false,
		"10.0.0.2:80":     false,
		"":                false,
		"not-an-ip:80":    false,
	}
	for addr, want := range cases {
		if got := IsLoopbackRemoteAddr(addr); got != want {
			t.Errorf("IsLoopbackRemoteAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func newGuardRequest(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Host = "127.0.0.1:4318"
	r.RemoteAddr = "127.0.0.1:54321"
	return r
}

func TestLocalOnlyAllowsLoopbackClient(t *testing.T) {
	handler := LocalOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newGuardRequest(http.MethodGet, "/"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLocalOnlyRejectsNonLoopbackClient(t *testing.T) {
	handler := LocalOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := newGuardRequest(http.MethodGet, "/")
	r.RemoteAddr = "192.168.1.5:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestLocalOnlyRejectsMissingRemoteAddr(t *testing.T) {
	handler := LocalOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := newGuardRequest(http.MethodGet, "/")
	r.RemoteAddr = ""
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestLocalOnlyRejectsForeignHostAndOrigin(t *testing.T) {
	handler := LocalOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	foreignHost := newGuardRequest(http.MethodGet, "/")
	foreignHost.Host = "evil.example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, foreignHost)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign host: expected 403, got %d", rec.Code)
	}

	crossOrigin := newGuardRequest(http.MethodGet, "/")
	crossOrigin.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, crossOrigin)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross origin: expected 403, got %d", rec.Code)
	}

	localOrigin := newGuardRequest(http.MethodGet, "/")
	localOrigin.Header.Set("Origin", "http://localhost:5173")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, localOrigin)
	if rec.Code != http.StatusOK {
		t.Fatalf("local origin: expected 200, got %d", rec.Code)
	}
}

func TestLocalOnlyRejectsOversizeBody(t *testing.T) {
	handler := LocalOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := newGuardRequest(http.MethodPost, "/")
	r.Body = http.NoBody
	r.ContentLength = maxBodyBytes + 1
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestIsLocalOrigin(t *testing.T) {
	if !IsLocalOrigin("http://127.0.0.1:17600") {
		t.Error("loopback origin should be allowed")
	}
	if IsLocalOrigin("https://evil.example.com") {
		t.Error("foreign origin should be rejected")
	}
	if IsLocalOrigin(strings.Repeat("x", 4)) {
		t.Error("garbage origin should be rejected")
	}
}
