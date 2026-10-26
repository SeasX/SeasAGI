package batch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Store Batch 持久化存储（文件系统实现）。
//
// experimental：未接入生产路径；复用时需先补齐包注释中列出的跨进程写锁、
// 过期清理与磁盘配额，否则多实例并发写会互相覆盖。
type Store struct {
	mu      sync.RWMutex
	dataDir string
	// onSave 在检查点成功落盘后回调，仅用于测试统计落盘次数；生产为 nil。
	onSave func()
}

// NewStore 创建存储（experimental，未接入生产路径）。
func NewStore(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

func (s *Store) ensureDir() error {
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return err
	}
	// 既有目录可能是旧版本以 0755 创建，显式收紧权限。
	return os.Chmod(s.dataDir, 0o700)
}

func (s *Store) batchPath(id string) string {
	return filepath.Join(s.dataDir, fmt.Sprintf("batch-%s.json", id))
}

func (s *Store) checkpointPath(batchID string) string {
	return filepath.Join(s.dataDir, fmt.Sprintf("checkpoints-%s.json", batchID))
}

// CreateBatch 创建 Batch 记录。
func (s *Store) CreateBatch(endpoint, completionWindow, inputFileID, apiKeyID, model string, metadata map[string]interface{}) (*BatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDir(); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}

	id := generateBatchID()
	record := &BatchRecord{
		ID:               id,
		Endpoint:         endpoint,
		CompletionWindow: completionWindow,
		Status:           StatusValidating,
		InputFileID:      inputFileID,
		APIKeyID:         apiKeyID,
		Model:            model,
		Metadata:         metadata,
		CreatedAt:        nowUnix(),
	}

	if err := s.saveBatch(record); err != nil {
		return nil, err
	}

	return record, nil
}

// saveBatch 保存记录到文件。
func (s *Store) saveBatch(record *BatchRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}
	return os.WriteFile(s.batchPath(record.ID), data, 0o600)
}

// GetBatch 获取记录。
func (s *Store) GetBatch(id string) (*BatchRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.batchPath(id))
	if err != nil {
		return nil, err
	}
	// 读取时收紧既有文件权限（旧版本可能以 0644 写入）。
	_ = os.Chmod(s.batchPath(id), 0o600)

	var record BatchRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdateBatchStatus 更新状态（校验状态机合法性）。
func (s *Store) UpdateBatchStatus(id string, newStatus BatchStatus) (*BatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.getBatchUnlocked(id)
	if err != nil {
		return nil, err
	}

	if !IsValidTransition(record.Status, newStatus) {
		return nil, fmt.Errorf("非法状态转换: %s → %s", record.Status, newStatus)
	}

	record.Status = newStatus
	ts := nowUnix()
	switch newStatus {
	case StatusInProgress:
		record.InProgressAt = ts
	case StatusFinalizing:
		record.FinalizingAt = ts
	case StatusCompleted:
		record.CompletedAt = ts
	case StatusFailed:
		record.FailedAt = ts
	case StatusExpired:
		record.ExpiredAt = ts
	case StatusCancelling:
		record.CancellingAt = ts
	case StatusCancelled:
		record.CancelledAt = ts
	}

	if err := s.saveBatch(record); err != nil {
		return nil, err
	}
	return record, nil
}

