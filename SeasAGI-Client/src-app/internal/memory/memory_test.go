package memory

import (
	"testing"
)

func TestMemoryExtract(t *testing.T) {
	store := NewInMemoryStore()
	mgr := NewManager(store, MemoryConfig{Enabled: true})

	messages := []map[string]any{
		{"role": "user", "content": "I prefer using Go for backend development"},
		{"role": "assistant", "content": "Go is a great choice for backend. It is efficient and compiled."},
	}

	if err := mgr.ExtractAndStore(messages, "session-1"); err != nil {
		t.Fatalf("ExtractAndStore: %v", err)
	}

	entries, err := mgr.ListMemories()
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least 1 memory entry")
	}
}

func TestMemoryStore(t *testing.T) {
	store := NewInMemoryStore()
	entry := &MemoryEntry{
		Content:  "test memory",
		Category: "fact",
	}
	if err := store.Store(entry); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if entry.ID == "" {
		t.Error("expected ID to be set")
	}

	entries, _ := store.List()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestMemoryRetrieve(t *testing.T) {
	store := NewInMemoryStore()
	store.Store(&MemoryEntry{Content: "Go is a programming language", Category: "fact", Relevance: 0.9})
	store.Store(&MemoryEntry{Content: "Python is also popular", Category: "fact", Relevance: 0.5})

	results, err := store.Retrieve("Go", 1)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "Go is a programming language" {
		t.Errorf("expected Go memory, got %s", results[0].Content)
	}
}

func TestMemoryInject(t *testing.T) {
	store := NewInMemoryStore()
	store.Store(&MemoryEntry{Content: "User prefers concise answers", Category: "preference", Relevance: 0.9})

	mgr := NewManager(store, MemoryConfig{Enabled: true, InjectionCount: 5})
	messages := []map[string]any{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "concise"},
	}

	result, err := mgr.RetrieveAndInject(messages, "concise")
	if err != nil {
		t.Fatalf("RetrieveAndInject: %v", err)
	}

	// system message 应包含注入的记忆
	sysMsg, _ := result[0]["content"].(string)
	if sysMsg == "You are a helpful assistant." {
		t.Error("expected system message to contain injected memory")
	}
}

func TestMemoryInjectNoSystem(t *testing.T) {
	store := NewInMemoryStore()
	store.Store(&MemoryEntry{Content: "test memory", Category: "fact", Relevance: 0.9})

	mgr := NewManager(store, MemoryConfig{Enabled: true})
	messages := []map[string]any{
		{"role": "user", "content": "test"},
	}

	result, _ := mgr.RetrieveAndInject(messages, "test")
	if len(result) < 2 {
		t.Errorf("expected at least 2 messages (injected system + original), got %d", len(result))
	}
	if result[0]["role"] != "system" {
		t.Errorf("expected first message to be system, got %v", result[0]["role"])
	}
}

func TestMemoryDisabled(t *testing.T) {
	store := NewInMemoryStore()
	mgr := NewManager(store, MemoryConfig{Enabled: false})

	messages := []map[string]any{
		{"role": "user", "content": "I prefer Go"},
	}
	// 不应提取任何记忆
	_ = mgr.ExtractAndStore(messages, "session-1")

	entries, _ := mgr.ListMemories()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries when disabled, got %d", len(entries))
	}
}

func TestMemoryListDelete(t *testing.T) {
	store := NewInMemoryStore()
	mgr := NewManager(store, MemoryConfig{Enabled: true})

	mgr.AddMemory("memory 1", "fact")
	mgr.AddMemory("memory 2", "fact")

	entries, _ := mgr.ListMemories()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// 删除第一个
	mgr.DeleteMemory(entries[0].ID)
	entries, _ = mgr.ListMemories()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after delete, got %d", len(entries))
	}
}

func TestMemoryAddManual(t *testing.T) {
	store := NewInMemoryStore()
	mgr := NewManager(store, MemoryConfig{Enabled: true})

	if err := mgr.AddMemory("manual memory", "instruction"); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	entries, _ := mgr.ListMemories()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Category != "instruction" {
		t.Errorf("expected category instruction, got %s", entries[0].Category)
	}
}

func TestMemoryExtractPreference(t *testing.T) {
	store := NewInMemoryStore()
	mgr := NewManager(store, MemoryConfig{Enabled: true})

	messages := []map[string]any{
		{"role": "user", "content": "I prefer using dark mode"},
	}
	_ = mgr.ExtractAndStore(messages, "session-1")

	entries, _ := mgr.ListMemories()
	found := false
	for _, e := range entries {
		if e.Category == "preference" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to extract a preference memory")
	}
}

func TestMemoryExtractInstruction(t *testing.T) {
	store := NewInMemoryStore()
	mgr := NewManager(store, MemoryConfig{Enabled: true})

	messages := []map[string]any{
		{"role": "user", "content": "Always respond in Chinese"},
	}
	_ = mgr.ExtractAndStore(messages, "session-1")

	entries, _ := mgr.ListMemories()
	found := false
	for _, e := range entries {
		if e.Category == "instruction" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to extract an instruction memory")
	}
}
