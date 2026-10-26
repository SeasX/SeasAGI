package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
)

func initAttackDB(t *testing.T) {
	t.Helper()

	database.Close()
	if err := database.Init(filepath.Join(t.TempDir(), "attack-matrix.db")); err != nil {
		t.Fatalf("database.Init failed: %v", err)
	}
	t.Cleanup(database.Close)
}

func setAttackEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIN_MODE", "test")
	t.Setenv("SEASAGI_ALLOW_DEV_SECRETS", "1")
	t.Setenv("JWT_SECRET", "attack-matrix-jwt-secret-0123456789")
	t.Setenv("JWT_REFRESH_SECRET", "attack-matrix-refresh-secret-0123456789")
	t.Setenv("ADMIN_SECRET", "attack-matrix-admin-secret")
	t.Setenv("INTERNAL_API_TOKEN", "attack-matrix-internal-token")
}

func doJSON(t *testing.T, router http.Handler, method, path, token string, headers map[string]string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body failed: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func registerAttackUser(t *testing.T, router http.Handler, email string) (string, string) {
	t.Helper()

	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", "", nil, map[string]string{
		"email":    email,
		"password": "Passw0rd!attack-matrix",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s failed: %d %s", email, rec.Code, rec.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.AccessToken == "" {
		t.Fatalf("register %s returned no access token: %s", email, rec.Body.String())
	}

	var userID string
	if err := database.DB.QueryRow(`SELECT user_id FROM users WHERE email = ?`, strings.ToLower(email)).Scan(&userID); err != nil {
		t.Fatalf("load user %s failed: %v", email, err)
	}
	return resp.AccessToken, userID
}

func mustExecAttack(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := database.DB.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v\nquery: %s", err, query)
	}
}

func mustScanAttack(t *testing.T, query string, args []any, dest ...any) {
	t.Helper()
	if err := database.DB.QueryRow(query, args...).Scan(dest...); err != nil {
		t.Fatalf("scan failed: %v\nquery: %s", err, query)
	}
}

