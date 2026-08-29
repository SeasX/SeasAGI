package routing

import (
	"context"
	"fmt"
	"sync"
)

// FusionConfig Fusion 路由配置
type FusionConfig struct {
	PanelTargets     []FusionTarget `json:"panel_targets"`     // 扇出目标列表
	JudgeModel       string         `json:"judge_model"`       // 判官模型
	JudgeChannel     string         `json:"judge_channel"`     // 判官渠道
	MaxParallel      int            `json:"max_parallel"`      // 最大并行数
	SynthesisPrompt  string         `json:"synthesis_prompt"`  // 合成提示词
}

// FusionTarget 扇出目标
type FusionTarget struct {
	Model     string `json:"model"`
	ChannelID string `json:"channel_id"`
}

// FusionExecutor 执行器接口（解耦，不依赖具体 provider 包）
type FusionExecutor interface {
	Execute(ctx context.Context, model string, channelID string, messages []map[string]any) (string, error)
}

// ExecuteFusion 并发扇出到多个模型，收集响应后由判官模型合成
func ExecuteFusion(ctx context.Context, cfg FusionConfig, messages []map[string]any, executor FusionExecutor) (string, error) {
	if len(cfg.PanelTargets) == 0 {
		return "", fmt.Errorf("no panel targets")
	}
	if executor == nil {
		return "", fmt.Errorf("no executor")
	}

	maxParallel := cfg.MaxParallel
	if maxParallel <= 0 {
		maxParallel = len(cfg.PanelTargets)
	}

	// 并发扇出
	type result struct {
		model    string
		response string
		err      error
	}

	results := make([]result, len(cfg.PanelTargets))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, target := range cfg.PanelTargets {
		wg.Add(1)
		go func(idx int, t FusionTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := executor.Execute(ctx, t.Model, t.ChannelID, messages)
			results[idx] = result{model: t.Model, response: resp, err: err}
		}(i, target)
	}
	wg.Wait()

	// 收集成功的响应
	var responses []string
	for _, r := range results {
		if r.err != nil {
			continue
		}
		responses = append(responses, fmt.Sprintf("[Model: %s]\n%s", r.model, r.response))
	}

	if len(responses) == 0 {
		return "", fmt.Errorf("all panel targets failed")
	}

	// 如果只有一个成功且没有判官模型，直接返回
	if len(responses) == 1 && cfg.JudgeModel == "" {
		return results[0].response, nil
	}

	// 判官合成
	if cfg.JudgeModel == "" {
		// 无判官时拼接返回
		combined := ""
		for _, r := range responses {
			combined += r + "\n\n"
		}
		return combined, nil
	}

	synthesisPrompt := cfg.SynthesisPrompt
	if synthesisPrompt == "" {
		synthesisPrompt = "You are a synthesis judge. Given the following responses from multiple AI models, synthesize a single comprehensive answer that combines the best elements of all responses:\n\n"
	}

	judgeMessages := []map[string]any{
		{"role": "system", "content": synthesisPrompt},
		{"role": "user", "content": joinResponses(responses)},
	}

	synthesized, err := executor.Execute(ctx, cfg.JudgeModel, cfg.JudgeChannel, judgeMessages)
	if err != nil {
		// 判官失败时返回拼接
		return joinResponses(responses), nil
	}
	return synthesized, nil
}

// joinResponses 拼接多个响应
func joinResponses(responses []string) string {
	result := ""
	for i, r := range responses {
		if i > 0 {
			result += "\n---\n"
		}
		result += r
	}
	return result
}
