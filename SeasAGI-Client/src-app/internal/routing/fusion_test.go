package routing

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// mockFusionExecutor 模拟执行器
type mockFusionExecutor struct {
	mu        sync.Mutex
	calls     int32
	failModel string
	responses map[string]string
}

func (m *mockFusionExecutor) Execute(ctx context.Context, model string, channelID string, messages []map[string]any) (string, error) {
	atomic.AddInt32(&m.calls, 1)
	if model == m.failModel {
		return "", errors.New("simulated failure")
	}
	if resp, ok := m.responses[model]; ok {
		return resp, nil
	}
	return "response from " + model, nil
}

func TestFusionParallel(t *testing.T) {
	cfg := FusionConfig{
		PanelTargets: []FusionTarget{
			{Model: "model-a", ChannelID: "ch1"},
			{Model: "model-b", ChannelID: "ch2"},
			{Model: "model-c", ChannelID: "ch3"},
		},
	}
	exec := &mockFusionExecutor{}
	messages := []map[string]any{{"role": "user", "content": "test"}}

	result, err := ExecuteFusion(context.Background(), cfg, messages, exec)
	if err != nil {
		t.Fatalf("ExecuteFusion: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	if atomic.LoadInt32(&exec.calls) != 3 {
		t.Errorf("expected 3 calls, got %d", exec.calls)
	}
}

func TestFusionJudgeSynthesis(t *testing.T) {
	cfg := FusionConfig{
		PanelTargets: []FusionTarget{
			{Model: "model-a", ChannelID: "ch1"},
			{Model: "model-b", ChannelID: "ch2"},
		},
		JudgeModel:   "judge-model",
		JudgeChannel: "judge-ch",
	}
	exec := &mockFusionExecutor{
		responses: map[string]string{
			"judge-model": "synthesized answer",
		},
	}
	messages := []map[string]any{{"role": "user", "content": "test"}}

	result, err := ExecuteFusion(context.Background(), cfg, messages, exec)
	if err != nil {
		t.Fatalf("ExecuteFusion: %v", err)
	}
	if result != "synthesized answer" {
		t.Errorf("expected synthesized answer, got %s", result)
	}
}

func TestFusionPartialFailure(t *testing.T) {
	cfg := FusionConfig{
		PanelTargets: []FusionTarget{
			{Model: "model-a", ChannelID: "ch1"},
			{Model: "fail-model", ChannelID: "ch2"},
			{Model: "model-c", ChannelID: "ch3"},
		},
		JudgeModel: "judge-model",
	}
	exec := &mockFusionExecutor{
		failModel: "fail-model",
		responses: map[string]string{
			"judge-model": "synthesized from partial",
		},
	}
	messages := []map[string]any{{"role": "user", "content": "test"}}

	result, err := ExecuteFusion(context.Background(), cfg, messages, exec)
	if err != nil {
		t.Fatalf("ExecuteFusion: %v", err)
	}
	if result != "synthesized from partial" {
		t.Errorf("expected synthesized from partial, got %s", result)
	}
}

func TestFusionMaxParallel(t *testing.T) {
	cfg := FusionConfig{
		PanelTargets: []FusionTarget{
			{Model: "m1", ChannelID: "ch1"},
			{Model: "m2", ChannelID: "ch2"},
			{Model: "m3", ChannelID: "ch3"},
			{Model: "m4", ChannelID: "ch4"},
		},
		MaxParallel: 2,
	}
	exec := &mockFusionExecutor{}
	messages := []map[string]any{{"role": "user", "content": "test"}}

	_, err := ExecuteFusion(context.Background(), cfg, messages, exec)
	if err != nil {
		t.Fatalf("ExecuteFusion: %v", err)
	}
	if atomic.LoadInt32(&exec.calls) != 4 {
		t.Errorf("expected 4 calls, got %d", exec.calls)
	}
}

func TestFusionAllFail(t *testing.T) {
	cfg := FusionConfig{
		PanelTargets: []FusionTarget{
			{Model: "fail-1", ChannelID: "ch1"},
			{Model: "fail-2", ChannelID: "ch2"},
		},
	}
	exec2 := &allFailExecutor{}
	messages := []map[string]any{{"role": "user", "content": "test"}}

	_, err := ExecuteFusion(context.Background(), cfg, messages, exec2)
	if err == nil {
		t.Error("expected error when all targets fail")
	}
}

type allFailExecutor struct{}

func (e *allFailExecutor) Execute(ctx context.Context, model string, channelID string, messages []map[string]any) (string, error) {
	return "", errors.New("all fail")
}

func TestFusionNoTargets(t *testing.T) {
	cfg := FusionConfig{}
	exec := &mockFusionExecutor{}
	messages := []map[string]any{{"role": "user", "content": "test"}}

	_, err := ExecuteFusion(context.Background(), cfg, messages, exec)
	if err == nil {
		t.Error("expected error for no targets")
	}
}

func TestFusionSingleSuccessNoJudge(t *testing.T) {
	cfg := FusionConfig{
		PanelTargets: []FusionTarget{
			{Model: "only-model", ChannelID: "ch1"},
		},
	}
	exec := &mockFusionExecutor{
		responses: map[string]string{"only-model": "single response"},
	}
	messages := []map[string]any{{"role": "user", "content": "test"}}

	result, err := ExecuteFusion(context.Background(), cfg, messages, exec)
	if err != nil {
		t.Fatalf("ExecuteFusion: %v", err)
	}
	if result != "single response" {
		t.Errorf("expected 'single response', got %s", result)
	}
}

func TestFusionJudgeFailureFallback(t *testing.T) {
	cfg := FusionConfig{
		PanelTargets: []FusionTarget{
			{Model: "model-a", ChannelID: "ch1"},
		},
		JudgeModel: "fail-judge",
	}
	exec := &mockFusionExecutor{
		failModel: "fail-judge",
	}
	messages := []map[string]any{{"role": "user", "content": "test"}}

	result, err := ExecuteFusion(context.Background(), cfg, messages, exec)
	if err != nil {
		t.Fatalf("ExecuteFusion: %v", err)
	}
	// 判官失败时应返回拼接
	if result == "" {
		t.Error("expected non-empty fallback result")
	}
}
