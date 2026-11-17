package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

func TestListRelayGatewaysFiltersDisabledAndInvalidRecords(t *testing.T) {
	initTestDB(t)

	mustExec(t, `INSERT INTO relay_gateways (gateway_id, name, host, port, region, enabled) VALUES (?, ?, ?, ?, ?, 1)`, "gw_enabled", "Enabled", "127.0.0.1", 8318, "local")
	mustExec(t, `INSERT INTO relay_gateways (gateway_id, name, host, port, region, enabled) VALUES (?, ?, ?, ?, ?, 0)`, "gw_disabled", "Disabled", "127.0.0.2", 8318, "local")
	mustExec(t, `INSERT INTO relay_gateways (gateway_id, name, host, port, region, enabled) VALUES (?, ?, ?, ?, ?, 1)`, "gw_invalid", "Invalid", "", 0, "local")

	items, err := NewRepository(database.Writer).List(context.Background(), true)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
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

// 管理接口经注入的仓储完成 CRUD，响应契约（默认端口、部分更新、404、列表过滤）保持不变。
func TestAdminRelayGatewayCRUD(t *testing.T) {
	initTestDB(t)
	gin.SetMode(gin.TestMode)
	h := NewHandler(NewRepository(database.Writer))
	r := gin.New()
	r.GET("/public", h.ListRelayGateways)
	r.GET("/admin", h.AdminListRelayGateways)
	r.POST("/admin", h.AdminCreateRelayGateway)
	r.PUT("/admin/:id", h.AdminUpdateRelayGateway)
	r.DELETE("/admin/:id", h.AdminDeleteRelayGateway)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	list := func(path string) []RelayGateway {
		var out struct{ Data []RelayGateway }
		_ = json.Unmarshal(call(http.MethodGet, path, "").Body.Bytes(), &out)
		return out.Data
	}

	if w := call(http.MethodPost, "/admin", `{"gateway_id":"gw1","name":"One","host":"10.0.0.1"}`); w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body)
	}
	if w := call(http.MethodPost, "/admin", `{"gateway_id":"gw1"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid create = %d", w.Code)
	}
	if got := list("/admin"); len(got) != 1 || got[0].Port != 8318 || !got[0].Enabled {
		t.Fatalf("after create = %+v", got)
	}
	if w := call(http.MethodPut, "/admin/gw1", `{"enabled":false,"region":"eu"}`); w.Code != http.StatusOK {
		t.Fatalf("update = %d %s", w.Code, w.Body)
	}
	if got := list("/admin"); len(got) != 1 || got[0].Enabled || got[0].Region != "eu" || got[0].Name != "One" || got[0].Host != "10.0.0.1" {
		t.Fatalf("after partial update = %+v", got)
	}
	if got := list("/public"); len(got) != 0 {
		t.Fatalf("disabled gateway listed publicly: %+v", got)
	}
	if w := call(http.MethodPut, "/admin/missing", `{"name":"x"}`); w.Code != http.StatusNotFound {
		t.Fatalf("update missing = %d", w.Code)
	}
	if w := call(http.MethodDelete, "/admin/gw1", ""); w.Code != http.StatusOK {
		t.Fatalf("delete = %d", w.Code)
	}
	if got := list("/admin"); len(got) != 0 {
		t.Fatalf("after delete = %+v", got)
	}
}
