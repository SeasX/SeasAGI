package relay

import (
	"path/filepath"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
)

func TestListRelayGatewaysFiltersDisabledAndInvalidRecords(t *testing.T) {
	initTestDB(t)

	mustExec(t, `INSERT INTO relay_gateways (gateway_id, name, host, port, region, enabled) VALUES (?, ?, ?, ?, ?, 1)`, "gw_enabled", "Enabled", "127.0.0.1", 8318, "local")
	mustExec(t, `INSERT INTO relay_gateways (gateway_id, name, host, port, region, enabled) VALUES (?, ?, ?, ?, ?, 0)`, "gw_disabled", "Disabled", "127.0.0.2", 8318, "local")
	mustExec(t, `INSERT INTO relay_gateways (gateway_id, name, host, port, region, enabled) VALUES (?, ?, ?, ?, ?, 1)`, "gw_invalid", "Invalid", "", 0, "local")

	items, err := listRelayGateways(true)
	if err != nil {
		t.Fatalf("listRelayGateways returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 enabled relay gateway, got %d", len(items))
	}
	if items[0].GatewayID != "gw_enabled" {
		t.Fatalf("expected gw_enabled, got %q", items[0].GatewayID)
	}
}

func initTestDB(t *testing.T) {
	t.Helper()

	database.Close()
	dbPath := filepath.Join(t.TempDir(), "platform-api-relay-test.db")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init failed: %v", err)
	}
	t.Cleanup(func() {
		database.Close()
	})
}

func mustExec(t *testing.T, query string, args ...any) {
	t.Helper()

	if _, err := database.DB.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v\nquery: %s", err, query)
	}
}
