package eval

import "time"

// TargetType Eval 目标类型。
type TargetType string

const (
	// TargetSuiteDefault 在 suite-default 上运行（默认配置）
	TargetSuiteDefault TargetType = "suite-default"
	// TargetModel 在指定模型上运行
	TargetModel TargetType = "model"
	// TargetCombo 在指定 combo 上运行
	TargetCombo TargetType = "combo"
)

// MatchStrategy 匹配策略。
type MatchStrategy string

const (
	// MatchContains 输出包含期望字符串
	MatchContains MatchStrategy = "contains"
	// MatchExact 输出完全等于期望字符串
	MatchExact MatchStrategy = "exact"
	// MatchRegex 输出匹配正则表达式
	MatchRegex MatchStrategy = "regex"
	// MatchCustom 自定义匹配函数
	MatchCustom MatchStrategy = "custom"
)

// CaseStatus 单个用例状态。
type CaseStatus string

const (
	CasePassed CaseStatus = "passed"
	CaseFailed CaseStatus = "failed"
	CaseError  CaseStatus = "error"
	CaseSkipped CaseStatus = "skipped"
)

// EvalCase 单个测试用例。
type EvalCase struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Prompt      string                 `json:"prompt"`
	Expected    string                 `json:"expected"`
	Match       MatchStrategy          `json:"match"`
	MatchFunc   func(string) bool      `json:"-"`
	Timeout     int                    `json:"timeout,omitempty"` // 秒
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// EvalSuite 测试套件。
type EvalSuite struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	TargetType  TargetType `json:"target_type"`
	TargetRef   string     `json:"target_ref,omitempty"` // model id / combo id
	Cases       []EvalCase `json:"cases"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

// CaseResult 单个用例运行结果。
type CaseResult struct {
	CaseID    string     `json:"case_id"`
	CaseName  string     `json:"case_name"`
	Status    CaseStatus `json:"status"`
	Output    string     `json:"output"`
	LatencyMs int64      `json:"latency_ms"`
	Error     string     `json:"error,omitempty"`
}

// PersistedEvalRun 持久化的 Eval 运行记录。
type PersistedEvalRun struct {
	ID           string       `json:"id"`
	SuiteID      string       `json:"suite_id"`
	SuiteName    string       `json:"suite_name"`
	TargetType   TargetType   `json:"target_type"`
	TargetRef    string       `json:"target_ref"`
	Total        int          `json:"total"`
	Passed       int          `json:"passed"`
	Failed       int          `json:"failed"`
	Errors       int          `json:"errors"`
	Skipped      int          `json:"skipped"`
	PassRate     float64      `json:"pass_rate"`
	AvgLatencyMs int64        `json:"avg_latency_ms"`
	Results      []CaseResult `json:"results,omitempty"`
	CreatedAt    string       `json:"created_at"`
}

// EvalScorecard 记分卡。
type EvalScorecard struct {
	SuiteID      string  `json:"suite_id"`
	SuiteName    string  `json:"suite_name"`
	TargetType   TargetType `json:"target_type"`
	TargetRef    string  `json:"target_ref"`
	PassRate     float64 `json:"pass_rate"`
	Total        int     `json:"total"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Errors       int     `json:"errors"`
	Skipped      int     `json:"skipped"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	GeneratedAt  string  `json:"generated_at"`
}

// ScorecardSummary 汇总记分卡。
type ScorecardSummary struct {
	OverallPassRate float64         `json:"overall_pass_rate"`
	PerSuite        []EvalScorecard `json:"per_suite"`
	TotalRuns       int             `json:"total_runs"`
	GeneratedAt     string          `json:"generated_at"`
}

// nowFormatted 返回 RFC3339 格式时间戳。
func nowFormatted() string {
	return time.Now().UTC().Format(time.RFC3339)
}
