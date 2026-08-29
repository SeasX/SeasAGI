package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mockExecutor 返回预设输出的模拟执行器。
func mockExecutor(output string) PromptExecutor {
	return func(prompt string, timeout int) (string, error) {
		return output, nil
	}
}

func TestEvalSuiteSave(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	suite := &EvalSuite{
		ID:          "suite-1",
		Name:        "基础测试套件",
		Description: "测试基础能力",
		TargetType:  TargetModel,
		TargetRef:   "gpt-4",
		Cases: []EvalCase{
			{ID: "c1", Name: "问候", Prompt: "你好", Expected: "你好", Match: MatchContains},
		},
		CreatedAt: nowFormatted(),
		UpdatedAt: nowFormatted(),
	}

	if err := store.SaveSuite(suite); err != nil {
		t.Fatalf("保存 suite 失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(filepath.Join(tmpDir, "suite-suite-1.json")); err != nil {
		t.Fatal("suite 文件应存在")
	}

	// 加载并验证
	loaded, err := store.LoadSuite("suite-1")
	if err != nil {
		t.Fatalf("加载 suite 失败: %v", err)
	}
	if loaded.Name != suite.Name {
		t.Fatal("加载的 suite 名称不匹配")
	}
	if len(loaded.Cases) != 1 {
		t.Fatal("应有 1 个用例")
	}
}

func TestEvalRunSave(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	suite := &EvalSuite{
		ID:         "suite-run-1",
		Name:       "运行测试",
		TargetType: TargetModel,
		TargetRef:  "gpt-4",
		Cases: []EvalCase{
			{ID: "c1", Name: "case1", Prompt: "hi", Expected: "hello", Match: MatchContains},
		},
	}

	runner := NewRunner(mockExecutor("hello world"))
	run, err := runner.RunSuite(suite)
	if err != nil {
		t.Fatalf("运行 suite 失败: %v", err)
	}

	if err := store.SaveRun(run); err != nil {
		t.Fatalf("保存 run 失败: %v", err)
	}

	// 加载并验证
	loaded, err := store.LoadRun(run.ID)
	if err != nil {
		t.Fatalf("加载 run 失败: %v", err)
	}
	if loaded.SuiteID != suite.ID {
		t.Fatal("加载的 run suite ID 不匹配")
	}
	if loaded.Total != 1 {
		t.Fatal("应有 1 个总用例")
	}
	if loaded.Passed != 1 {
		t.Fatal("应有 1 个通过用例")
	}
}

func TestEvalMatchContains(t *testing.T) {
	suite := &EvalSuite{
		ID:         "suite-contains",
		Name:       "Contains 匹配测试",
		TargetType: TargetModel,
		TargetRef:  "gpt-4",
		Cases: []EvalCase{
			{ID: "c1", Name: "包含测试", Prompt: "讲个笑话", Expected: "笑话", Match: MatchContains},
			{ID: "c2", Name: "不包含测试", Prompt: "讲个笑话", Expected: "严肃", Match: MatchContains},
		},
	}

	runner := NewRunner(mockExecutor("这是一个笑话，很有趣"))
	run, err := runner.RunSuite(suite)
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}

	if run.Passed != 1 {
		t.Fatalf("应有 1 个通过，实际 %d", run.Passed)
	}
	if run.Failed != 1 {
		t.Fatalf("应有 1 个失败，实际 %d", run.Failed)
	}
}

func TestEvalMatchExact(t *testing.T) {
	suite := &EvalSuite{
		ID:         "suite-exact",
		Name:       "Exact 匹配测试",
		TargetType: TargetModel,
		TargetRef:  "gpt-4",
		Cases: []EvalCase{
			{ID: "c1", Name: "精确匹配", Prompt: "回答是", Expected: "yes", Match: MatchExact},
			{ID: "c2", Name: "带空格精确匹配", Prompt: "回答是", Expected: "yes", Match: MatchExact},
		},
	}

	runner := NewRunner(mockExecutor("yes"))
	run, err := runner.RunSuite(suite)
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}

	if run.Passed != 2 {
		t.Fatalf("两个都应通过，实际 %d", run.Passed)
	}

	// 测试不匹配
	runner2 := NewRunner(mockExecutor("yes no"))
	run2, _ := runner2.RunSuite(suite)
	if run2.Failed != 2 {
		t.Fatalf("两个都应失败，实际 %d", run2.Failed)
	}
}

