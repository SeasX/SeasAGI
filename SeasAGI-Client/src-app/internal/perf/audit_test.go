package perf

import (
	"testing"
	"time"
)

func TestPerfAuditColdStart(t *testing.T) {
	auditor := NewAuditor()
	defer auditor.Reset()

	result := auditor.MeasureColdStart(func(recorder PhaseRecorder) {
		recorder.Phase("init_db", func() {
			time.Sleep(10 * time.Millisecond)
		})
		recorder.Phase("load_config", func() {
			time.Sleep(5 * time.Millisecond)
		})
		recorder.Phase("start_server", func() {
			time.Sleep(3 * time.Millisecond)
		})
	})

	if result.TotalMs < 15 {
		t.Fatalf("总耗时应 >= 15ms，实际 %dms", result.TotalMs)
	}

	if len(result.Phases) != 3 {
		t.Fatalf("应有 3 个阶段，实际 %d", len(result.Phases))
	}

	// 验证阶段名称
	if result.Phases[0].Name != "init_db" {
		t.Fatal("第一阶段应为 init_db")
	}
	if result.Phases[1].Name != "load_config" {
		t.Fatal("第二阶段应为 load_config")
	}
	if result.Phases[2].Name != "start_server" {
		t.Fatal("第三阶段应为 start_server")
	}

	// init_db 耗时应最长
	if result.Phases[0].DurationMs < result.Phases[1].DurationMs {
		t.Fatal("init_db 耗时应 >= load_config")
	}

	// 应有 goroutine 和内存信息
	if result.GoroutineCount <= 0 {
		t.Fatal("goroutine 数量应 > 0")
	}
	if result.MemAllocBytes == 0 {
		t.Fatal("内存分配应 > 0")
	}
}

func TestPerfAuditDBQuery(t *testing.T) {
	auditor := NewAuditor()
	defer auditor.Reset()

	// 记录正常查询
	auditor.RecordDBQuery("SELECT * FROM combos", 5, 10)

	// 记录慢查询
	auditor.RecordDBQuery("SELECT * FROM usage_logs WHERE created_at > ?", 150, 1000)

	queries := auditor.GetDBQueries()
	if len(queries) != 2 {
		t.Fatalf("应有 2 条查询记录，实际 %d", len(queries))
	}

	// 第一条不是慢查询
	if queries[0].Slow {
		t.Fatal("第一条查询不应为慢查询")
	}

	// 第二条是慢查询
	if !queries[1].Slow {
		t.Fatal("第二条查询应为慢查询")
	}
	if queries[1].DurationMs != 150 {
		t.Fatalf("慢查询耗时应为 150ms，实际 %d", queries[1].DurationMs)
	}
}

func TestPerfAuditFindings(t *testing.T) {
	auditor := NewAuditor()
	defer auditor.Reset()

	// 添加多个发现
	auditor.AddFinding(Finding{
		ID:       "f1",
		Category: "cold_start",
		Title:    "Slow module import",
		Effort:   2,
		Impact:   5,
	})

	auditor.AddFinding(Finding{
		ID:       "f2",
		Category: "db_query",
		Title:    "Missing index",
		Effort:   1,
		Impact:   4,
	})

	auditor.AddFinding(Finding{
		ID:       "f3",
		Category: "memory",
		Title:    "Large allocation",
		Effort:   3,
		Impact:   2,
	})

	findings := auditor.GetFindings()
	if len(findings) != 3 {
		t.Fatalf("应有 3 个发现，实际 %d", len(findings))
	}

	// 按 Score 降序排序
	// f1: 2×5=10, f2: 1×4=4, f3: 3×2=6
	// 排序后: f1(10) > f3(6) > f2(4)
	if findings[0].ID != "f1" {
		t.Fatalf("第一个应为 f1（score=10），实际 %s", findings[0].ID)
	}
	if findings[1].ID != "f3" {
		t.Fatalf("第二个应为 f3（score=6），实际 %s", findings[1].ID)
	}
	if findings[2].ID != "f2" {
		t.Fatalf("第三个应为 f2（score=4），实际 %s", findings[2].ID)
	}
}

func TestPerfAuditReport(t *testing.T) {
	auditor := NewAuditor()
	defer auditor.Reset()

	// 记录一些数据
	auditor.RecordDBQuery("SELECT 1", 5, 1)
	auditor.RecordDBQuery("SELECT * FROM big_table", 200, 5000)
	auditor.AddFinding(Finding{
		ID:       "f1",
		Category: "db_query",
		Title:    "Slow query detected",
		Effort:   1,
		Impact:   5,
	})

	report := auditor.GenerateReport()

	// 慢查询自动生成 1 条 + 手动 1 条 = 2 条
	if report.TotalFindings != 2 {
		t.Fatalf("应有 2 个发现（1 手动 + 1 慢查询自动），实际 %d", report.TotalFindings)
	}

	if report.SlowQueries != 1 {
		t.Fatalf("应有 1 个慢查询，实际 %d", report.SlowQueries)
	}

	if report.TotalDBQueries != 2 {
		t.Fatalf("应有 2 条 DB 查询，实际 %d", report.TotalDBQueries)
	}

	if len(report.Findings) != 2 {
		t.Fatal("报告应包含所有发现")
	}
}

func TestPerfAuditSlowQueryGeneratesFinding(t *testing.T) {
	auditor := NewAuditor()
	defer auditor.Reset()

	// 慢查询应自动生成发现
	auditor.RecordDBQuery("SLOW QUERY", 300, 100)

	findings := auditor.GetFindings()
	if len(findings) != 1 {
		t.Fatalf("慢查询应生成 1 个发现，实际 %d", len(findings))
	}

	if findings[0].Category != "db_query" {
		t.Fatal("发现类别应为 db_query")
	}
}
