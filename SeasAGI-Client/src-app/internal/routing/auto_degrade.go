package routing

// AutoDegradeThreshold is the P95 latency threshold (in ms) above which a channel is deprioritized.
const AutoDegradeThreshold = 3000.0

// degradeSlowChannels reorders plan steps to push channels with P95 latency above threshold to the end.
func degradeSlowChannels(steps []PlanStep) []PlanStep {
	if len(steps) <= 1 {
		return steps
	}

	fast := make([]PlanStep, 0, len(steps))
	slow := make([]PlanStep, 0, len(steps))

	for _, step := range steps {
		p95 := GetP95Latency(step.Channel.ChannelID)
		if p95 > 0 && p95 >= AutoDegradeThreshold {
			slow = append(slow, step)
		} else {
			fast = append(fast, step)
		}
	}

	return append(fast, slow...)
}