// TestAttackMatrixCrossUserWrites 覆盖社区版开源客户端场景下的跨用户越权、
// 伪造头、重复上报：
//   - B 修改/删除 A 的 combo → 404/403 且 A 数据不变；
//   - B 解绑 A 的设备 → 404 且绑定关系仍在；
//   - 伪造 X-Tenant-Id / X-Device-Id 不改变用户接口的数据归属；
//   - 同一 request_id 重复上报用量：社区版 usage_records 没有 request_id 列，
//     usage 上报入口也不做客户端幂等（request_id 字段被忽略），因此按服务端规则
//     每次已鉴权的内部上报都记为独立事件（不产生同一请求内的重复行）。
//     若后续为社区版增加 request_id 幂等，应改为断言第二次 ingested=0。
func TestAttackMatrixCrossUserWrites(t *testing.T) {
	initAttackDB(t)
	setAttackEnv(t)

	router, err := BuildRouter()
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}

	tokenA, userA := registerAttackUser(t, router, "alice@attack-matrix.test")
	tokenB, userB := registerAttackUser(t, router, "bob@attack-matrix.test")
	if userA == userB {
		t.Fatal("expected distinct users")
	}

	t.Run("cross_user_combo_update_and_delete", func(t *testing.T) {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/combos", tokenA, nil, map[string]any{
			"logical_name": "alice-combo",
			"display_name": "A combo",
			"steps": []map[string]any{{
				"model":      "gpt-4o-mini",
				"channel_id": "ch-1",
			}},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create combo failed: %d %s", rec.Code, rec.Body.String())
		}
		var created struct {
			Data struct {
				ComboID string `json:"combo_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Data.ComboID == "" {
			t.Fatalf("decode created combo failed: %s", rec.Body.String())
		}
		comboID := created.Data.ComboID

		// B 修改 A 的 combo：所有权不匹配，返回 404，A 的记录保持原样。
		rec = doJSON(t, router, http.MethodPut, "/api/v1/combos/"+comboID, tokenB, nil, map[string]any{
			"display_name": "hijacked",
		})
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
			t.Fatalf("foreign combo update returned %d, want 403/404", rec.Code)
		}
		var displayName, status string
		var version int
		mustScanAttack(t, `SELECT display_name, status, version FROM model_combos WHERE combo_id = ? AND owner_id = ?`,
			[]any{comboID, userA}, &displayName, &status, &version)
		if displayName != "A combo" || status == "archived" || version != 1 {
			t.Fatalf("A combo was modified: display_name=%q status=%q version=%d", displayName, status, version)
		}

		// B 删除 A 的 combo：同样必须拒绝，不能软删除 A 的记录。
		rec = doJSON(t, router, http.MethodDelete, "/api/v1/combos/"+comboID, tokenB, nil, nil)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
			t.Fatalf("foreign combo delete returned %d, want 403/404", rec.Code)
		}
		mustScanAttack(t, `SELECT status FROM model_combos WHERE combo_id = ?`, []any{comboID}, &status)
		if status == "archived" {
			t.Fatal("foreign combo delete archived A's record")
		}
	})

	t.Run("cross_user_device_unbind", func(t *testing.T) {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/devices/bind", tokenA, nil, map[string]any{
			"device_name": "Alice MacBook",
			"platform":    "darwin",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("bind device failed: %d %s", rec.Code, rec.Body.String())
		}
		var bound struct {
			DeviceID string `json:"device_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &bound); err != nil || bound.DeviceID == "" {
			t.Fatalf("decode bound device failed: %s", rec.Body.String())
		}

		rec = doJSON(t, router, http.MethodDelete, "/api/v1/devices/"+bound.DeviceID, tokenB, nil, nil)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
			t.Fatalf("foreign device unbind returned %d, want 403/404", rec.Code)
		}
		var count int
		mustScanAttack(t, `SELECT COUNT(*) FROM devices WHERE device_id = ? AND user_id = ?`,
			[]any{bound.DeviceID, userA}, &count)
		if count != 1 {
			t.Fatalf("A device binding was removed by B, count=%d", count)
		}
	})

	t.Run("forged_tenant_and_device_headers", func(t *testing.T) {
		// 社区版没有租户作用域的用户接口：用户身份只取 JWT claim，
		// 租户只从 users 表读取，X-Tenant-Id / X-Device-Id 均不参与数据归属。
		mustExecAttack(t, `INSERT INTO usage_records (user_id, device_id, channel_id, model, request_count) VALUES (?, 'alice-device', 'ch-1', 'm', 7)`, userA)
		mustExecAttack(t, `INSERT INTO usage_records (user_id, device_id, channel_id, model, request_count) VALUES (?, 'bob-device', 'ch-1', 'm', 2)`, userB)

		forged := map[string]string{
			"X-Tenant-Id": "tenant-owned-by-alice",
			"X-Device-Id": "alice-device",
		}

		rec := doJSON(t, router, http.MethodGet, "/api/v1/user/profile", tokenB, forged, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("profile request failed: %d %s", rec.Code, rec.Body.String())
		}
		var profile struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &profile); err != nil {
			t.Fatalf("decode profile failed: %v", err)
		}
		if profile.UserID != userB {
			t.Fatalf("forged headers changed profile identity: got %q, want %q", profile.UserID, userB)
		}

		rec = doJSON(t, router, http.MethodGet, "/api/v1/usage?period=day", tokenB, forged, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("usage query failed: %d %s", rec.Code, rec.Body.String())
		}
		var usage struct {
			TotalRequests int `json:"total_requests"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &usage); err != nil {
			t.Fatalf("decode usage failed: %v", err)
		}
		if usage.TotalRequests != 2 {
			t.Fatalf("usage scoped to forged tenant/device: total_requests=%d, want 2", usage.TotalRequests)
		}
	})

	t.Run("replayed_usage_request_id", func(t *testing.T) {
		internal := map[string]string{"Authorization": "Bearer attack-matrix-internal-token"}
		body := map[string]any{
			"user_id":       userA,
			"device_id":     "alice-device",
			"channel_id":    "ch-replay",
			"model":         "gpt-4o-mini",
			"input_tokens":  12,
			"output_tokens": 8,
			"request_id":    "replayed-request-id",
		}

		first := doJSON(t, router, http.MethodPost, "/api/v1/internal/usage", "", internal, body)
		second := doJSON(t, router, http.MethodPost, "/api/v1/internal/usage", "", internal, body)
		if first.Code != http.StatusOK || second.Code != http.StatusOK {
			t.Fatalf("usage ingest failed: %d / %d", first.Code, second.Code)
		}
		var firstResp, secondResp struct {
			Ingested int `json:"ingested"`
		}
		_ = json.Unmarshal(first.Body.Bytes(), &firstResp)
		_ = json.Unmarshal(second.Body.Bytes(), &secondResp)
		// 社区版不做 request_id 幂等：每次上报各自生成一行，客户端 request_id 被忽略。
		if firstResp.Ingested != 1 || secondResp.Ingested != 1 {
			t.Fatalf("community ingest rule changed: first=%d second=%d (expected 1/1)", firstResp.Ingested, secondResp.Ingested)
		}
		var count int
		mustScanAttack(t, `SELECT COUNT(*) FROM usage_records WHERE user_id = ? AND channel_id = 'ch-replay'`,
			[]any{userA}, &count)
		if count != 2 {
			t.Fatalf("expected two independent usage rows (no client-side idempotency), got %d", count)
		}
	})
}
