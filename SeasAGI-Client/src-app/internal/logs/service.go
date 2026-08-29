package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type RequestLog struct {
	RequestID        string      `json:"request_id"`
	CreatedAt        string      `json:"created_at"`
	LogicalModelName string      `json:"logical_model_name"`
	ChannelID        string      `json:"channel_id"`
	UpstreamModel    string      `json:"upstream_model"`
	RouteTrace       string      `json:"route_trace"`
	RouteSteps       []RouteStep `json:"route_steps,omitempty"`
	Status           string      `json:"status"`
	DurationMs       float64     `json:"duration_ms"`
	ErrorCode        *string     `json:"error_code"`
	ErrorMessage     *string     `json:"error_message"`
	AppliedConstraints string   `json:"applied_constraints,omitempty"` // JSON of constraint conditions applied
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
	s.logs = loaded
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
