package trace

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

var testStore *TraceStore

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "trace-test-*")
	if err != nil {
		panic(err)
	}

	InitStore(tmpDir + "/test.db")
	testStore = GetStore()
	if testStore == nil {
		os.RemoveAll(tmpDir)
		panic("failed to init store")
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestInitStore(t *testing.T) {
	if testStore == nil {
		t.Fatal("expected store to be initialized")
	}
	// Verify table and indices exist by querying
	_, err := testStore.QueryRecent(1)
	if err != nil {
		t.Fatalf("expected no error querying recent: %v", err)
	}
}

func TestInsertAndQueryByTraceID(t *testing.T) {
	rec := Record{
		TraceID:    "test-insert-001",
		UserID:     "user-001",
		DeviceID:   "device-001",
		TenantID:   "tenant-001",
		ChannelID:  "channel-001",
		Model:      "gpt-4",
		Method:     "POST",
		Path:       "/v1/chat/completions",
		StatusCode: 200,
		LatencyMs:  150,
		ClientIP:   "192.168.1.1",
		CreatedAt:  time.Now().Format(time.RFC3339),
	}

	testStore.Insert(rec)

	got, err := testStore.QueryByTraceID("test-insert-001")
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if got == nil {
		t.Fatal("expected record, got nil")
	}

	// Verify all fields round-trip correctly
	cases := []struct {
		name string
		want interface{}
		got  interface{}
	}{
		{"TraceID", rec.TraceID, got.TraceID},
		{"UserID", rec.UserID, got.UserID},
		{"DeviceID", rec.DeviceID, got.DeviceID},
		{"TenantID", rec.TenantID, got.TenantID},
		{"ChannelID", rec.ChannelID, got.ChannelID},
		{"Model", rec.Model, got.Model},
		{"Method", rec.Method, got.Method},
		{"Path", rec.Path, got.Path},
		{"StatusCode", rec.StatusCode, got.StatusCode},
		{"LatencyMs", rec.LatencyMs, got.LatencyMs},
		{"ClientIP", rec.ClientIP, got.ClientIP},
	}
	for _, c := range cases {
		if c.want != c.got {
			t.Errorf("%s: expected %v, got %v", c.name, c.want, c.got)
		}
	}
	if got.CreatedAt == "" {
		t.Error("expected CreatedAt to be non-empty")
	}
}

func TestMultipleRecordsAndQueryRecent(t *testing.T) {
	count := 3
	for i := 0; i < count; i++ {
		rec := Record{
			TraceID:   fmt.Sprintf("test-multi-%d", i),
			UserID:    "user-multi",
			DeviceID:  "device-multi",
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		testStore.Insert(rec)
	}

	results, err := testStore.QueryRecent(100)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}

	found := 0
	for _, r := range results {
		if r.UserID == "user-multi" {
			found++
		}
	}
	if found < count {
		t.Errorf("expected at least %d records with user-multi, got %d", count, found)
	}
}

func TestQueryByUserID(t *testing.T) {
	userID := "test-user-query"
	count := 3
	for i := 0; i < count; i++ {
		rec := Record{
			TraceID:   fmt.Sprintf("test-user-%d", i),
			UserID:    userID,
			DeviceID:  "device-user",
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		testStore.Insert(rec)
	}

	results, err := testStore.QueryByUserID(userID, 100)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if len(results) != count {
		t.Errorf("expected %d records, got %d", count, len(results))
	}
	for _, r := range results {
		if r.UserID != userID {
			t.Errorf("expected UserID %s, got %s", userID, r.UserID)
		}
	}
}

func TestQueryByDeviceID(t *testing.T) {
	deviceID := "test-device-query"
	count := 3
	for i := 0; i < count; i++ {
		rec := Record{
			TraceID:   fmt.Sprintf("test-dev-%d", i),
			UserID:    "user-dev",
			DeviceID:  deviceID,
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		testStore.Insert(rec)
	}

	results, err := testStore.QueryByDeviceID(deviceID, 100)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if len(results) != count {
		t.Errorf("expected %d records, got %d", count, len(results))
	}
	for _, r := range results {
		if r.DeviceID != deviceID {
			t.Errorf("expected DeviceID %s, got %s", deviceID, r.DeviceID)
		}
	}
}

func TestQueryByTraceIDNotFound(t *testing.T) {
	_, err := testStore.QueryByTraceID("test-nonexistent-trace")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestPurgeBeforeRemovesOldRecords(t *testing.T) {
	oldTime := "2023-01-01T00:00:00Z"
	testStore.Insert(Record{
		TraceID:   "test-purge-old-001",
		UserID:    "user-purge-old",
		DeviceID:  "device-purge-old",
		CreatedAt: oldTime,
	})
	testStore.Insert(Record{
		TraceID:   "test-purge-old-002",
		UserID:    "user-purge-old",
		DeviceID:  "device-purge-old",
		CreatedAt: oldTime,
	})

	cutoff, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	n, err := testStore.PurgeBefore(cutoff)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if n < 2 {
		t.Errorf("expected at least 2 deleted rows, got %d", n)
	}

	// Verify old records are gone
	_, err = testStore.QueryByTraceID("test-purge-old-001")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows for purged record, got %v", err)
	}
	_, err = testStore.QueryByTraceID("test-purge-old-002")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows for purged record, got %v", err)
	}
}

func TestPurgeBeforeKeepsRecentRecords(t *testing.T) {
	recentTime := time.Now().Format(time.RFC3339)
	testStore.Insert(Record{
		TraceID:   "test-purge-recent-001",
		UserID:    "user-purge-recent",
		DeviceID:  "device-purge-recent",
		CreatedAt: recentTime,
	})

	// Purge records before 2024 — should not affect the recent record
	cutoff, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	_, err := testStore.PurgeBefore(cutoff)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}

	got, err := testStore.QueryByTraceID("test-purge-recent-001")
	if err != nil {
		t.Fatalf("expected recent record to survive purge, got error: %v", err)
	}
	if got == nil {
		t.Fatal("expected recent record to survive purge, got nil")
	}
}

func TestQueryRecentLimit(t *testing.T) {
	// Insert 5 records
	count := 5
	for i := 0; i < count; i++ {
		rec := Record{
			TraceID:   fmt.Sprintf("test-limit-%d", i),
			UserID:    "user-limit",
			DeviceID:  "device-limit",
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		testStore.Insert(rec)
	}

	// Query with limit 2 — should return at most 2
	limit := 2
	results, err := testStore.QueryRecent(limit)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if len(results) > limit {
		t.Errorf("expected at most %d records, got %d", limit, len(results))
	}
}

func TestConcurrentInserts(t *testing.T) {
	n := 10
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rec := Record{
				TraceID:   fmt.Sprintf("test-concurrent-%d", idx),
				UserID:    "user-concurrent",
				DeviceID:  "device-concurrent",
				CreatedAt: time.Now().Format(time.RFC3339),
			}
			testStore.Insert(rec)
		}(i)
	}
	wg.Wait()

	results, err := testStore.QueryByUserID("user-concurrent", 100)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if len(results) < n {
		t.Errorf("expected at least %d concurrent records, got %d", n, len(results))
	}
}