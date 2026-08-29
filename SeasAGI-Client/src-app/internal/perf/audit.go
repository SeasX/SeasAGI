package perf

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Finding 性能审计发现。
type Finding struct {
	ID          string  `json:"id"`
	Category    string  `json:"category"` // "cold_start" / "db_query" / "memory" / "goroutine"
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Effort      int     `json:"effort"` // 1-5（5 为最费 effort）
	Impact      int     `json:"impact"` // 1-5（5 为最大影响）
	Score       float64 `json:"score"`  // effort × impact
}

// ColdStartResult 冷启动测量结果。
type ColdStartResult struct {
	TotalMs        int64         `json:"total_ms"`
	Phases         []PhaseTiming `json:"phases"`
	GoroutineCount int           `json:"goroutine_count"`
	MemAllocBytes  uint64        `json:"mem_alloc_bytes"`
}

// PhaseTiming 阶段计时。
type PhaseTiming struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
}

// DBQueryProfile DB 查询性能分析。
type DBQueryProfile struct {
	Query      string `json:"query"`
	DurationMs int64  `json:"duration_ms"`
	Rows       int    `json:"rows"`
	Slow       bool   `json:"slow"`
}

// Auditor 性能审计器。
type Auditor struct {
	mu              sync.Mutex
	coldStartPhases []PhaseTiming
	dbQueries       []DBQueryProfile
	findings        []Finding
}

// NewAuditor 创建审计器。
func NewAuditor() *Auditor {
	return &Auditor{
		coldStartPhases: make([]PhaseTiming, 0),
		dbQueries:       make([]DBQueryProfile, 0),
		findings:        make([]Finding, 0),
	}
}

// MeasureColdStart 测量冷启动时间。
// phaseFn 按顺序执行各阶段并记录耗时。
func (a *Auditor) MeasureColdStart(phaseFn func(recorder PhaseRecorder)) *ColdStartResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.coldStartPhases = make([]PhaseTiming, 0)
	start := time.Now()

	recorder := PhaseRecorder{auditor: a}
	phaseFn(recorder)

	totalMs := time.Since(start).Milliseconds()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &ColdStartResult{
		TotalMs:        totalMs,
		Phases:         a.coldStartPhases,
		GoroutineCount: runtime.NumGoroutine(),
		MemAllocBytes:  memStats.Alloc,
	}
}

// PhaseRecorder 阶段记录器。
type PhaseRecorder struct {
	auditor *Auditor
}

// Phase 记录一个阶段的耗时。
func (r PhaseRecorder) Phase(name string, fn func()) {
	start := time.Now()
	fn()
	r.auditor.coldStartPhases = append(r.auditor.coldStartPhases, PhaseTiming{
		Name:       name,
		DurationMs: time.Since(start).Milliseconds(),
	})
}

// RecordDBQuery 记录 DB 查询 profile。
func (a *Auditor) RecordDBQuery(query string, durationMs int64, rows int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	slow := durationMs > 100 // 超过 100ms 视为慢查询

	a.dbQueries = append(a.dbQueries, DBQueryProfile{
		Query:      query,
		DurationMs: durationMs,
		Rows:       rows,
		Slow:       slow,
	})

	if slow {
		a.findings = append(a.findings, Finding{
			ID:          fmt.Sprintf("slow-query-%d", len(a.findings)+1),
			Category:    "db_query",
			Title:       "Slow DB Query",
			Description: fmt.Sprintf("Query '%s' took %dms", query, durationMs),
			Effort:      2,
			Impact:      3,
			Score:       6,
		})
	}
}

// GetDBQueries 获取所有 DB 查询记录。
func (a *Auditor) GetDBQueries() []DBQueryProfile {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make([]DBQueryProfile, len(a.dbQueries))
	copy(result, a.dbQueries)
	return result
}

// AddFinding 添加发现。
func (a *Auditor) AddFinding(finding Finding) {
	a.mu.Lock()
	defer a.mu.Unlock()

	finding.Score = float64(finding.Effort) * float64(finding.Impact)
	a.findings = append(a.findings, finding)
}

// GetFindings 获取发现列表（按 Score 降序排序）。
func (a *Auditor) GetFindings() []Finding {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make([]Finding, len(a.findings))
	copy(result, a.findings)

	// 按 Score 降序排序（高影响低 effort 优先）
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	return result
}

// GenerateReport 生成审计报告。
func (a *Auditor) GenerateReport() *AuditReport {
	a.mu.Lock()
	defer a.mu.Unlock()

	findings := make([]Finding, len(a.findings))
	copy(findings, a.findings)
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Score > findings[j].Score
	})

	slowQueries := 0
	for _, q := range a.dbQueries {
		if q.Slow {
			slowQueries++
		}
	}

	return &AuditReport{
		TotalFindings:  len(a.findings),
		SlowQueries:    slowQueries,
		TotalDBQueries: len(a.dbQueries),
		Findings:       findings,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

// AuditReport 审计报告。
type AuditReport struct {
	TotalFindings  int       `json:"total_findings"`
	SlowQueries    int       `json:"slow_queries"`
	TotalDBQueries int       `json:"total_db_queries"`
	Findings       []Finding `json:"findings"`
	GeneratedAt    string    `json:"generated_at"`
}

// Reset 重置审计器。
func (a *Auditor) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.coldStartPhases = make([]PhaseTiming, 0)
	a.dbQueries = make([]DBQueryProfile, 0)
	a.findings = make([]Finding, 0)
}
