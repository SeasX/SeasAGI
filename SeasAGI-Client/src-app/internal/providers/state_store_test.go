package providers

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestStateStoreRoundTrip(t *testing.T) {
	store, err := NewStateStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewStateStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	pm := NewPenaltyManager()
	pm.RecordRateLimit("k1")
	pm.RecordRateLimit("k1") // penalty = 6

	cm := NewCooldownManager(time.Minute)
	defer cm.Stop()
	cm.SetCooldownWithReason("k2", "rate_limited")

	if err := store.SaveAll(pm, cm); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	pm2 := NewPenaltyManager()
	cm2 := NewCooldownManager(time.Minute)
	defer cm2.Stop()
	if err := store.LoadAll(pm2, cm2); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if got := pm2.GetPenalty("k1"); got != 6 {
		t.Fatalf("restored penalty = %d, want 6", got)
	}
	if !cm2.IsOnCooldown("k2") {
		t.Fatal("restored cooldown should be active for k2")
	}
	if cm2.IsOnCooldown("k1") {
		t.Fatal("k1 should not be on cooldown")
	}
}

func TestStateStorePersistsHashedCooldownKey(t *testing.T) {
	store, err := NewStateStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewStateStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	pm := NewPenaltyManager()
	cm := NewCooldownManager(time.Minute)
	defer cm.Stop()
	cm.SetCooldownWithReason("sk-raw-plaintext", "key_failed")

	if err := store.SaveAll(pm, cm); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	rows, err := store.db.Query("SELECT key FROM cooldown_state")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var found int
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatalf("scan: %v", err)
		}
		found++
		if key == "sk-raw-plaintext" {
			t.Fatal("raw API key must not be persisted as cooldown key")
		}
	}
	if found != 1 {
		t.Fatalf("expected 1 cooldown row, got %d", found)
	}
}

func TestStateStoreMarshalSnapshots(t *testing.T) {
	pm := NewPenaltyManager()
	pm.RecordRateLimit("a")
	data, err := pm.MarshalPenaltyState()
	if err != nil {
		t.Fatalf("MarshalPenaltyState: %v", err)
	}
	var penaltyRecords []penaltyStateRecord
	if err := json.Unmarshal(data, &penaltyRecords); err != nil {
		t.Fatalf("penalty snapshot not valid JSON: %v", err)
	}
	if len(penaltyRecords) != 1 || penaltyRecords[0].Key != "a" {
		t.Fatalf("unexpected penalty snapshot: %v", penaltyRecords)
	}

	cm := NewCooldownManager(time.Minute)
	defer cm.Stop()
	cm.SetCooldown("b")
	cdata, err := cm.MarshalCooldownState()
	if err != nil {
		t.Fatalf("MarshalCooldownState: %v", err)
	}
	var cooldownRecords []cooldownStateRecord
	if err := json.Unmarshal(cdata, &cooldownRecords); err != nil {
		t.Fatalf("cooldown snapshot not valid JSON: %v", err)
	}
	if len(cooldownRecords) != 1 {
		t.Fatalf("unexpected cooldown snapshot: %v", cooldownRecords)
	}
}

func TestStateStoreCloseIdempotent(t *testing.T) {
	store, err := NewStateStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewStateStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close should be a no-op, got: %v", err)
	}
}
