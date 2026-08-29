package oauth

import (
	"sync"
	"testing"
	"time"
)

func TestNewTokenStore(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	if store.dir != dir {
		t.Errorf("expected dir %q, got %q", dir, store.dir)
	}
	if store.tokens == nil {
		t.Error("expected tokens map to be initialized, got nil")
	}
	if len(store.tokens) != 0 {
		t.Errorf("expected empty tokens map, got %d entries", len(store.tokens))
	}
}

func TestSaveAndGet(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	expiresAt := time.Now().Add(1 * time.Hour)
	info := &TokenInfo{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		ExpiresAt:    expiresAt,
		Scope:        "read write",
	}

	if err := store.Save("github", info); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Get("github")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil token info")
	}
	if got.AccessToken != "access-123" {
		t.Errorf("expected AccessToken %q, got %q", "access-123", got.AccessToken)
	}
	if got.RefreshToken != "refresh-456" {
		t.Errorf("expected RefreshToken %q, got %q", "refresh-456", got.RefreshToken)
	}
	if got.TokenType != "Bearer" {
		t.Errorf("expected TokenType %q, got %q", "Bearer", got.TokenType)
	}
	if !got.ExpiresAt.Equal(expiresAt) {
		t.Errorf("expected ExpiresAt %v, got %v", expiresAt, got.ExpiresAt)
	}
	if got.Scope != "read write" {
		t.Errorf("expected Scope %q, got %q", "read write", got.Scope)
	}
}

func TestSaveOverwritesExistingToken(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	info1 := &TokenInfo{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scope:        "read",
	}
	info2 := &TokenInfo{
		AccessToken:  "access-new",
		RefreshToken: "refresh-new",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		Scope:        "read write",
	}

	if err := store.Save("github", info1); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}
	if err := store.Save("github", info2); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	got, err := store.Get("github")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.AccessToken != "access-new" {
		t.Errorf("expected AccessToken %q, got %q", "access-new", got.AccessToken)
	}
	if got.RefreshToken != "refresh-new" {
		t.Errorf("expected RefreshToken %q, got %q", "refresh-new", got.RefreshToken)
	}
	if got.Scope != "read write" {
		t.Errorf("expected Scope %q, got %q", "read write", got.Scope)
	}
}

func TestGetReturnsErrorForUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	info, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if info != nil {
		t.Errorf("expected nil TokenInfo, got %+v", info)
	}
}

func TestDeleteRemovesToken(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	info := &TokenInfo{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	if err := store.Save("github", info); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := store.Delete("github"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	got, err := store.Get("github")
	if err == nil {
		t.Fatal("expected error after Delete, got nil")
	}
	if got != nil {
		t.Errorf("expected nil after Delete, got %+v", got)
	}
}

func TestDeleteUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	// Deleting a non-existent provider should not return an error
	if err := store.Delete("nonexistent"); err != nil {
		t.Errorf("expected no error when deleting unknown provider, got: %v", err)
	}
}

func TestListProviders(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	providers := []string{"github", "gitlab", "bitbucket"}
	for _, p := range providers {
		info := &TokenInfo{
			AccessToken:  "access-" + p,
			RefreshToken: "refresh-" + p,
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
		}
		if err := store.Save(p, info); err != nil {
			t.Fatalf("Save(%q) failed: %v", p, err)
		}
	}

	got := store.ListProviders()
	if len(got) != len(providers) {
		t.Fatalf("expected %d providers, got %d: %v", len(providers), len(got), got)
	}

	expected := make(map[string]bool)
	for _, p := range providers {
		expected[p] = true
	}
	for _, p := range got {
		if !expected[p] {
			t.Errorf("unexpected provider %q in list", p)
		}
		delete(expected, p)
	}
	if len(expected) > 0 {
		for p := range expected {
			t.Errorf("missing provider %q in list", p)
		}
	}
}

func TestListProvidersEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	got := store.ListProviders()
	if got == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d entries: %v", len(got), got)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	expiresAt := time.Now().Add(1 * time.Hour).Round(time.Second)
	info := &TokenInfo{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		ExpiresAt:    expiresAt,
		Scope:        "read write",
	}
	if err := store.Save("github", info); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Create a new store pointing to the same directory
	store2 := NewTokenStore(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	got, err := store2.Get("github")
	if err != nil {
		t.Fatalf("Get failed after Load: %v", err)
	}
	if got.AccessToken != "access-123" {
		t.Errorf("expected AccessToken %q, got %q", "access-123", got.AccessToken)
	}
	if got.RefreshToken != "refresh-456" {
		t.Errorf("expected RefreshToken %q, got %q", "refresh-456", got.RefreshToken)
	}
	if got.TokenType != "Bearer" {
		t.Errorf("expected TokenType %q, got %q", "Bearer", got.TokenType)
	}
	if !got.ExpiresAt.Equal(expiresAt) {
		t.Errorf("expected ExpiresAt %v, got %v", expiresAt, got.ExpiresAt)
	}
	if got.Scope != "read write" {
		t.Errorf("expected Scope %q, got %q", "read write", got.Scope)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	// No file has been written yet — Load should succeed with no error
	if err := store.Load(); err != nil {
		t.Errorf("expected no error when loading non-existent file, got: %v", err)
	}
}

func TestPersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	// First store: save a token
	store1 := NewTokenStore(dir)
	info1 := &TokenInfo{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Round(time.Second),
		Scope:        "repo",
	}
	if err := store1.Save("github", info1); err != nil {
		t.Fatalf("store1 Save failed: %v", err)
	}

	info2 := &TokenInfo{
		AccessToken:  "access-2",
		RefreshToken: "refresh-2",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour).Round(time.Second),
		Scope:        "api",
	}
	if err := store1.Save("gitlab", info2); err != nil {
		t.Fatalf("store1 Save(gitlab) failed: %v", err)
	}

	// Second store: load from disk and verify
	store2 := NewTokenStore(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("store2 Load failed: %v", err)
	}

	got1, err := store2.Get("github")
	if err != nil {
		t.Fatalf("store2 Get(github) failed: %v", err)
	}
	if got1.AccessToken != "access-1" || got1.RefreshToken != "refresh-1" || got1.Scope != "repo" {
		t.Errorf("got unexpected token info for github: %+v", got1)
	}

	got2, err := store2.Get("gitlab")
	if err != nil {
		t.Fatalf("store2 Get(gitlab) failed: %v", err)
	}
	if got2.AccessToken != "access-2" || got2.RefreshToken != "refresh-2" || got2.Scope != "api" {
		t.Errorf("got unexpected token info for gitlab: %+v", got2)
	}

	// Verify list matches
	providers := store2.ListProviders()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d: %v", len(providers), providers)
	}
}

func TestTokenStoreConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	// Pre-save a token for concurrent reads
	baseInfo := &TokenInfo{
		AccessToken:  "base-access",
		RefreshToken: "base-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	if err := store.Save("base", baseInfo); err != nil {
		t.Fatalf("Save(base) failed: %v", err)
	}

	var wg sync.WaitGroup
	n := 20

	// Concurrent saves
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			provider := "provider"
			info := &TokenInfo{
				AccessToken:  "access",
				RefreshToken: "refresh",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				Scope:        "scope",
			}
			_ = store.Save(provider, info)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = store.Get("base")
			_ = store.ListProviders()
		}()
	}

	// Concurrent deletes
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.Delete("base")
		}()
	}

	wg.Wait()

	// The test should not deadlock or panic; we just need to verify we can still
	// use the store after concurrent access.
	_ = store.ListProviders()
}
