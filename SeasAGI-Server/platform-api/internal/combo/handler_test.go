package combo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/timeutil"
	"github.com/gin-gonic/gin"
)

// destCounter 记录 Scan 接收的参数个数，用于校验列清单与扫描顺序一致。
type destCounter struct {
	count int
}

func (d *destCounter) Scan(dest ...interface{}) error {
	d.count = len(dest)
	return nil
}

// 查询列清单必须与 scanCombo 的扫描目标数量一致（历史缺陷是 17 列 vs 19 目标）。
func TestComboSelectColumnsMatchScanDestinations(t *testing.T) {
	columns := strings.Split(comboSelectColumns, ",")
	counter := &destCounter{}
	if _, err := scanCombo(counter); err != nil {
		t.Fatalf("scanCombo failed: %v", err)
	}
	if len(columns) != counter.count {
		t.Fatalf("select columns = %d, scan destinations = %d", len(columns), counter.count)
	}
}

// 列清单必须包含后续迁移新增的 quick_strategy / task_profile。
func TestComboSelectColumnsIncludeTaskProfile(t *testing.T) {
	for _, col := range []string{"quick_strategy", "task_profile"} {
		if !strings.Contains(comboSelectColumns, col) {
			t.Fatalf("comboSelectColumns missing %q", col)
		}
	}
}

// 组合写入的时间字段必须统一为 SQLite 格式（不再混用 RFC3339）。
func TestCreateUserComboWritesSQLiteTimestamp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := database.Init(filepath.Join(t.TempDir(), "combo-test.db")); err != nil {
		t.Fatalf("database init failed: %v", err)
	}
	t.Cleanup(database.Close)

	router := gin.New()
	router.POST("/combos", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		CreateUserCombo(c)
	})

	payload := `{"logical_name":"fast","display_name":"Fast","steps":[{"model":"gpt-4o-mini"}]}`
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/combos", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("create combo status = %d: %s", recorder.Code, recorder.Body.String())
	}

	var comboID string
	var createdAt, updatedAt string
	// quote() 返回 SQL 字面量，绕过 go-sqlite3 对 DATETIME 列的 time.Time 自动转换，
	// 可直接观察磁盘上实际写入的字符串格式。
	if err := database.DB.QueryRow(
		`SELECT combo_id, quote(created_at), quote(updated_at) FROM model_combos WHERE owner_id = ?`, "user-1",
	).Scan(&comboID, &createdAt, &updatedAt); err != nil {
		t.Fatalf("query combo failed: %v", err)
	}

	sqliteLayout := regexp.MustCompile(`^'\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}'$`)
	for name, value := range map[string]string{"created_at": createdAt, "updated_at": updatedAt} {
		if !sqliteLayout.MatchString(value) {
			t.Fatalf("%s = %q, want SQLite layout %q", name, value, timeutil.SQLiteLayout)
		}
	}

	// 响应体中的时间也应保持同一格式。
	var resp struct {
		Data ModelCombo `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`).MatchString(resp.Data.CreatedAt) {
		t.Fatalf("response created_at = %q, want SQLite layout", resp.Data.CreatedAt)
	}
}
