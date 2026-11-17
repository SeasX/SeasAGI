package usage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func initTestDB(t *testing.T) {
	t.Helper()
	if err := database.Init(filepath.Join(t.TempDir(), "usage-test.db")); err != nil {
		t.Fatalf("database init failed: %v", err)
	}
	t.Cleanup(database.Close)
}

// 用量上报入口必须真正写入 usage_records，避免统计恒为 0。
func TestIngestUsageWritesRecords(t *testing.T) {
	initTestDB(t)

	router := gin.New()
	router.POST("/internal/usage", IngestUsage)

	body := `[{"request_id":"req-1","user_id":"user-1","channel_id":"ch-1","model":"gpt-4o-mini","input_tokens":12,"output_tokens":34,"cost_usd":0.0012,"total_cost_usd":0.0012,"status_code":200},
	          {"request_id":"req-2","user_id":"user-1","channel_id":"ch-1","model":"gpt-4o-mini","status_code":502,"error_category":"upstream"}]`
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/usage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("ingest status = %d: %s", recorder.Code, recorder.Body.String())
	}

	var count, input, output, errors int
	if err := database.DB.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(is_error),0)
		 FROM usage_records WHERE user_id = ?`, "user-1",
	).Scan(&count, &input, &output, &errors); err != nil {
		t.Fatalf("query usage failed: %v", err)
	}
	if count != 2 || input != 12 || output != 34 || errors != 1 {
		t.Fatalf("unexpected usage rows: count=%d input=%d output=%d errors=%d", count, input, output, errors)
	}
}

// 单个事件对象也应被接受。
func TestIngestUsageAcceptsSingleObject(t *testing.T) {
	initTestDB(t)

	router := gin.New()
	router.POST("/internal/usage", IngestUsage)

	body := `{"request_id":"req-single","user_id":"user-2","channel_id":"ch-2","model":"m"}`
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/usage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("ingest status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM usage_records WHERE user_id = ?`, "user-2").Scan(&count); err != nil {
		t.Fatalf("query usage failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

// 同一 request_id 重投只保留首行；成本字段不再被接收后丢弃。
func TestIngestUsageIsIdempotentAndPersistsCost(t *testing.T) {
	initTestDB(t)

	router := gin.New()
	router.POST("/internal/usage", IngestUsage)
	body := `{"request_id":"req-dup","user_id":"user-3","channel_id":"ch-3","model":"gpt-4o","input_tokens":100,"output_tokens":50,"cost_usd":0.0025,"total_cost_usd":0.0037}`

	for attempt, wantIngested := range []int{1, 0} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/internal/usage", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d: %s", attempt, rec.Code, rec.Body.String())
		}
		var resp struct {
			Ingested   int `json:"ingested"`
			Duplicates int `json:"duplicates"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Ingested != wantIngested {
			t.Fatalf("attempt %d: ingested = %d, want %d", attempt, resp.Ingested, wantIngested)
		}
		if attempt == 1 && resp.Duplicates != 1 {
			t.Fatalf("attempt 1: duplicates = %d, want 1", resp.Duplicates)
		}
	}

	var count int
	var cost, total float64
	if err := database.DB.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(cost_usd),0), COALESCE(SUM(total_cost_usd),0)
		 FROM usage_records WHERE request_id = 'req-dup'`,
	).Scan(&count, &cost, &total); err != nil {
		t.Fatalf("query usage: %v", err)
	}
	if count != 1 {
		t.Fatalf("duplicate rows = %d, want 1", count)
	}
	if cost != 0.0025 || total != 0.0037 {
		t.Fatalf("cost not persisted: cost_usd=%v total_cost_usd=%v", cost, total)
	}
}

// 缺少 request_id 会导致统计无法去重，必须整批拒绝而不是静默丢弃。
func TestIngestUsageRejectsMissingRequestID(t *testing.T) {
	initTestDB(t)

	router := gin.New()
	router.POST("/internal/usage", IngestUsage)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/usage",
		strings.NewReader(`{"user_id":"user-4","channel_id":"ch-4","model":"m"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	var count int
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM usage_records`).Scan(&count)
	if count != 0 {
		t.Fatalf("rejected batch wrote %d rows, want 0", count)
	}
}

// 内部上报入口必须鉴权：未配置 503、令牌错误 401、正确令牌放行。
func TestInternalAuthProtectsIngestRoute(t *testing.T) {
	cases := []struct {
		name   string
		env    string
		header string
		want   int
	}{
		{"not-configured", "", "", http.StatusServiceUnavailable},
		{"anonymous", "test-internal-token", "", http.StatusUnauthorized},
		{"wrong-token", "test-internal-token", "Bearer wrong", http.StatusUnauthorized},
		{"valid", "test-internal-token", "Bearer test-internal-token", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("INTERNAL_API_TOKEN", tc.env)
			t.Setenv("ADMIN_SECRET", "")

			router := gin.New()
			router.POST("/internal/usage", InternalAuth(), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/internal/usage", strings.NewReader("{}"))
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			router.ServeHTTP(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.want)
			}
		})
	}
}

// 周期起点必须按 UTC 计算，保证当天/周/月统计不漏首日。
func TestPeriodStartUTC(t *testing.T) {
	// 2026-10-25 是周日；UTC 当天起点应为 00:00:00。
	now := time.Date(2026, 10, 25, 23, 59, 59, 0, time.UTC)

	day := periodStartUTC(now, "day")
	if want := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC); !day.Equal(want) {
		t.Fatalf("day start = %v, want %v", day, want)
	}

	week := periodStartUTC(now, "week")
	if want := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC); !week.Equal(want) {
		t.Fatalf("week start = %v, want %v", week, want)
	}

	month := periodStartUTC(now, "month")
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !month.Equal(want) {
		t.Fatalf("month start = %v, want %v", month, want)
	}

	// 本地时区输入也应换算到 UTC。
	local := time.Date(2026, 10, 26, 7, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if got := periodStartUTC(local, "day"); !got.Equal(time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("local day start = %v, want 2026-10-25 UTC", got)
	}
}