func TestEvalMatchRegex(t *testing.T) {
	suite := &EvalSuite{
		ID:         "suite-regex",
		Name:       "Regex 匹配测试",
		TargetType: TargetModel,
		TargetRef:  "gpt-4",
		Cases: []EvalCase{
			{ID: "c1", Name: "邮箱正则", Prompt: "给邮箱", Expected: `[a-z]+@[a-z]+\.[a-z]+`, Match: MatchRegex},
			{ID: "c2", Name: "数字正则", Prompt: "给数字", Expected: `^\d+$`, Match: MatchRegex},
			{ID: "c3", Name: "非法正则", Prompt: "测试", Expected: `[`, Match: MatchRegex},
		},
	}

	runner := NewRunner(mockExecutor("test@example.com"))
	run, err := runner.RunSuite(suite)
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}

	// c1 应通过（邮箱匹配）
	// c2 应失败（不是纯数字）
	// c3 应失败（非法正则）
	if run.Passed != 1 {
		t.Fatalf("应有 1 个通过，实际 %d", run.Passed)
	}
	if run.Failed != 2 {
		t.Fatalf("应有 2 个失败，实际 %d", run.Failed)
	}
}

func TestEvalScorecard(t *testing.T) {
	suite := &EvalSuite{
		ID:         "suite-scorecard",
		Name:       "记分卡测试",
		TargetType: TargetCombo,
		TargetRef:  "combo-1",
		Cases: []EvalCase{
			{ID: "c1", Name: "case1", Prompt: "p1", Expected: "pass", Match: MatchContains},
			{ID: "c2", Name: "case2", Prompt: "p2", Expected: "pass", Match: MatchContains},
			{ID: "c3", Name: "case3", Prompt: "p3", Expected: "fail", Match: MatchContains},
		},
	}

	runner := NewRunner(mockExecutor("this is a pass result"))
	run, _ := runner.RunSuite(suite)

	scorecard := GenerateScorecard(run)
	if scorecard.SuiteID != suite.ID {
		t.Fatal("记分卡 suite ID 不匹配")
	}
	if scorecard.Total != 3 {
		t.Fatalf("总计应为 3，实际 %d", scorecard.Total)
	}
	if scorecard.Passed != 2 {
		t.Fatalf("通过应为 2，实际 %d", scorecard.Passed)
	}
	if scorecard.Failed != 1 {
		t.Fatalf("失败应为 1，实际 %d", scorecard.Failed)
	}
	// pass rate 应约为 66.67%
	if scorecard.PassRate < 66.0 || scorecard.PassRate > 67.0 {
		t.Fatalf("pass rate 应约为 66.67，实际 %.2f", scorecard.PassRate)
	}

	// 测试汇总
	summary := GenerateSummary([]*PersistedEvalRun{run})
	if summary.TotalRuns != 1 {
		t.Fatalf("总运行数应为 1，实际 %d", summary.TotalRuns)
	}
	if len(summary.PerSuite) != 1 {
		t.Fatalf("per suite 应有 1 个，实际 %d", len(summary.PerSuite))
	}
}

