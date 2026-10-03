package analytics

import "testing"

func TestRecordLocalEvent(t *testing.T) {
	if err := RecordLocalEvent("app_open"); err != nil {
		t.Fatalf("RecordLocalEvent returned error: %v", err)
	}
}

func TestReportEventsIfEnabled(t *testing.T) {
	if err := ReportEventsIfEnabled(); err != nil {
		t.Fatalf("ReportEventsIfEnabled returned error: %v", err)
	}
}

func TestEventFields(t *testing.T) {
	e := Event{EventType: "app_open", Timestamp: "2026-01-01T00:00:00Z", Version: "0.1.5"}
	if e.EventType == "" || e.Timestamp == "" || e.Version == "" {
		t.Fatalf("event fields must be populated: %+v", e)
	}
}
