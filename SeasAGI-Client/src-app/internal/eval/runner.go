package eval

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PromptExecutor 执行 prompt 的回调函数类型。
// 返回模型输出和可能的错误。
type PromptExecutor func(prompt string, timeout int) (string, error)

// Runner Eval 执行器。
type Runner struct {
	executor PromptExecutor
}

// NewRunner 创建执行器。
func NewRunner(executor PromptExecutor) *Runner {
	return &Runner{executor: executor}
}

// RunSuite 运行整个测试套件，返回持久化运行记录。
func (r *Runner) RunSuite(suite *EvalSuite) (*PersistedEvalRun, error) {
	if suite == nil {
		return nil, fmt.Errorf("suite 不能为 nil")
	}
	if len(suite.Cases) == 0 {
		return nil, fmt.Errorf("suite 没有测试用例")
	}

	results := make([]CaseResult, 0, len(suite.Cases))
	var totalLatency int64
	passed, failed, errors, skipped := 0, 0, 0, 0

	for _, c := range suite.Cases {
		result := r.runCase(&c)
		results = append(results, result)
		totalLatency += result.LatencyMs

		switch result.Status {
		case CasePassed:
			passed++
		case CaseFailed:
			failed++
		case CaseError:
			errors++
		case CaseSkipped:
			skipped++
		}
	}

	total := len(suite.Cases)
	var passRate float64
	if total > 0 {
		passRate = float64(passed) / float64(total) * 100
	}
	var avgLatency int64
	if total > 0 {
		avgLatency = totalLatency / int64(total)
	}

	run := &PersistedEvalRun{
		ID:           generateRunID(suite.ID),
		SuiteID:      suite.ID,
		SuiteName:    suite.Name,
		TargetType:   suite.TargetType,
		TargetRef:    suite.TargetRef,
		Total:        total,
		Passed:       passed,
		Failed:       failed,
		Errors:       errors,
		Skipped:      skipped,
		PassRate:     passRate,
		AvgLatencyMs: avgLatency,
		Results:      results,
		CreatedAt:    nowFormatted(),
	}

	return run, nil
}

// runCase 运行单个用例。
func (r *Runner) runCase(c *EvalCase) CaseResult {
	start := time.Now()

	if r.executor == nil {
		return CaseResult{
			CaseID:    c.ID,
			CaseName:  c.Name,
			Status:    CaseError,
			Error:     "executor 未设置",
			LatencyMs: 0,
		}
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	output, err := r.executor(c.Prompt, timeout)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CaseResult{
			CaseID:    c.ID,
			CaseName:  c.Name,
			Status:    CaseError,
			Output:    output,
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}

	matched := r.matchOutput(c, output)
	status := CaseFailed
	if matched {
		status = CasePassed
	}

	return CaseResult{
		CaseID:    c.ID,
		CaseName:  c.Name,
		Status:    status,
		Output:    output,
		LatencyMs: latency,
	}
}

// matchOutput 根据匹配策略检查输出。
func (r *Runner) matchOutput(c *EvalCase, output string) bool {
	switch c.Match {
	case MatchContains:
		return strings.Contains(output, c.Expected)

	case MatchExact:
		return strings.TrimSpace(output) == strings.TrimSpace(c.Expected)

	case MatchRegex:
		re, err := regexp.Compile(c.Expected)
		if err != nil {
			return false
		}
		return re.MatchString(output)

	case MatchCustom:
		if c.MatchFunc != nil {
			return c.MatchFunc(output)
		}
		return false

	default:
		return false
	}
}

// generateRunID 生成运行 ID。
func generateRunID(suiteID string) string {
	return fmt.Sprintf("%s-run-%d", suiteID, time.Now().UnixNano())
}

// GenerateScorecard 从运行记录生成记分卡。
func GenerateScorecard(run *PersistedEvalRun) EvalScorecard {
	return EvalScorecard{
		SuiteID:      run.SuiteID,
		SuiteName:    run.SuiteName,
		TargetType:   run.TargetType,
		TargetRef:    run.TargetRef,
		PassRate:     run.PassRate,
		Total:        run.Total,
		Passed:       run.Passed,
		Failed:       run.Failed,
		Errors:       run.Errors,
		Skipped:      run.Skipped,
		AvgLatencyMs: run.AvgLatencyMs,
		GeneratedAt:  nowFormatted(),
	}
}

// GenerateSummary 从多个运行记录生成汇总记分卡。
func GenerateSummary(runs []*PersistedEvalRun) ScorecardSummary {
	if len(runs) == 0 {
		return ScorecardSummary{
			GeneratedAt: nowFormatted(),
		}
	}

	var totalPassed, totalCases int
	scorecards := make([]EvalScorecard, 0, len(runs))

	for _, run := range runs {
		scorecards = append(scorecards, GenerateScorecard(run))
		totalPassed += run.Passed
		totalCases += run.Total
	}

	var overallRate float64
	if totalCases > 0 {
		overallRate = float64(totalPassed) / float64(totalCases) * 100
	}

	return ScorecardSummary{
		OverallPassRate: overallRate,
		PerSuite:        scorecards,
		TotalRuns:       len(runs),
		GeneratedAt:     nowFormatted(),
	}
}

// MatchContainsOutput 便捷函数：检查输出是否包含期望字符串。
func MatchContainsOutput(output, expected string) bool {
	return strings.Contains(output, expected)
}

// MatchExactOutput 便捷函数：检查输出是否完全匹配。
func MatchExactOutput(output, expected string) bool {
	return strings.TrimSpace(output) == strings.TrimSpace(expected)
}

// MatchRegexOutput 便捷函数：检查输出是否匹配正则。
func MatchRegexOutput(output, pattern string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(output)
}
