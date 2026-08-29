package analytics

type Event struct {
	EventType string `json:"event_type"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

func RecordLocalEvent(eventType string) error {
	_ = eventType
	return nil
}

func ReportEventsIfEnabled() error {
	return nil
}
