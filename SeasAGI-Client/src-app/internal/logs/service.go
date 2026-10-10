package logs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// genesisHash 是哈希链起点（全 0 十六进制），表示该记录之前没有更早的受保护记录。
const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

type RequestLog struct {
	RequestID          string      `json:"request_id"`
	CreatedAt          string      `json:"created_at"`
	LogicalModelName   string      `json:"logical_model_name"`
	ChannelID          string      `json:"channel_id"`
	UpstreamModel      string      `json:"upstream_model"`
	RouteTrace         string      `json:"route_trace"`
	RouteSteps         []RouteStep `json:"route_steps,omitempty"`
	Status             string      `json:"status"`
	DurationMs         float64     `json:"duration_ms"`
	HTTPStatus         int         `json:"http_status"`   // 回写客户端的 HTTP 状态码
	TTFTMs             int64       `json:"ttft_ms"`       // 首字节延迟（毫秒），0 表示未收到
	InputTokens        int64       `json:"input_tokens"`  // 上游返回的输入 token 数
	OutputTokens       int64       `json:"output_tokens"` // 上游返回的输出 token 数
	ErrorCode          *string     `json:"error_code"`
	ErrorMessage       *string     `json:"error_message"`
	AppliedConstraints string      `json:"applied_constraints,omitempty"` // JSON of constraint conditions applied
	IntentScenario     string      `json:"intent_scenario,omitempty"`     // M4.5: 场景化意图（code_logic/image_gen/...）
	// RequestPayload 失败请求的脱敏请求体快照（敏感字段已 REDACTED），用于排障与审计。
	RequestPayload string `json:"request_payload,omitempty"`
	// PrevHash / Hash 构成防篡改哈希链：Hash = SHA256(PrevHash + 规范化记录 JSON)。
	// 任一字段被改动都会使 Hash 不匹配，从而被 VerifyIntegrity 检出（审计篡改检测）。
	PrevHash string `json:"prev_hash,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

type RouteStep struct {
	ChannelID     string `json:"channel_id"`
	UpstreamModel string `json:"upstream_model"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
	StepRole      string `json:"step_role,omitempty"`      // primary / backup / last_resort
	WithinStep    bool   `json:"within_step,omitempty"`    // true when this is a provider-fallback within the same logical step
	MaxPrice      *int   `json:"max_price,omitempty"`      // effective max_price constraint for this attempt
	MaxLatencyMs  *int   `json:"max_latency_ms,omitempty"` // effective max_latency_ms constraint
}

const (
	maxLogs      = 500
	persistBatch = 10
)

type Service struct {
	mu       sync.RWMutex
	logs     []RequestLog
	filePath string
	dirty    int
}

func NewService() *Service {
	homeDir, _ := os.UserHomeDir()
	filePath := filepath.Join(homeDir, ".seasagi", "logs.json")

	svc := &Service{
		logs:     make([]RequestLog, 0, 128),
		filePath: filePath,
	}
	svc.loadFromDisk()
	return svc
}

func (s *Service) loadFromDisk() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	var loaded []RequestLog
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	if len(loaded) > maxLogs {
		loaded = loaded[:maxLogs]
	}
	// 历史（未签名）数据补算哈希链，避免旧日志被误报为"链损坏"。
	if !logsHaveIntegrity(loaded) {
		backfillHashes(loaded)
	}
	s.logs = loaded
}

// computeLogHash 计算单条记录的哈希：SHA256(prevHash + 规范化 JSON)。规范化时排除
// Hash 字段自身，使哈希只覆盖记录内容与前序链接。
func computeLogHash(prevHash string, log RequestLog) string {
	log.Hash = ""
	data, err := json.Marshal(log)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(append([]byte(prevHash), data...))
	return hex.EncodeToString(sum[:])
}

// logsHaveIntegrity 判断所有记录是否都已带哈希。
func logsHaveIntegrity(logs []RequestLog) bool {
	for i := range logs {
		if logs[i].Hash == "" {
			return false
		}
	}
	return true
}

// backfillHashes 从最旧记录向最新记录依次补算哈希链（logs 为最新在前）。
func backfillHashes(logs []RequestLog) {
	for i := len(logs) - 1; i >= 0; i-- {
		prev := genesisHash
		if i < len(logs)-1 {
			prev = logs[i+1].Hash
		}
		logs[i].PrevHash = prev
		logs[i].Hash = computeLogHash(prev, logs[i])
	}
}

func (s *Service) persistToDisk() {
	data, err := json.Marshal(s.logs)
	if err != nil {
		return
	}
	dir := filepath.Dir(s.filePath)
	os.MkdirAll(dir, 0700)
	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return
	}
	os.Rename(tmpFile, s.filePath)
}

