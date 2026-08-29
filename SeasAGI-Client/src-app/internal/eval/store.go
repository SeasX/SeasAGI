package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Store Eval 持久化存储。
type Store struct {
	mu      sync.RWMutex
	dataDir string
}

// NewStore 创建存储。
func NewStore(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// ensureDir 确保目录存在。
func (s *Store) ensureDir() error {
	return os.MkdirAll(s.dataDir, 0755)
}

// suitePath 返回 suite 文件路径。
func (s *Store) suitePath(suiteID string) string {
	return filepath.Join(s.dataDir, fmt.Sprintf("suite-%s.json", suiteID))
}

// runPath 返回 run 文件路径。
func (s *Store) runPath(runID string) string {
	return filepath.Join(s.dataDir, fmt.Sprintf("run-%s.json", runID))
}

// SaveSuite 保存测试套件。
func (s *Store) SaveSuite(suite *EvalSuite) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	data, err := json.MarshalIndent(suite, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 suite 失败: %w", err)
	}

	return os.WriteFile(s.suitePath(suite.ID), data, 0644)
}

// LoadSuite 加载测试套件。
func (s *Store) LoadSuite(suiteID string) (*EvalSuite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.suitePath(suiteID))
	if err != nil {
		return nil, err
	}

	var suite EvalSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, err
	}
	return &suite, nil
}

// ListSuites 列出所有 suite。
func (s *Store) ListSuites() ([]*EvalSuite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil, err
	}

	var suites []*EvalSuite
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !filepath.IsAbs(name) && !matchSuiteFile(name) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dataDir, name))
		if err != nil {
			continue
		}

		var suite EvalSuite
		if err := json.Unmarshal(data, &suite); err != nil {
			continue
		}
		suites = append(suites, &suite)
	}

	// 按 CreatedAt 降序排序
	sort.Slice(suites, func(i, j int) bool {
		return suites[i].CreatedAt > suites[j].CreatedAt
	})

	return suites, nil
}

// matchSuiteFile 检查文件名是否符合 suite 文件模式。
func matchSuiteFile(name string) bool {
	return len(name) > 6 && name[:6] == "suite-" && filepath.Ext(name) == ".json"
}

// matchRunFile 检查文件名是否符合 run 文件模式。
func matchRunFile(name string) bool {
	return len(name) > 4 && name[:4] == "run-" && filepath.Ext(name) == ".json"
}

// SaveRun 保存运行记录。
func (s *Store) SaveRun(run *PersistedEvalRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 run 失败: %w", err)
	}

	return os.WriteFile(s.runPath(run.ID), data, 0644)
}

// LoadRun 加载运行记录。
func (s *Store) LoadRun(runID string) (*PersistedEvalRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.runPath(runID))
	if err != nil {
		return nil, err
	}

	var run PersistedEvalRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// ListRuns 列出所有运行记录（按时间降序）。
func (s *Store) ListRuns() ([]*PersistedEvalRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil, err
	}

	var runs []*PersistedEvalRun
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !matchRunFile(name) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dataDir, name))
		if err != nil {
			continue
		}

		var run PersistedEvalRun
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		runs = append(runs, &run)
	}

	// 按 CreatedAt 降序排序
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt > runs[j].CreatedAt
	})

	return runs, nil
}

// ListRunsByTarget 按 target 类型+引用列出运行记录。
func (s *Store) ListRunsByTarget(targetType TargetType, targetRef string) ([]*PersistedEvalRun, error) {
	allRuns, err := s.ListRuns()
	if err != nil {
		return nil, err
	}

	var filtered []*PersistedEvalRun
	for _, run := range allRuns {
		if run.TargetType == targetType && run.TargetRef == targetRef {
			filtered = append(filtered, run)
		}
	}

	return filtered, nil
}

// ListRunsBySuite 按 suite ID 列出运行记录。
func (s *Store) ListRunsBySuite(suiteID string) ([]*PersistedEvalRun, error) {
	allRuns, err := s.ListRuns()
	if err != nil {
		return nil, err
	}

	var filtered []*PersistedEvalRun
	for _, run := range allRuns {
		if run.SuiteID == suiteID {
			filtered = append(filtered, run)
		}
	}

	return filtered, nil
}

// GetLatestRunForTarget 获取指定目标的最新运行记录（供路由决策参考）。
func (s *Store) GetLatestRunForTarget(targetType TargetType, targetRef string) (*PersistedEvalRun, error) {
	runs, err := s.ListRunsByTarget(targetType, targetRef)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return runs[0], nil // ListRunsByTarget 已按时间降序
}

// DeleteSuite 删除测试套件。
func (s *Store) DeleteSuite(suiteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return os.Remove(s.suitePath(suiteID))
}

// DeleteRun 删除运行记录。
func (s *Store) DeleteRun(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return os.Remove(s.runPath(runID))
}
