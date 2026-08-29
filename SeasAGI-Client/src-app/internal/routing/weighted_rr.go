package routing

import (
	"sync"
)

// smoothWeightedItem is a single entry in the smooth WRR selection state.
type smoothWeightedItem struct {
	step          PlanStep
	weight        int
	currentWeight int
}

// smoothWRRState holds the persistent state for smooth weighted round-robin
// across multiple invocations. Keyed by the routing group name (e.g. model name).
type smoothWRRState struct {
	mu    sync.Mutex
	items map[string][]*smoothWeightedItem // key → ordered items
}

func newSmoothWRRState() *smoothWRRState {
	return &smoothWRRState{items: make(map[string][]*smoothWeightedItem)}
}

// initGroup ensures the group's item list is built from the given candidates.
// Weights default to 1 when Channel.Weight <= 0.
func (s *smoothWRRState) initGroup(key string, candidates []PlanStep) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.items[key]; ok {
		// Check if the candidate set has changed (same channels in same order)
		if len(existing) == len(candidates) {
			changed := false
			for i, c := range candidates {
				if existing[i].step.Channel.ChannelID != c.Channel.ChannelID ||
					existing[i].step.UpstreamModel != c.UpstreamModel {
					changed = true
					break
				}
				// Update weight in case it changed
				w := c.Channel.Weight
				if w <= 0 {
					w = 1
				}
				existing[i].weight = w
			}
			if !changed {
				return
			}
		}
	}

	// Rebuild
	items := make([]*smoothWeightedItem, 0, len(candidates))
	for _, c := range candidates {
		w := c.Channel.Weight
		if w <= 0 {
			w = 1
		}
		items = append(items, &smoothWeightedItem{
			step:          c,
			weight:        w,
			currentWeight: 0,
		})
	}
	s.items[key] = items
}

// selectNext performs one step of the smooth WRR algorithm (nginx-style).
// It returns the selected PlanStep and its index in the original slice.
//
// Algorithm:
//   1. For each item: currentWeight += weight
//   2. Find the item with the highest currentWeight
//   3. That item: currentWeight -= totalWeight
//   4. Return the selected item
//
// This produces a smooth interleaved distribution that avoids bursts.
func (s *smoothWRRState) selectNext(key string) (PlanStep, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, ok := s.items[key]
	if !ok || len(items) == 0 {
		return PlanStep{}, -1, false
	}

	totalWeight := 0
	bestIdx := 0
	for i, item := range items {
		item.currentWeight += item.weight
		totalWeight += item.weight
		if item.currentWeight > items[bestIdx].currentWeight {
			bestIdx = i
		}
	}

	items[bestIdx].currentWeight -= totalWeight
	return items[bestIdx].step, bestIdx, true
}

// reorderForWRR reorders candidates using smooth WRR for a single selection.
// Returns the candidates in the order they should be tried (best first).
func (s *smoothWRRState) reorderForWRR(key string, candidates []PlanStep) []PlanStep {
	if len(candidates) <= 1 {
		return candidates
	}

	s.initGroup(key, candidates)

	// Simulate len(candidates) selections to produce a full ordering
	ordered := make([]PlanStep, 0, len(candidates))
	used := make([]bool, len(candidates))

	// Clone the state for simulation so we don't advance the real state
	s.mu.Lock()
	items := s.items[key]
	s.mu.Unlock()

	// We need to pick each item exactly once, in smooth-WRR priority order
	for range candidates {
		// Find best among unused
		totalWeight := 0
		bestIdx := -1
		for i, item := range items {
			if used[i] {
				continue
			}
			// Use the current state + weight as the selection score
			score := item.currentWeight + item.weight
			if bestIdx == -1 || score > items[bestIdx].currentWeight+items[bestIdx].weight {
				bestIdx = i
			}
			totalWeight += item.weight
		}
		if bestIdx < 0 {
			break
		}
		used[bestIdx] = true
		ordered = append(ordered, items[bestIdx].step)
		// Advance the state
		items[bestIdx].currentWeight += items[bestIdx].weight
		items[bestIdx].currentWeight -= totalWeight
	}

	// Ensure all items are included even if something went wrong
	for i, item := range items {
		if !used[i] {
			ordered = append(ordered, item.step)
		}
	}

	return ordered
}
