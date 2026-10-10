package logs

import (
	"strings"
	"testing"
)

func TestVerifyIntegrityIntactAfterRecording(t *testing.T) {
	s := newTestService(t)
	for i := 0; i < 5; i++ {
		_ = s.RecordLog(RequestLog{RequestID: string(rune('a' + i)), Status: "success"})
	}
	if idx := s.VerifyIntegrity(); idx != -1 {
		t.Fatalf("chain should be intact, got broken at %d", idx)
	}
}

func TestVerifyIntegrityEmptyService(t *testing.T) {
	s := newTestService(t)
	if idx := s.VerifyIntegrity(); idx != -1 {
		t.Fatalf("empty service should be intact, got %d", idx)
	}
}

func TestVerifyIntegrityDetectsFieldTamper(t *testing.T) {
	s := newTestService(t)
	_ = s.RecordLog(RequestLog{RequestID: "r1", Status: "success"})
	_ = s.RecordLog(RequestLog{RequestID: "r2", Status: "failure"})

	// 直接篡改内存中最旧一条记录的字段（模拟落盘后被改动）。
	s.logs[1].Status = "failure"
	if idx := s.VerifyIntegrity(); idx != 1 {
		t.Fatalf("tampered record should be detected at index 1, got %d", idx)
	}
}

func TestVerifyIntegrityDetectsHashTamper(t *testing.T) {
	s := newTestService(t)
	_ = s.RecordLog(RequestLog{RequestID: "r1"})
	_ = s.RecordLog(RequestLog{RequestID: "r2"})

	s.logs[0].Hash = "deadbeef"
	if idx := s.VerifyIntegrity(); idx != 0 {
		t.Fatalf("tampered hash should be detected at index 0, got %d", idx)
	}
}

func TestVerifyIntegritySurvivesPersistence(t *testing.T) {
	s := newTestService(t)
	for i := 0; i < persistBatch; i++ {
		_ = s.RecordLog(RequestLog{RequestID: "r" + string(rune('0'+i))})
	}

	reloaded := &Service{filePath: s.filePath}
	reloaded.loadFromDisk()
	if idx := reloaded.VerifyIntegrity(); idx != -1 {
		t.Fatalf("reloaded chain should be intact, got broken at %d", idx)
	}
}

func TestBackfillHashesForLegacyRecords(t *testing.T) {
	// 构造旧格式（无哈希）数据，加载后应补算整链且校验通过。
	legacy := []RequestLog{
		{RequestID: "newest"},
		{RequestID: "older"},
	}
	backfillHashes(legacy)
	if idx := (&Service{logs: legacy}).VerifyIntegrity(); idx != -1 {
		t.Fatalf("backfilled legacy chain should verify, got %d", idx)
	}
	if legacy[0].PrevHash != legacy[1].Hash {
		t.Fatalf("backfill should link newest.PrevHash to older.Hash")
	}
	if !strings.HasPrefix(legacy[1].PrevHash, "0000") {
		t.Fatalf("oldest record should link back to genesis, got %q", legacy[1].PrevHash)
	}
}