func (s *Service) RecordLog(log RequestLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if log.CreatedAt == "" {
		log.CreatedAt = time.Now().Format(time.RFC3339)
	}

	// 计算防篡改哈希链：PrevHash 指向当前最新记录的哈希。
	log.PrevHash = genesisHash
	if len(s.logs) > 0 {
		log.PrevHash = s.logs[0].Hash
	}
	log.Hash = computeLogHash(log.PrevHash, log)

	s.logs = append([]RequestLog{log}, s.logs...)
	if len(s.logs) > maxLogs {
		s.logs = s.logs[:maxLogs]
	}

	s.dirty++
	if s.dirty >= persistBatch {
		s.persistToDisk()
		s.dirty = 0
	}

	return nil
}

func (s *Service) ListLogs(limit, offset int) ([]RequestLog, error) {
	return s.ListLogsFiltered(limit, offset, "", "", "", "", "")
}

// GetIntentScenarioStats 按场景聚合最近日志的意图分布（M4.5）。
// ponytail: 500 条内存日志直接线性计数，无需索引/时间桶；需要趋势图时再加时间分桶。
func (s *Service) GetIntentScenarioStats() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := make(map[string]int)
	total := 0
	for _, l := range s.logs {
		if l.IntentScenario == "" {
			continue
		}
		counts[l.IntentScenario]++
		total++
	}
	stats := make([]map[string]any, 0, len(counts))
	for scenario, count := range counts {
		share := 0.0
		if total > 0 {
			share = float64(count) / float64(total)
		}
		stats = append(stats, map[string]any{
			"scenario": scenario,
			"count":    count,
			"share":    share,
		})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i]["count"].(int) > stats[j]["count"].(int) })
	return stats
}

func (s *Service) ListLogsFiltered(limit, offset int, status, channelID, timeFrom, timeTo, keyword string) ([]RequestLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filtered := s.logs
	if status != "" {
		filtered = filterByStatus(filtered, status)
	}
	if channelID != "" {
		filtered = filterByChannel(filtered, channelID)
	}
	if timeFrom != "" || timeTo != "" {
		filtered = filterByTimeRange(filtered, timeFrom, timeTo)
	}
	if keyword != "" {
		filtered = filterByKeyword(filtered, keyword)
	}

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 100
	}
	if offset >= len(filtered) {
		return []RequestLog{}, nil
	}

	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	result := make([]RequestLog, 0, end-offset)
	result = append(result, filtered[offset:end]...)
	return result, nil
}

func filterByStatus(logs []RequestLog, status string) []RequestLog {
	var result []RequestLog
	for _, l := range logs {
		if l.Status == status {
			result = append(result, l)
		}
	}
	return result
}

func filterByChannel(logs []RequestLog, channelID string) []RequestLog {
	var result []RequestLog
	for _, l := range logs {
		if l.ChannelID == channelID {
			result = append(result, l)
		}
	}
	return result
}

func filterByTimeRange(logs []RequestLog, timeFrom, timeTo string) []RequestLog {
	var result []RequestLog
	fromTime, _ := time.Parse(time.RFC3339, timeFrom)
	toTime, _ := time.Parse(time.RFC3339, timeTo)
	for _, l := range logs {
		t, err := time.Parse(time.RFC3339, l.CreatedAt)
		if err != nil {
			continue
		}
		if !fromTime.IsZero() && t.Before(fromTime) {
			continue
		}
		if !toTime.IsZero() && t.After(toTime) {
			continue
		}
		result = append(result, l)
	}
	return result
}

func filterByKeyword(logs []RequestLog, keyword string) []RequestLog {
	kw := strings.ToLower(keyword)
	var result []RequestLog
	for _, l := range logs {
		if strings.Contains(strings.ToLower(l.LogicalModelName), kw) ||
			strings.Contains(strings.ToLower(l.ChannelID), kw) ||
			strings.Contains(strings.ToLower(l.UpstreamModel), kw) ||
			strings.Contains(strings.ToLower(l.RouteTrace), kw) ||
			(l.ErrorMessage != nil && strings.Contains(strings.ToLower(*l.ErrorMessage), kw)) ||
			strings.Contains(strings.ToLower(l.RequestID), kw) {
			result = append(result, l)
		}
	}
	return result
}

func (s *Service) ClearLogs() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = nil
	s.dirty = 0
	s.persistToDisk()

	return nil
}

// VerifyIntegrity 校验哈希链完整性，返回首个校验失败记录的下标；-1 表示链完整。
// 篡改（修改/删除/替换）任一记录的字段都会使其哈希或链接不匹配而被检出。
func (s *Service) VerifyIntegrity() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i, l := range s.logs {
		if l.Hash == "" || l.Hash != computeLogHash(l.PrevHash, l) {
			return i
		}
		// 相邻链接：当前记录的 PrevHash 应等于下一条（更旧）记录的 Hash。
		if i+1 < len(s.logs) && l.PrevHash != s.logs[i+1].Hash {
			return i
		}
	}
	return -1
}
