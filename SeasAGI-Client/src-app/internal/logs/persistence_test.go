package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// testPersistCount 是持久化相关用例写入的日志条数（保持在 maxLogs 以内）。
const testPersistCount = 10

func newTestService(t *testing.T) *Service {
	t.Helper()
	return &Service{
		logs:     make([]RequestLog, 0, 8),
		filePath: filepath.Join(t.TempDir(), "logs.json"),
	}
}

func TestRecordLogPrependsAndSetsTimestamp(t *testing.T) {
	s := newTestService(t)
	_ = s.RecordLog(RequestLog{RequestID: "r1", LogicalModelName: "gpt-4o"})
	_ = s.RecordLog(RequestLog{RequestID: "r2", LogicalModelName: "claude"})

	got, err := s.ListLogs(0, 0)
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].RequestID != "r2" {
		t.Fatalf("newest log should be first, got %q", got[0].RequestID)
	}
	if got[0].CreatedAt == "" {
		t.Fatal("CreatedAt should be auto-populated")
	}
}

func TestListLogsFiltered(t *testing.T) {
	s := newTestService(t)
	_ = s.RecordLog(RequestLog{RequestID: "r1", Status: "success", ChannelID: "ch1", UpstreamModel: "gpt-4o", CreatedAt: "2024-01-01T00:00:00Z"})
	_ = s.RecordLog(RequestLog{RequestID: "r2", Status: "failure", ChannelID: "ch2", LogicalModelName: "claude", CreatedAt: "2024-06-01T00:00:00Z"})

	byStatus, _ := s.ListLogsFiltered(0, 0, "failure", "", "", "", "")
	if len(byStatus) != 1 || byStatus[0].RequestID != "r2" {
		t.Fatalf("status filter wrong: %+v", byStatus)
	}

	byChannel, _ := s.ListLogsFiltered(0, 0, "", "ch1", "", "", "")
	if len(byChannel) != 1 || byChannel[0].RequestID != "r1" {
		t.Fatalf("channel filter wrong: %+v", byChannel)
	}

	byKeyword, _ := s.ListLogsFiltered(0, 0, "", "", "", "", "CLAUDE")
	if len(byKeyword) != 1 || byKeyword[0].RequestID != "r2" {
		t.Fatalf("keyword filter (case-insensitive) wrong: %+v", byKeyword)
	}

	byTime, _ := s.ListLogsFiltered(0, 0, "", "", "2024-03-01T00:00:00Z", "", "")
	if len(byTime) != 1 || byTime[0].RequestID != "r2" {
		t.Fatalf("time filter wrong: %+v", byTime)
	}
}

func TestListLogsPagination(t *testing.T) {
	s := newTestService(t)
	for i := 0; i < 5; i++ {
		_ = s.RecordLog(RequestLog{RequestID: string(rune('a' + i))})
	}
	// 内存顺序（最新在前）：e,d,c,b,a；offset=1 limit=2 → d,c
	got, _ := s.ListLogs(2, 1)
	if len(got) != 2 || got[0].RequestID != "d" || got[1].RequestID != "c" {
		t.Fatalf("pagination wrong: %+v", got)
	}

	// offset 越界返回空
	out, _ := s.ListLogs(10, 100)
	if len(out) != 0 {
		t.Fatalf("out-of-range offset should return empty, got %d", len(out))
	}
}

func TestPersistAndReload(t *testing.T) {
	s := newTestService(t)
	for i := 0; i < testPersistCount; i++ {
		_ = s.RecordLog(RequestLog{RequestID: "r" + string(rune('0'+i))})
	}

	if _, err := os.Stat(s.filePath); err != nil {
		t.Fatalf("log file should be persisted after %d records: %v", testPersistCount, err)
	}

	reloaded := &Service{filePath: s.filePath}
	reloaded.loadFromDisk()
	if len(reloaded.logs) != testPersistCount {
		t.Fatalf("reloaded %d logs, want %d", len(reloaded.logs), testPersistCount)
	}
}

func TestClearLogs(t *testing.T) {
	s := newTestService(t)
	_ = s.RecordLog(RequestLog{RequestID: "r1"})
	if err := s.ClearLogs(); err != nil {
		t.Fatalf("ClearLogs: %v", err)
	}
	got, _ := s.ListLogs(0, 0)
	if len(got) != 0 {
		t.Fatalf("logs should be cleared, got %d", len(got))
	}
	// 清空后应落盘为空（追加日志格式下为空文件），重新加载不得恢复旧条目。
	reloaded := &Service{filePath: s.filePath}
	reloaded.loadFromDisk()
	if len(reloaded.logs) != 0 {
		t.Fatalf("cleared file should hold no logs, got %d", len(reloaded.logs))
	}
}

