package routing

import (
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

func TestDegradeSlowChannelsReorders(t *testing.T) {
	fast := PlanStep{Channel: config.Channel{ChannelID: "t-fast"}, UpstreamModel: "fast"}
	slow := PlanStep{Channel: config.Channel{ChannelID: "t-slow"}, UpstreamModel: "slow"}

	// 给 slow 渠道记录一条超过阈值的延迟样本
	RecordLatency("t-slow", AutoDegradeThreshold+500)

	ordered := degradeSlowChannels([]PlanStep{slow, fast})
	if len(ordered) != 2 {
		t.Fatalf("len = %d, want 2", len(ordered))
	}
	if ordered[0].Channel.ChannelID != "t-fast" || ordered[1].Channel.ChannelID != "t-slow" {
		t.Fatalf("order = [%s, %s], want fast then slow",
			ordered[0].Channel.ChannelID, ordered[1].Channel.ChannelID)
	}
}

func TestDegradeSlowChannelsSkipsWhenSingle(t *testing.T) {
	step := PlanStep{Channel: config.Channel{ChannelID: "t-single"}, UpstreamModel: "one"}
	ordered := degradeSlowChannels([]PlanStep{step})
	if len(ordered) != 1 || ordered[0].Channel.ChannelID != "t-single" {
		t.Fatalf("single step should be returned unchanged, got %+v", ordered)
	}

	if got := degradeSlowChannels(nil); got != nil {
		t.Fatalf("nil input should return nil, got %+v", got)
	}
}

func TestDegradeSlowChannelsNoSamplesKeepsOrder(t *testing.T) {
	a := PlanStep{Channel: config.Channel{ChannelID: "t-none-a"}, UpstreamModel: "a"}
	b := PlanStep{Channel: config.Channel{ChannelID: "t-none-b"}, UpstreamModel: "b"}
	ordered := degradeSlowChannels([]PlanStep{a, b})
	if ordered[0].UpstreamModel != "a" || ordered[1].UpstreamModel != "b" {
		t.Fatalf("order without samples should be preserved, got [%s, %s]",
			ordered[0].UpstreamModel, ordered[1].UpstreamModel)
	}
}
