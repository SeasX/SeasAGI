package routing

import (
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

func wrrSteps(pairs ...[2]any) []PlanStep {
	steps := make([]PlanStep, 0, len(pairs))
	for _, p := range pairs {
		id := p[0].(string)
		weight := p[1].(int)
		steps = append(steps, PlanStep{
			Channel:       config.Channel{ChannelID: id, Weight: weight},
			UpstreamModel: id,
		})
	}
	return steps
}

func TestSmoothWRRSelectDistribution(t *testing.T) {
	s := newSmoothWRRState()
	steps := wrrSteps([2]any{"a", 5}, [2]any{"b", 1})
	s.initGroup("g", steps)

	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		step, _, ok := s.selectNext("g")
		if !ok {
			t.Fatal("selectNext returned false")
		}
		counts[step.Channel.ChannelID]++
	}
	// 权重 5:1，6 次选择应为 a=5, b=1
	if counts["a"] != 5 || counts["b"] != 1 {
		t.Fatalf("distribution = %v, want a=5 b=1", counts)
	}
}

func TestSmoothWRRReorderForWRR(t *testing.T) {
	s := newSmoothWRRState()
	steps := wrrSteps([2]any{"heavy", 5}, [2]any{"light", 1})

	ordered := s.reorderForWRR("g", steps)
	if len(ordered) != 2 {
		t.Fatalf("len = %d, want 2", len(ordered))
	}
	if ordered[0].Channel.ChannelID != "heavy" {
		t.Fatalf("heaviest weight should be first, got %s", ordered[0].Channel.ChannelID)
	}
}

func TestSmoothWRRInitGroupUnchangedIsNoop(t *testing.T) {
	s := newSmoothWRRState()
	steps := wrrSteps([2]any{"a", 2}, [2]any{"b", 2})
	s.initGroup("g", steps)

	// 推进一次状态
	_, _, _ = s.selectNext("g")

	s.mu.Lock()
	before := s.items["g"][0].currentWeight
	s.mu.Unlock()

	// 相同候选集再次 initGroup 不应重建（currentWeight 保留）
	s.initGroup("g", steps)
	s.mu.Lock()
	after := s.items["g"][0].currentWeight
	s.mu.Unlock()
	if before != after {
		t.Fatalf("initGroup rebuilt state for unchanged candidates: before=%d after=%d", before, after)
	}

	// 权重变化应被就地更新
	updated := wrrSteps([2]any{"a", 9}, [2]any{"b", 2})
	s.initGroup("g", updated)
	s.mu.Lock()
	w := s.items["g"][0].weight
	s.mu.Unlock()
	if w != 9 {
		t.Fatalf("weight not updated in place, got %d", w)
	}
}

func TestSmoothWRRSelectUnknownGroup(t *testing.T) {
	s := newSmoothWRRState()
	if _, _, ok := s.selectNext("missing"); ok {
		t.Fatal("selectNext on unknown group should return false")
	}
}

func TestSmoothWRRReorderSingleCandidate(t *testing.T) {
	s := newSmoothWRRState()
	steps := wrrSteps([2]any{"only", 1})
	ordered := s.reorderForWRR("g", steps)
	if len(ordered) != 1 || ordered[0].Channel.ChannelID != "only" {
		t.Fatalf("single candidate should pass through, got %+v", ordered)
	}
}
