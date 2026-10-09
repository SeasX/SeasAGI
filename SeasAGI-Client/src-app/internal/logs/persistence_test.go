package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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
	for i := 0; i < persistBatch; i++ {
		_ = s.RecordLog(RequestLog{RequestID: "r" + string(rune('0'+i))})
	}

	if _, err := os.Stat(s.filePath); err != nil {
		t.Fatalf("log file should be persisted after %d records: %v", persistBatch, err)
	}

	reloaded := &Service{filePath: s.filePath}
	reloaded.loadFromDisk()
	if len(reloaded.logs) != persistBatch {
		t.Fatalf("reloaded %d logs, want %d", len(reloaded.logs), persistBatch)
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
	// 清空后应落盘为空
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		t.Fatalf("read cleared file: %v", err)
	}
	var loaded []RequestLog
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("cleared file should be valid JSON: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("cleared file should hold no logs, got %d", len(loaded))
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
