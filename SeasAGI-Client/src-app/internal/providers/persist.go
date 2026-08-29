package providers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// --- persistable state types ---

type penaltyStateRecord struct {
	Key       string    `json:"key"`
	Penalty   int       `json:"penalty"`
	LastHit   time.Time `json:"last_hit"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

type cooldownStateRecord struct {
	Key       string    `json:"key"`
	ExpiresAt time.Time `json:"expires_at"`
	Reason    string    `json:"reason"`
	UpdatedAt time.Time `json:"updated_at"`
}

// schema DDL for the penalty/cooldown persistence tables
const penaltySchemaSQL = `
CREATE TABLE IF NOT EXISTS penalty_state (
    key        TEXT PRIMARY KEY,
    penalty    INTEGER NOT NULL DEFAULT 0,
    last_hit   TEXT NOT NULL,
    count      INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cooldown_state (
    key        TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
`

// --- PenaltyManager persistence ---

// PersistToDB writes all penalty entries to the given database.
func (pm *PenaltyManager) PersistToDB(db *sql.DB) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("penalty persist: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Clear existing
	if _, err := tx.Exec("DELETE FROM penalty_state"); err != nil {
		return fmt.Errorf("penalty persist: clear: %w", err)
	}

	stmt, err := tx.Prepare("INSERT INTO penalty_state (key, penalty, last_hit, count, updated_at) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("penalty persist: prepare: %w", err)
	}
	defer stmt.Close()

	for key, entry := range pm.entries {
		// Apply decay before persisting
		elapsed := now.Sub(entry.LastHit)
		decaySteps := int(elapsed / pm.decayInterval)
		penalty := entry.Penalty - (decaySteps * pm.decayAmount)
		if penalty <= 0 {
			continue // skip expired entries
		}
		if _, err := stmt.Exec(key, penalty, entry.LastHit.UTC().Format(time.RFC3339), entry.Count, now.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("penalty persist: exec %s: %w", key, err)
		}
	}

	return tx.Commit()
}

// RestoreFromDB loads penalty entries from the given database.
func (pm *PenaltyManager) RestoreFromDB(db *sql.DB) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	rows, err := db.Query("SELECT key, penalty, last_hit, count FROM penalty_state")
	if err != nil {
		return fmt.Errorf("penalty restore: query: %w", err)
	}
	defer rows.Close()

	restored := make(map[string]*PenaltyEntry)
	for rows.Next() {
		var key string
		var penalty, count int
		var lastHitStr string
		if err := rows.Scan(&key, &penalty, &lastHitStr, &count); err != nil {
			return fmt.Errorf("penalty restore: scan: %w", err)
		}
		lastHit, err := time.Parse(time.RFC3339, lastHitStr)
		if err != nil {
			continue // skip invalid
		}
		// Check if still valid (not fully decayed)
		now := time.Now()
		elapsed := now.Sub(lastHit)
		decaySteps := int(elapsed / pm.decayInterval)
		currentPenalty := penalty - (decaySteps * pm.decayAmount)
		if currentPenalty <= 0 {
			continue // expired during downtime
		}
		restored[key] = &PenaltyEntry{
			Penalty: currentPenalty,
			LastHit: lastHit,
			Count:   count,
		}
	}

	pm.entries = restored
	return nil
}

// --- CooldownManager persistence ---

// PersistToDB writes all cooldown entries to the given database.
func (cm *CooldownManager) PersistToDB(db *sql.DB) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("cooldown persist: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM cooldown_state"); err != nil {
		return fmt.Errorf("cooldown persist: clear: %w", err)
	}

	stmt, err := tx.Prepare("INSERT INTO cooldown_state (key, expires_at, reason, updated_at) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("cooldown persist: prepare: %w", err)
	}
	defer stmt.Close()

	for key, entry := range cm.entries {
		if now.After(entry.ExpiresAt) {
			continue // skip expired
		}
		if _, err := stmt.Exec(key, entry.ExpiresAt.UTC().Format(time.RFC3339), entry.Reason, now.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("cooldown persist: exec %s: %w", key, err)
		}
	}

	return tx.Commit()
}

// RestoreFromDB loads cooldown entries from the given database.
func (cm *CooldownManager) RestoreFromDB(db *sql.DB) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	rows, err := db.Query("SELECT key, expires_at, reason FROM cooldown_state")
	if err != nil {
		return fmt.Errorf("cooldown restore: query: %w", err)
	}
	defer rows.Close()

	restored := make(map[string]*CooldownEntry)
	for rows.Next() {
		var key, reason string
		var expiresAtStr string
		if err := rows.Scan(&key, &expiresAtStr, &reason); err != nil {
			return fmt.Errorf("cooldown restore: scan: %w", err)
		}
		expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
		if err != nil {
			continue
		}
		if time.Now().After(expiresAt) {
			continue // expired during downtime
		}
		restored[key] = &CooldownEntry{
			ExpiresAt: expiresAt,
			Reason:    reason,
		}
	}

	cm.entries = restored
	return nil
}

// --- helper: marshal state for backup ---

// MarshalPenaltyState returns a JSON snapshot of all penalty entries (for backup/debugging).
func (pm *PenaltyManager) MarshalPenaltyState() ([]byte, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	records := make([]penaltyStateRecord, 0, len(pm.entries))
	for key, entry := range pm.entries {
		records = append(records, penaltyStateRecord{
			Key:       key,
			Penalty:   entry.Penalty,
			LastHit:   entry.LastHit,
			Count:     entry.Count,
			UpdatedAt: time.Now().UTC(),
		})
	}
	return json.Marshal(records)
}

// MarshalCooldownState returns a JSON snapshot of all cooldown entries.
func (cm *CooldownManager) MarshalCooldownState() ([]byte, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	records := make([]cooldownStateRecord, 0, len(cm.entries))
	for key, entry := range cm.entries {
		records = append(records, cooldownStateRecord{
			Key:       key,
			ExpiresAt: entry.ExpiresAt,
			Reason:    entry.Reason,
			UpdatedAt: time.Now().UTC(),
		})
	}
	return json.Marshal(records)
}
