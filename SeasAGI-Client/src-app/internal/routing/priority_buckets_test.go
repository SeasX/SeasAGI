package routing

import (
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

func prioSteps(pairs ...[2]any) []PlanStep {
	steps := make([]PlanStep, 0, len(pairs))
	for _, p := range pairs {
		id := p[0].(string)
		priority := p[1].(int)
		steps = append(steps, PlanStep{
			Channel:       config.Channel{ChannelID: id, Priority: priority},
			UpstreamModel: id,
		})
	}
	return steps
}

func TestGroupByPriority(t *testing.T) {
	steps := prioSteps(
		[2]any{"p1a", 1},
		[2]any{"p2", 2},
		[2]any{"def", 0}, // 0 视作默认 100（最低优先级）
		[2]any{"p1b", 1},
	)

	buckets := groupByPriority(steps)
	if len(buckets) != 3 {
		t.Fatalf("bucket count = %d, want 3", len(buckets))
	}
	// 最高优先级（1）在前，其次 2，最后默认 100
	if len(buckets[0]) != 2 {
		t.Fatalf("first bucket size = %d, want 2 (priority 1)", len(buckets[0]))
	}
	if buckets[0][0].Channel.ChannelID != "p1a" || buckets[0][1].Channel.ChannelID != "p1b" {
		t.Fatalf("first bucket = %s,%s, want p1a,p1b", buckets[0][0].Channel.ChannelID, buckets[0][1].Channel.ChannelID)
	}
	if buckets[1][0].Channel.ChannelID != "p2" {
		t.Fatalf("second bucket = %s, want p2", buckets[1][0].Channel.ChannelID)
	}
	if buckets[2][0].Channel.ChannelID != "def" {
		t.Fatalf("third bucket = %s, want def", buckets[2][0].Channel.ChannelID)
	}
}

func TestGroupByPrioritySingleOrEmpty(t *testing.T) {
	single := prioSteps([2]any{"only", 5})
	buckets := groupByPriority(single)
	if len(buckets) != 1 || len(buckets[0]) != 1 {
		t.Fatalf("single candidate should produce one bucket, got %+v", buckets)
	}
}

func TestReorderWithPriorityBuckets(t *testing.T) {
	r := &Resolver{}
	wrr := newSmoothWRRState()

	steps := prioSteps(
		[2]any{"low", 100},
		[2]any{"high", 1},
	)

	ordered := r.reorderWithPriorityBuckets("g", steps, wrr)
	if len(ordered) != 2 {
		t.Fatalf("len = %d, want 2", len(ordered))
	}
	if ordered[0].Channel.ChannelID != "high" {
		t.Fatalf("highest priority channel should be first, got %s", ordered[0].Channel.ChannelID)
	}
	if ordered[1].Channel.ChannelID != "low" {
		t.Fatalf("lowest priority channel should be last, got %s", ordered[1].Channel.ChannelID)
	}
}

func TestReorderWithPriorityBucketsSamePriorityFallsBackToWRR(t *testing.T) {
	r := &Resolver{}
	wrr := newSmoothWRRState()

	steps := []PlanStep{
		{Channel: config.Channel{ChannelID: "a", Priority: 1, Weight: 1}, UpstreamModel: "a"},
		{Channel: config.Channel{ChannelID: "b", Priority: 1, Weight: 9}, UpstreamModel: "b"},
	}

	ordered := r.reorderWithPriorityBuckets("g", steps, wrr)
	if len(ordered) != 2 {
		t.Fatalf("len = %d, want 2", len(ordered))
	}
	// 同一优先级内按 WRR，权重更高的 b 应排前
	if ordered[0].Channel.ChannelID != "b" {
		t.Fatalf("expected heavier weight first within same bucket, got %s", ordered[0].Channel.ChannelID)
	}
}
