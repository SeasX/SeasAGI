package routing

import (
	"sort"
	"strconv"
	"sync"
)

// fillFirstState tracks the current "active" channel per routing group.
// In FillFirst mode, we stick to one channel until it's exhausted/unavailable,
// then move to the next. This is useful for rolling-window quota management
// (e.g., Pro accounts with message limits).
type fillFirstState struct {
	mu      sync.Mutex
	active  map[string]string // key → active channel ID
	counts  map[string]int    // key → request count for current active channel
}

func newFillFirstState() *fillFirstState {
	return &fillFirstState{
		active: make(map[string]string),
		counts: make(map[string]int),
	}
}

// reorderFillFirst puts the active channel first, then the rest by priority.
// If no active channel is set, picks the highest-priority channel as active.
func (f *fillFirstState) reorder(key string, candidates []PlanStep) []PlanStep {
	if len(candidates) <= 1 {
		return candidates
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Sort by priority (lower = higher priority)
	sorted := make([]PlanStep, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		pi := sorted[i].Channel.Priority
		pj := sorted[j].Channel.Priority
		if pi == 0 {
			pi = 100 // default low priority
		}
		if pj == 0 {
			pj = 100
		}
		return pi < pj
	})

	activeID, hasActive := f.active[key]
	if !hasActive {
		// Set first candidate as active
		f.active[key] = sorted[0].Channel.ChannelID
		f.counts[key] = 0
		activeID = sorted[0].Channel.ChannelID
	}

	// Find the active channel and move it to front
	for i, s := range sorted {
		if s.Channel.ChannelID == activeID {
			if i == 0 {
				return sorted
			}
			ordered := make([]PlanStep, 0, len(sorted))
			ordered = append(ordered, sorted[i])
			ordered = append(ordered, sorted[:i]...)
			ordered = append(ordered, sorted[i+1:]...)
			return ordered
		}
	}

	// Active channel not found (removed/disabled), pick first as new active
	f.active[key] = sorted[0].Channel.ChannelID
	f.counts[key] = 0
	return sorted
}

// advanceActive moves to the next channel in priority order.
// Called when the active channel hits a hard failure (429/5xx) and cooldown kicks in.
func (f *fillFirstState) advanceActive(key string, candidates []PlanStep) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(candidates) == 0 {
		delete(f.active, key)
		delete(f.counts, key)
		return
	}

	activeID, ok := f.active[key]
	if !ok {
		f.active[key] = candidates[0].Channel.ChannelID
		f.counts[key] = 0
		return
	}

	// Find current active index and move to next
	for i, c := range candidates {
		if c.Channel.ChannelID == activeID {
			next := (i + 1) % len(candidates)
			f.active[key] = candidates[next].Channel.ChannelID
			f.counts[key] = 0
			return
		}
	}

	// Not found, reset to first
	f.active[key] = candidates[0].Channel.ChannelID
	f.counts[key] = 0
}

// --- Priority bucket selection ---

// groupByPriority splits candidates into priority buckets.
// Lower priority value = higher priority. Default (0) is treated as priority 100.
// Returns ordered buckets (highest priority first).
func groupByPriority(candidates []PlanStep) [][]PlanStep {
	if len(candidates) <= 1 {
		return [][]PlanStep{candidates}
	}

	// Build priority → steps map
	bucketMap := make(map[int][]PlanStep)
	for _, s := range candidates {
		p := s.Channel.Priority
		if p == 0 {
			p = 100 // default
		}
		bucketMap[p] = append(bucketMap[p], s)
	}

	// Collect and sort priority levels
	levels := make([]int, 0, len(bucketMap))
	for p := range bucketMap {
		levels = append(levels, p)
	}
	sort.Ints(levels)

	// Build ordered buckets
	buckets := make([][]PlanStep, 0, len(levels))
	for _, p := range levels {
		buckets = append(buckets, bucketMap[p])
	}
	return buckets
}

// reorderWithPriorityBuckets groups candidates by priority, then applies
// WRR within each bucket. Buckets are concatenated highest-priority-first.
func (r *Resolver) reorderWithPriorityBuckets(key string, candidates []PlanStep, wrrState *smoothWRRState) []PlanStep {
	if len(candidates) <= 1 {
		return candidates
	}

	buckets := groupByPriority(candidates)
	if len(buckets) <= 1 {
		// All same priority — just WRR
		if wrrState != nil {
			return wrrState.reorderForWRR(key, candidates)
		}
		return candidates
	}

	// Within each bucket, apply WRR; concatenate buckets in priority order
	result := make([]PlanStep, 0, len(candidates))
	for bucketIdx, bucket := range buckets {
		bucketKey := key + "::p" + strconv.Itoa(bucketIdx)
		if len(bucket) > 1 && wrrState != nil {
			bucket = wrrState.reorderForWRR(bucketKey, bucket)
		}
		result = append(result, bucket...)
	}
	return result
}