// getBatchUnlocked 读取记录（不加锁）。
func (s *Store) getBatchUnlocked(id string) (*BatchRecord, error) {
	data, err := os.ReadFile(s.batchPath(id))
	if err != nil {
		return nil, err
	}
	// 读取时收紧既有文件权限（旧版本可能以 0644 写入）。
	_ = os.Chmod(s.batchPath(id), 0o600)
	var record BatchRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdateRequestCounts 更新请求计数。
func (s *Store) UpdateRequestCounts(id string, total, completed, failed int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.getBatchUnlocked(id)
	if err != nil {
		return err
	}

	record.RequestCountsTotal = total
	record.RequestCountsCompleted = completed
	record.RequestCountsFailed = failed

	return s.saveBatch(record)
}

// ListBatches 游标分页查询。
// 按 created_at DESC + id DESC 排序，after 为上一页最后一条的 ID。
func (s *Store) ListBatches(apiKeyID string, limit int, after string) (*CursorPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil, err
	}

	var records []BatchRecord
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isBatchFile(name) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dataDir, name))
		if err != nil {
			continue
		}

		var record BatchRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		if apiKeyID != "" && record.APIKeyID != apiKeyID {
			continue
		}

		records = append(records, record)
	}

	// 按 created_at DESC + id DESC 排序
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt != records[j].CreatedAt {
			return records[i].CreatedAt > records[j].CreatedAt
		}
		return records[i].ID > records[j].ID
	})

	// 游标分页
	startIdx := 0
	if after != "" {
		for i, r := range records {
			if r.ID == after {
				startIdx = i + 1
				break
			}
		}
	}

	if limit <= 0 {
		limit = 20
	}

	endIdx := startIdx + limit
	hasMore := false
	if endIdx < len(records) {
		hasMore = true
	}
	if endIdx > len(records) {
		endIdx = len(records)
	}

	page := &CursorPage{
		Items:   records[startIdx:endIdx],
		HasMore: hasMore,
	}

	if hasMore && len(page.Items) > 0 {
		page.After = page.Items[len(page.Items)-1].ID
	}

	return page, nil
}

// isBatchFile 检查文件名是否为 batch 文件。
func isBatchFile(name string) bool {
	return strings.HasPrefix(name, "batch-") && strings.HasSuffix(name, ".json")
}

// DeleteBatch 删除单个 Batch 及其检查点。
func (s *Store) DeleteBatch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 删除检查点文件
	cpPath := s.checkpointPath(id)
	os.Remove(cpPath)

	// 删除 batch 文件
	return os.Remove(s.batchPath(id))
}

// DeleteCompletedBatches 批量清理已完成的 Batch。
func (s *Store) DeleteCompletedBatches() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isBatchFile(name) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dataDir, name))
		if err != nil {
			continue
		}

		var record BatchRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		if record.Status == StatusCompleted {
			// 删除检查点
			os.Remove(s.checkpointPath(record.ID))
			// 删除 batch
			if err := os.Remove(filepath.Join(s.dataDir, name)); err == nil {
				deleted++
			}
		}
	}

	return deleted, nil
}

// --- 检查点机制 ---

