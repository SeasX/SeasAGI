package logs

import "testing"

func TestGetIntentScenarioStats(t *testing.T) {
	s := &Service{logs: []RequestLog{
		{IntentScenario: "code_logic"},
		{IntentScenario: "code_logic"},
		{IntentScenario: "image_gen"},
		{}, // 无场景的旧日志应被跳过
	}}
	stats := s.GetIntentScenarioStats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(stats))
	}
	if stats[0]["scenario"] != "code_logic" || stats[0]["count"].(int) != 2 {
		t.Fatalf("top scenario should be code_logic x2, got %v", stats[0])
	}
	if got := stats[1]["count"].(int); got != 1 {
		t.Fatalf("image_gen count = %d, want 1", got)
	}
	if share := stats[0]["share"].(float64); share != 2.0/3.0 {
		t.Fatalf("share = %v, want %v", share, 2.0/3.0)
	}
}
