package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTestDir sets HOME to a temp directory and returns the cleanup function.
func setupTestDir(t *testing.T) (string, func()) {
	t.Helper()
	origHome := os.Getenv("HOME")
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	return tempDir, func() {
		os.Setenv("HOME", origHome)
	}
}

// createSessionFile writes a session JSON file under the claude app directory.
// If data is nil, an empty file is created.
func createSessionFile(t *testing.T, homeDir, id string, data map[string]interface{}) string {
	t.Helper()
	dir := filepath.Join(homeDir, ".claude", "projects")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if data != nil {
		if err := json.NewEncoder(f).Encode(data); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// setModTime sets the modification time of a file.
func setModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func TestNewService(t *testing.T) {
	s := NewService()
	if s == nil {
		t.Fatal("NewService() returned nil")
	}
}

func TestListSessions_EmptyDir(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	// Create the directory structure so Walk doesn't error
	dir := filepath.Join(homeDir, ".claude", "projects")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	s := NewService()
	sessions, err := s.ListSessions("claude", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessions_WithFiles(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	createSessionFile(t, homeDir, "session1", map[string]interface{}{
		"title": "Session One",
		"model": "claude-3-opus",
	})
	createSessionFile(t, homeDir, "session2", map[string]interface{}{
		"title": "Session Two",
		"model": "claude-3-sonnet",
	})

	s := NewService()
	sessions, err := s.ListSessions("claude", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify sessions are populated with data from the JSON files
	ids := make(map[string]bool)
	titles := make(map[string]bool)
	models := make(map[string]bool)
	for _, sess := range sessions {
		ids[sess.ID] = true
		titles[sess.Title] = true
		models[sess.Model] = true
	}
	if !ids["session1"] || !ids["session2"] {
		t.Fatalf("expected sessions session1 and session2, got %v", ids)
	}
	if !titles["Session One"] || !titles["Session Two"] {
		t.Fatalf("expected titles 'Session One' and 'Session Two', got %v", titles)
	}
	if !models["claude-3-opus"] || !models["claude-3-sonnet"] {
		t.Fatalf("expected models 'claude-3-opus' and 'claude-3-sonnet', got %v", models)
	}
}

func TestListSessions_Limit(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	// Create 3 sessions
	createSessionFile(t, homeDir, "session1", map[string]interface{}{"title": "A"})
	createSessionFile(t, homeDir, "session2", map[string]interface{}{"title": "B"})
	createSessionFile(t, homeDir, "session3", map[string]interface{}{"title": "C"})

	s := NewService()
	sessions, err := s.ListSessions("claude", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestGetSession(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	createSessionFile(t, homeDir, "mysession", map[string]interface{}{
		"title": "My Session",
		"model": "claude-3-opus",
	})

	s := NewService()
	session, err := s.GetSession("claude", "mysession")
	if err != nil {
		t.Fatal(err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.ID != "mysession" {
		t.Fatalf("expected ID 'mysession', got %q", session.ID)
	}
	if session.Title != "My Session" {
		t.Fatalf("expected Title 'My Session', got %q", session.Title)
	}
	if session.Model != "claude-3-opus" {
		t.Fatalf("expected Model 'claude-3-opus', got %q", session.Model)
	}
	if session.App != "claude" {
		t.Fatalf("expected App 'claude', got %q", session.App)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	// Create the directory but no session files
	dir := filepath.Join(homeDir, ".claude", "projects")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	s := NewService()
	session, err := s.GetSession("claude", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if session != nil {
		t.Fatal("expected nil session for non-existent ID")
	}
}

func TestGetMessages(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	createSessionFile(t, homeDir, "chat1", map[string]interface{}{
		"title": "Chat",
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "Hello",
			},
			map[string]interface{}{
				"role":    "assistant",
				"content": "Hi there!",
				"model":   "claude-3-opus",
			},
		},
	})

	s := NewService()
	messages, err := s.GetMessages("claude", "chat1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Hello" {
		t.Fatalf("unexpected first message: role=%q content=%q", messages[0].Role, messages[0].Content)
	}
	if messages[1].Role != "assistant" || messages[1].Content != "Hi there!" || messages[1].Model != "claude-3-opus" {
		t.Fatalf("unexpected second message: role=%q content=%q model=%q", messages[1].Role, messages[1].Content, messages[1].Model)
	}
}

func TestGetMessages_Empty(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	createSessionFile(t, homeDir, "chat1", map[string]interface{}{
		"title":    "Empty Chat",
		"messages": []interface{}{},
	})

	s := NewService()
	messages, err := s.GetMessages("claude", "chat1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(messages))
	}
}

func TestSearchSessions_ByTitle(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	createSessionFile(t, homeDir, "session1", map[string]interface{}{
		"title": "Go Programming",
		"model": "claude-3-opus",
	})
	createSessionFile(t, homeDir, "session2", map[string]interface{}{
		"title": "Rust Programming",
		"model": "claude-3-sonnet",
	})
	createSessionFile(t, homeDir, "session3", map[string]interface{}{
		"title": "Cooking Recipes",
		"model": "claude-3-haiku",
	})

	s := NewService()
	sessions, err := s.SearchSessions("claude", "Programming", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSearchSessions_ByModel(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	createSessionFile(t, homeDir, "session1", map[string]interface{}{
		"title": "Project Alpha",
		"model": "claude-3-opus",
	})
	createSessionFile(t, homeDir, "session2", map[string]interface{}{
		"title": "Project Beta",
		"model": "claude-3-sonnet",
	})
	createSessionFile(t, homeDir, "session3", map[string]interface{}{
		"title": "Project Gamma",
		"model": "claude-3-opus",
	})

	s := NewService()
	sessions, err := s.SearchSessions("claude", "opus", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions matching model 'opus', got %d", len(sessions))
	}
}

func TestSearchSessions_NoMatch(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	createSessionFile(t, homeDir, "session1", map[string]interface{}{
		"title": "Go Programming",
		"model": "claude-3-opus",
	})
	createSessionFile(t, homeDir, "session2", map[string]interface{}{
		"title": "Rust Programming",
		"model": "claude-3-sonnet",
	})

	s := NewService()
	sessions, err := s.SearchSessions("claude", "Python", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestSearchSessions_CaseInsensitive(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	createSessionFile(t, homeDir, "session1", map[string]interface{}{
		"title": "Go Programming",
		"model": "claude-3-opus",
	})
	createSessionFile(t, homeDir, "session2", map[string]interface{}{
		"title": "Rust Programming",
		"model": "claude-3-sonnet",
	})

	s := NewService()
	// Search with different case from the stored title
	sessions, err := s.SearchSessions("claude", "programming", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions for case-insensitive search, got %d", len(sessions))
	}
}

func TestDeleteSession(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	path := createSessionFile(t, homeDir, "session1", map[string]interface{}{
		"title": "To Delete",
		"model": "claude-3-opus",
	})

	// Verify file exists before deletion
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("session file should exist before deletion")
	}

	s := NewService()
	err := s.DeleteSession("claude", "session1")
	if err != nil {
		t.Fatal(err)
	}

	// Verify file is gone after deletion
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("session file should be deleted")
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	homeDir, cleanup := setupTestDir(t)
	defer cleanup()

	// Create the directory but no session files
	dir := filepath.Join(homeDir, ".claude", "projects")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	s := NewService()
	err := s.DeleteSession("claude", "nonexistent")
	// The implementation returns nil when session is not found
	if err != nil {
		t.Fatalf("expected nil error for non-existent session, got %v", err)
	}
}