// EnsureCheckpoints 批量创建检查点（INSERT OR IGNORE 语义）。
func (s *Store) EnsureCheckpoints(batchID string, items []struct {
	LineNumber int
	CustomID   string
}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	checkpoints, err := s.loadCheckpoints(batchID)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	now := nowUnix()
	existing := make(map[int]bool)
	for _, cp := range checkpoints {
		existing[cp.LineNumber] = true
	}

	for _, item := range items {
		if existing[item.LineNumber] {
			continue
		}
		checkpoints = append(checkpoints, BatchItemCheckpoint{
			BatchID:    batchID,
			LineNumber: item.LineNumber,
			CustomID:   item.CustomID,
			Status:     CheckpointPending,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}

	return s.saveCheckpoints(batchID, checkpoints)
}

// loadCheckpoints 加载检查点。
func (s *Store) loadCheckpoints(batchID string) ([]BatchItemCheckpoint, error) {
	data, err := os.ReadFile(s.checkpointPath(batchID))
	if err != nil {
		return nil, err
	}
	// 读取时收紧既有文件权限（旧版本可能以 0644 写入）。
	_ = os.Chmod(s.checkpointPath(batchID), 0o600)
	var checkpoints []BatchItemCheckpoint
	if err := json.Unmarshal(data, &checkpoints); err != nil {
		return nil, err
	}
	return checkpoints, nil
}

// saveCheckpoints 保存检查点。
func (s *Store) saveCheckpoints(batchID string, checkpoints []BatchItemCheckpoint) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(checkpoints, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.checkpointPath(batchID), data, 0o600); err != nil {
		return err
	}
	if s.onSave != nil {
		s.onSave()
	}
	return nil
}

// CheckpointUpdate 单个检查点的批量更新项。
// Status 取 CheckpointProcessing 时允许创建不存在的检查点（upsert），
// 取 CheckpointCompleted/CheckpointErrored 时要求检查点已存在。
type CheckpointUpdate struct {
	LineNumber int
	CustomID   string
	Status     CheckpointStatus
	Result     interface{}
	Error      interface{}
}

// ApplyCheckpointUpdates 在一次 load + save 中提交多条检查点更新。
//
// 逐行调用 MarkCheckpointResult/MarkCheckpointError 时，每行都要重读并重写整个
// 检查点文件，N 行就是 O(N²) IO；批量处理应使用本方法，把 N 次 IO 降为 1 次。
// 更新是全成或全不成：任一行号不存在（且不是 processing upsert）时不落盘。
func (s *Store) ApplyCheckpointUpdates(batchID string, updates []CheckpointUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	checkpoints, err := s.loadCheckpoints(batchID)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	now := nowUnix()
	index := make(map[int]int, len(checkpoints))
	for i := range checkpoints {
		index[checkpoints[i].LineNumber] = i
	}

	// 先整体校验，避免部分写入后才发现非法行号。
	for _, u := range updates {
		if u.Status == CheckpointProcessing {
			continue
		}
		if _, ok := index[u.LineNumber]; !ok {
			return fmt.Errorf("检查点 line %d 不存在", u.LineNumber)
		}
	}

	for _, u := range updates {
		i, ok := index[u.LineNumber]
		if !ok {
			checkpoints = append(checkpoints, BatchItemCheckpoint{
				BatchID:    batchID,
				LineNumber: u.LineNumber,
				CustomID:   u.CustomID,
				Status:     u.Status,
				CreatedAt:  now,
				UpdatedAt:  now,
			})
			index[u.LineNumber] = len(checkpoints) - 1
			continue
		}

		cp := &checkpoints[i]
		cp.Status = u.Status
		cp.UpdatedAt = now
		if u.CustomID != "" {
			cp.CustomID = u.CustomID
		}
		switch u.Status {
		case CheckpointCompleted:
			cp.Result = u.Result
			cp.Error = nil
		case CheckpointErrored:
			cp.Error = u.Error
			cp.Result = nil
		}
	}

	return s.saveCheckpoints(batchID, checkpoints)
}

// MarkCheckpointProcessing 标记检查点为处理中（单条便捷方法，批量场景请用 ApplyCheckpointUpdates）。
func (s *Store) MarkCheckpointProcessing(batchID string, lineNumber int, customID string) error {
	return s.ApplyCheckpointUpdates(batchID, []CheckpointUpdate{{
		LineNumber: lineNumber,
		CustomID:   customID,
		Status:     CheckpointProcessing,
	}})
}

// MarkCheckpointResult 标记检查点为已完成（单条便捷方法，批量场景请用 ApplyCheckpointUpdates）。
func (s *Store) MarkCheckpointResult(batchID string, lineNumber int, result interface{}) error {
	return s.ApplyCheckpointUpdates(batchID, []CheckpointUpdate{{
		LineNumber: lineNumber,
		Status:     CheckpointCompleted,
		Result:     result,
	}})
}

// MarkCheckpointError 标记检查点为错误（单条便捷方法，批量场景请用 ApplyCheckpointUpdates）。
func (s *Store) MarkCheckpointError(batchID string, lineNumber int, errResult interface{}) error {
	return s.ApplyCheckpointUpdates(batchID, []CheckpointUpdate{{
		LineNumber: lineNumber,
		Status:     CheckpointErrored,
		Error:      errResult,
	}})
}

// ListCheckpoints 列出所有检查点（按行号排序）。
func (s *Store) ListCheckpoints(batchID string) ([]BatchItemCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	checkpoints, err := s.loadCheckpoints(batchID)
	if err != nil {
		return nil, err
	}

	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].LineNumber < checkpoints[j].LineNumber
	})

	return checkpoints, nil
}

// GetPendingCheckpoints 获取待处理的检查点（用于断点续传）。
func (s *Store) GetPendingCheckpoints(batchID string) ([]BatchItemCheckpoint, error) {
	all, err := s.ListCheckpoints(batchID)
	if err != nil {
		return nil, err
	}

	var pending []BatchItemCheckpoint
	for _, cp := range all {
		if cp.Status == CheckpointPending || cp.Status == CheckpointProcessing {
			pending = append(pending, cp)
		}
	}
	return pending, nil
}

// batchIDCounter 保证同一秒内 ID 唯一的计数器。
var batchIDCounter uint64

// generateBatchID 生成唯一 batch ID。
func generateBatchID() string {
	seq := atomic.AddUint64(&batchIDCounter, 1)
	return fmt.Sprintf("batch_%d_%d", time.Now().Unix(), seq)
}