func TestLoadFromDiskTrimsAndIgnoresMalformed(t *testing.T) {
	// 超过 maxLogs 的条目应在加载时截断
	over := make([]RequestLog, maxLogs+10)
	for i := range over {
		over[i] = RequestLog{RequestID: "r"}
	}
	data, _ := json.Marshal(over)
	path := filepath.Join(t.TempDir(), "logs.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Service{filePath: path}
	s.loadFromDisk()
	if len(s.logs) != maxLogs {
		t.Fatalf("loaded %d logs, want trimmed to %d", len(s.logs), maxLogs)
	}

	// 损坏文件不应 panic，也不应改变已有内存数据
	bad := &Service{logs: []RequestLog{{RequestID: "keep"}}, filePath: filepath.Join(t.TempDir(), "bad.json")}
	_ = os.WriteFile(bad.filePath, []byte("{not json"), 0o600)
	bad.loadFromDisk()
	if len(bad.logs) != 1 || bad.logs[0].RequestID != "keep" {
		t.Fatalf("malformed file should be ignored, got %+v", bad.logs)
	}
}

// TestLoadMigratesLegacyJSONArrayToAppendLog 验证旧版整文件 JSON 数组在加载后
// 迁移为追加日志，后续追加写入不会破坏历史数据。
func TestLoadMigratesLegacyJSONArrayToAppendLog(t *testing.T) {
	legacy := []RequestLog{
		{RequestID: "newest"},
		{RequestID: "older"},
	}
	backfillHashes(legacy)
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy logs: %v", err)
	}
	path := filepath.Join(t.TempDir(), "logs.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write legacy logs: %v", err)
	}

	s := &Service{filePath: path}
	s.loadFromDisk()
	if len(s.logs) != 2 {
		t.Fatalf("expected 2 legacy logs, got %d", len(s.logs))
	}

	// 迁移后继续追加，重新加载时必须同时保留历史与新增记录。
	_ = s.RecordLog(RequestLog{RequestID: "fresh"})

	reloaded := &Service{filePath: path}
	reloaded.loadFromDisk()
	if len(reloaded.logs) != 3 {
		t.Fatalf("expected 3 logs after migration + append, got %d", len(reloaded.logs))
	}
	if reloaded.logs[0].RequestID != "fresh" || reloaded.logs[2].RequestID != "older" {
		t.Fatalf("unexpected log order after migration: %q ... %q",
			reloaded.logs[0].RequestID, reloaded.logs[len(reloaded.logs)-1].RequestID)
	}
	if idx := reloaded.VerifyIntegrity(); idx != -1 {
		t.Fatalf("reloaded hash chain broken at %d", idx)
	}
}

// TestConcurrentRecordLogKeepsAllEntries 验证并发写入不丢条目：
// 全部请求 ID 都应保留，哈希链完整，且落盘后可完整回放。
func TestConcurrentRecordLogKeepsAllEntries(t *testing.T) {
	s := newTestService(t)

	const writers = 16
	const perWriter = 10
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				_ = s.RecordLog(RequestLog{RequestID: fmt.Sprintf("w%d-%d", w, i)})
			}
		}(w)
	}
	wg.Wait()

	total := writers * perWriter
	if total > maxLogs {
		t.Fatalf("test setup writes more than maxLogs, entries would be evicted")
	}

	got, err := s.ListLogs(total, 0)
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if len(got) != total {
		t.Fatalf("expected %d logs after concurrent writes, got %d", total, len(got))
	}
	seen := make(map[string]bool, total)
	for _, l := range got {
		seen[l.RequestID] = true
	}
	if len(seen) != total {
		t.Fatalf("expected %d unique request IDs, got %d", total, len(seen))
	}
	if idx := s.VerifyIntegrity(); idx != -1 {
		t.Fatalf("in-memory hash chain broken at %d", idx)
	}

	reloaded := &Service{filePath: s.filePath}
	reloaded.loadFromDisk()
	if len(reloaded.logs) != total {
		t.Fatalf("expected %d logs after reload, got %d", total, len(reloaded.logs))
	}
	if idx := reloaded.VerifyIntegrity(); idx != -1 {
		t.Fatalf("reloaded hash chain broken at %d", idx)
	}
}

// TestLogPersistenceBoundedByMaxEntries 验证文件条目数与内存条目数都有上限：
// 追加写入后文件最多保留约 2*maxLogs 条，重新加载只恢复最新 maxLogs 条。
func TestLogPersistenceBoundedByMaxEntries(t *testing.T) {
	s := newTestService(t)

	total := maxLogs*2 + 25
	for i := 0; i < total; i++ {
		_ = s.RecordLog(RequestLog{RequestID: fmt.Sprintf("r%d", i)})
	}

	if len(s.logs) != maxLogs {
		t.Fatalf("in-memory logs should be capped at %d, got %d", maxLogs, len(s.logs))
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	lines := 0
	for _, raw := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) > 0 {
			lines++
		}
	}
	if lines > maxLogs*2 {
		t.Fatalf("log file should stay bounded (<= %d lines), got %d lines", maxLogs*2, lines)
	}
	if lines < maxLogs {
		t.Fatalf("log file should keep at least %d recent entries, got %d lines", maxLogs, lines)
	}

	reloaded := &Service{filePath: s.filePath}
	reloaded.loadFromDisk()
	if len(reloaded.logs) != maxLogs {
		t.Fatalf("reload should keep %d entries, got %d", maxLogs, len(reloaded.logs))
	}
	if got, want := reloaded.logs[0].RequestID, fmt.Sprintf("r%d", total-1); got != want {
		t.Fatalf("newest reloaded log = %q, want %q", got, want)
	}
	if idx := reloaded.VerifyIntegrity(); idx != -1 {
		t.Fatalf("reloaded hash chain broken at %d", idx)
	}
}

// TestListLogsReturnsLatestN 验证读取接口按「最新在前」返回最近 N 条。
func TestListLogsReturnsLatestN(t *testing.T) {
	s := newTestService(t)
	for i := 0; i < 5; i++ {
		_ = s.RecordLog(RequestLog{RequestID: fmt.Sprintf("r%d", i)})
	}

	got, err := s.ListLogs(2, 0)
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(got))
	}
	if got[0].RequestID != "r4" || got[1].RequestID != "r3" {
		t.Fatalf("expected latest two logs r4,r3, got %q,%q", got[0].RequestID, got[1].RequestID)
	}
}
