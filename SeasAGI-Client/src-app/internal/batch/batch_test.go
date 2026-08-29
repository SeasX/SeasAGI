package batch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBatchCreate(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	record, err := store.CreateBatch(
		"/v1/chat/completions",
		"24h",
		"file-input-1",
		"key-1",
		"gpt-4",
		map[string]interface{}{"source": "test"},
	)
	if err != nil {
		t.Fatalf("创建 batch 失败: %v", err)
	}

	if record.ID == "" {
		t.Fatal("batch ID 不应为空")
	}
	if record.Status != StatusValidating {
		t.Fatalf("初始状态应为 validating，实际 %s", record.Status)
	}
	if record.Endpoint != "/v1/chat/completions" {
		t.Fatal("endpoint 不匹配")
	}
	if record.InputFileID != "file-input-1" {
		t.Fatal("input file ID 不匹配")
	}

	// 验证文件存在
	if _, err := os.Stat(filepath.Join(tmpDir, "batch-"+record.ID+".json")); err != nil {
		t.Fatal("batch 文件应存在")
	}
}

func TestBatchStateTransition(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	record, _ := store.CreateBatch("/v1/chat", "24h", "f1", "k1", "gpt-4", nil)

	// validating → in_progress（合法）
	updated, err := store.UpdateBatchStatus(record.ID, StatusInProgress)
	if err != nil {
		t.Fatalf("validating → in_progress 应成功: %v", err)
	}
	if updated.Status != StatusInProgress {
		t.Fatal("状态应更新为 in_progress")
	}
	if updated.InProgressAt == 0 {
		t.Fatal("InProgressAt 应被设置")
	}

	// in_progress → finalizing（合法）
	_, err = store.UpdateBatchStatus(record.ID, StatusFinalizing)
	if err != nil {
		t.Fatalf("in_progress → finalizing 应成功: %v", err)
	}

	// finalizing → completed（合法）
	_, err = store.UpdateBatchStatus(record.ID, StatusCompleted)
	if err != nil {
		t.Fatalf("finalizing → completed 应成功: %v", err)
	}

	// completed → in_progress（非法，completed 是终态）
	_, err = store.UpdateBatchStatus(record.ID, StatusInProgress)
	if err == nil {
		t.Fatal("completed → in_progress 应失败")
	}

	// validating → completed（非法跳跃）
	record2, _ := store.CreateBatch("/v1/chat", "24h", "f2", "k2", "gpt-4", nil)
	_, err = store.UpdateBatchStatus(record2.ID, StatusCompleted)
	if err == nil {
		t.Fatal("validating → completed 应失败（非法跳跃）")
	}
}

func TestBatchCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	record, _ := store.CreateBatch("/v1/chat", "24h", "f1", "k1", "gpt-4", nil)

	// 创建检查点
	items := []struct {
		LineNumber int
		CustomID   string
	}{
		{LineNumber: 1, CustomID: "req-1"},
		{LineNumber: 2, CustomID: "req-2"},
		{LineNumber: 3, CustomID: "req-3"},
	}

	if err := store.EnsureCheckpoints(record.ID, items); err != nil {
		t.Fatalf("创建检查点失败: %v", err)
	}

	// 列出检查点
	all, err := store.ListCheckpoints(record.ID)
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("应有 3 个检查点，实际 %d", len(all))
	}

	// 标记 line 1 为处理中
	if err := store.MarkCheckpointProcessing(record.ID, 1, "req-1"); err != nil {
		t.Fatalf("标记处理中失败: %v", err)
	}

	// 标记 line 1 为已完成
	if err := store.MarkCheckpointResult(record.ID, 1, map[string]string{"output": "result-1"}); err != nil {
		t.Fatalf("标记完成失败: %v", err)
	}

	// 标记 line 2 为错误
	if err := store.MarkCheckpointError(record.ID, 2, "timeout"); err != nil {
		t.Fatalf("标记错误失败: %v", err)
	}

	// 验证检查点状态
	all, _ = store.ListCheckpoints(record.ID)
	if all[0].Status != CheckpointCompleted {
		t.Fatalf("line 1 应为 completed，实际 %s", all[0].Status)
	}
	if all[1].Status != CheckpointErrored {
		t.Fatalf("line 2 应为 errored，实际 %s", all[1].Status)
	}
	if all[2].Status != CheckpointPending {
		t.Fatalf("line 3 应为 pending，实际 %s", all[2].Status)
	}

	// 获取待处理检查点（断点续传）
	pending, err := store.GetPendingCheckpoints(record.ID)
	if err != nil {
		t.Fatalf("获取待处理检查点失败: %v", err)
	}
	// line 2 (errored) 不算 pending，line 3 (pending) 算
	// 但 errored 不在 pending/processing 中，所以只有 line 3
	if len(pending) != 1 {
		t.Fatalf("应有 1 个待处理检查点，实际 %d", len(pending))
	}
	if pending[0].LineNumber != 3 {
		t.Fatalf("待处理应为 line 3，实际 line %d", pending[0].LineNumber)
	}

	// EnsureCheckpoints 幂等性：重复调用不增加
	store.EnsureCheckpoints(record.ID, items)
	all, _ = store.ListCheckpoints(record.ID)
	if len(all) != 3 {
		t.Fatalf("重复 EnsureCheckpoints 后应仍为 3 个，实际 %d", len(all))
	}
}