func TestEvalListForRouting(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// 创建多个不同 target 的运行记录
	suite1 := &EvalSuite{
		ID:         "suite-route-1",
		Name:       "模型A测试",
		TargetType: TargetModel,
		TargetRef:  "model-a",
		Cases:      []EvalCase{{ID: "c1", Name: "c", Prompt: "p", Expected: "ok", Match: MatchContains}},
	}
	suite2 := &EvalSuite{
		ID:         "suite-route-2",
		Name:       "模型B测试",
		TargetType: TargetModel,
		TargetRef:  "model-b",
		Cases:      []EvalCase{{ID: "c1", Name: "c", Prompt: "p", Expected: "ok", Match: MatchContains}},
	}

	runner := NewRunner(mockExecutor("ok"))

	run1, _ := runner.RunSuite(suite1)
	run2, _ := runner.RunSuite(suite2)

	store.SaveRun(run1)
	store.SaveRun(run2)

	// 按 target 查询
	runsA, err := store.ListRunsByTarget(TargetModel, "model-a")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(runsA) != 1 {
		t.Fatalf("model-a 应有 1 条记录，实际 %d", len(runsA))
	}

	// 获取最新运行记录（供路由决策参考）
	latest, err := store.GetLatestRunForTarget(TargetModel, "model-a")
	if err != nil {
		t.Fatalf("获取最新记录失败: %v", err)
	}
	if latest == nil {
		t.Fatal("应能获取到最新记录")
	}
	if latest.TargetRef != "model-a" {
		t.Fatal("最新记录 target 不匹配")
	}

	// 按 suite 查询
	runsSuite1, err := store.ListRunsBySuite("suite-route-1")
	if err != nil {
		t.Fatalf("按 suite 查询失败: %v", err)
	}
	if len(runsSuite1) != 1 {
		t.Fatalf("suite-route-1 应有 1 条记录，实际 %d", len(runsSuite1))
	}

	// 列出全部
	allRuns, err := store.ListRuns()
	if err != nil {
		t.Fatalf("列出全部失败: %v", err)
	}
	if len(allRuns) != 2 {
		t.Fatalf("应有 2 条记录，实际 %d", len(allRuns))
	}
}

func TestEvalMatchCustom(t *testing.T) {
	suite := &EvalSuite{
		ID:         "suite-custom",
		Name:       "Custom 匹配测试",
		TargetType: TargetModel,
		TargetRef:  "gpt-4",
		Cases: []EvalCase{
			{
				ID:       "c1",
				Name:     "自定义匹配",
				Prompt:   "测试",
				Expected: "",
				Match:    MatchCustom,
				MatchFunc: func(output string) bool {
					return len(output) > 5
				},
			},
			{
				ID:       "c2",
				Name:     "无匹配函数",
				Prompt:   "测试",
				Expected: "",
				Match:    MatchCustom,
				MatchFunc: nil,
			},
		},
	}

	runner := NewRunner(mockExecutor("这是一个比较长的输出"))
	run, err := runner.RunSuite(suite)
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}

	// c1 应通过（输出长度 > 5）
	// c2 应失败（无匹配函数）
	if run.Passed != 1 {
		t.Fatalf("应有 1 个通过，实际 %d", run.Passed)
	}
	if run.Failed != 1 {
		t.Fatalf("应有 1 个失败，实际 %d", run.Failed)
	}
}

func TestEvalExecutorError(t *testing.T) {
	suite := &EvalSuite{
		ID:         "suite-error",
		Name:       "错误测试",
		TargetType: TargetModel,
		TargetRef:  "gpt-4",
		Cases: []EvalCase{
			{ID: "c1", Name: "错误用例", Prompt: "p", Expected: "ok", Match: MatchContains},
		},
	}

	errorExecutor := func(prompt string, timeout int) (string, error) {
		return "", fmt.Errorf("模拟执行错误")
	}

	runner := NewRunner(errorExecutor)
	run, err := runner.RunSuite(suite)
	if err != nil {
		t.Fatalf("运行不应返回错误: %v", err)
	}

	if run.Errors != 1 {
		t.Fatalf("应有 1 个错误，实际 %d", run.Errors)
	}
	if run.Passed != 0 {
		t.Fatalf("不应有通过，实际 %d", run.Passed)
	}
}