func TestBatchCursorPagination(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// 创建 5 个 batch
	for i := 0; i < 5; i++ {
		store.CreateBatch("/v1/chat", "24h", "f", "key-1", "gpt-4", nil)
	}

	// 第一页，limit=2
	page1, err := store.ListBatches("key-1", 2, "")
	if err != nil {
		t.Fatalf("查询第一页失败: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("第一页应有 2 条，实际 %d", len(page1.Items))
	}
	if !page1.HasMore {
		t.Fatal("应有更多数据")
	}

	// 第二页，用第一页的 After 游标
	page2, err := store.ListBatches("key-1", 2, page1.After)
	if err != nil {
		t.Fatalf("查询第二页失败: %v", err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("第二页应有 2 条，实际 %d", len(page2.Items))
	}
	if !page2.HasMore {
		t.Fatal("应有更多数据")
	}

	// 第三页
	page3, err := store.ListBatches("key-1", 2, page2.After)
	if err != nil {
		t.Fatalf("查询第三页失败: %v", err)
	}
	if len(page3.Items) != 1 {
		t.Fatalf("第三页应有 1 条，实际 %d", len(page3.Items))
	}
	if page3.HasMore {
		t.Fatal("不应有更多数据")
	}

	// 按 apiKeyID 过滤
	store.CreateBatch("/v1/chat", "24h", "f", "key-2", "gpt-4", nil)
	pageOther, _ := store.ListBatches("key-2", 10, "")
	if len(pageOther.Items) != 1 {
		t.Fatalf("key-2 应有 1 条，实际 %d", len(pageOther.Items))
	}
}

func TestBatchDeleteCompleted(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// 创建 3 个 batch，2 个完成
	r1, _ := store.CreateBatch("/v1/chat", "24h", "f1", "k1", "gpt-4", nil)
	r2, _ := store.CreateBatch("/v1/chat", "24h", "f2", "k1", "gpt-4", nil)
	r3, _ := store.CreateBatch("/v1/chat", "24h", "f3", "k1", "gpt-4", nil)

	// r1 → completed
	store.UpdateBatchStatus(r1.ID, StatusInProgress)
	store.UpdateBatchStatus(r1.ID, StatusFinalizing)
	store.UpdateBatchStatus(r1.ID, StatusCompleted)

	// r2 → completed
	store.UpdateBatchStatus(r2.ID, StatusInProgress)
	store.UpdateBatchStatus(r2.ID, StatusFinalizing)
	store.UpdateBatchStatus(r2.ID, StatusCompleted)

	// r3 保持 validating

	// 删除已完成的
	deleted, err := store.DeleteCompletedBatches()
	if err != nil {
		t.Fatalf("批量清理失败: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("应删除 2 个，实际 %d", deleted)
	}

	// r3 应仍存在
	_, err = store.GetBatch(r3.ID)
	if err != nil {
		t.Fatal("r3 应仍存在")
	}

	// r1 应已被删除
	_, err = store.GetBatch(r1.ID)
	if err == nil {
		t.Fatal("r1 应已被删除")
	}
}
