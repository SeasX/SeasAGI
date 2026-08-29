package batch

import "time"

// BatchStatus Batch 状态类型。
type BatchStatus string

const (
	StatusValidating  BatchStatus = "validating"
	StatusFailed      BatchStatus = "failed"
	StatusInProgress  BatchStatus = "in_progress"
	StatusFinalizing  BatchStatus = "finalizing"
	StatusCompleted   BatchStatus = "completed"
	StatusExpired     BatchStatus = "expired"
	StatusCancelling  BatchStatus = "cancelling"
	StatusCancelled   BatchStatus = "cancelled"
)

// CheckpointStatus 检查点状态。
type CheckpointStatus string

const (
	CheckpointPending    CheckpointStatus = "pending"
	CheckpointProcessing CheckpointStatus = "processing"
	CheckpointCompleted  CheckpointStatus = "completed"
	CheckpointErrored    CheckpointStatus = "errored"
)

// BatchRecord Batch 记录。
type BatchRecord struct {
	ID                     string                 `json:"id"`
	Endpoint               string                 `json:"endpoint"`
	CompletionWindow       string                 `json:"completion_window"`
	Status                 BatchStatus            `json:"status"`
	InputFileID            string                 `json:"input_file_id"`
	OutputFileID           string                 `json:"output_file_id,omitempty"`
	ErrorFileID            string                 `json:"error_file_id,omitempty"`
	CreatedAt              int64                  `json:"created_at"`
	InProgressAt           int64                  `json:"in_progress_at,omitempty"`
	ExpiresAt              int64                  `json:"expires_at,omitempty"`
	FinalizingAt           int64                  `json:"finalizing_at,omitempty"`
	CompletedAt            int64                  `json:"completed_at,omitempty"`
	FailedAt               int64                  `json:"failed_at,omitempty"`
	ExpiredAt              int64                  `json:"expired_at,omitempty"`
	CancellingAt           int64                  `json:"cancelling_at,omitempty"`
	CancelledAt            int64                  `json:"cancelled_at,omitempty"`
	RequestCountsTotal     int                    `json:"request_counts_total"`
	RequestCountsCompleted int                    `json:"request_counts_completed"`
	RequestCountsFailed    int                    `json:"request_counts_failed"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	APIKeyID               string                 `json:"api_key_id,omitempty"`
	Errors                 interface{}            `json:"errors,omitempty"`
	Model                  string                 `json:"model,omitempty"`
	Usage                  interface{}            `json:"usage,omitempty"`
}

// BatchItemCheckpoint 检查点记录（支持断点续传）。
type BatchItemCheckpoint struct {
	BatchID    string            `json:"batch_id"`
	LineNumber int               `json:"line_number"`
	CustomID   string            `json:"custom_id,omitempty"`
	Status     CheckpointStatus  `json:"status"`
	Result     interface{}       `json:"result,omitempty"`
	Error      interface{}       `json:"error,omitempty"`
	CreatedAt  int64             `json:"created_at"`
	UpdatedAt  int64             `json:"updated_at"`
}

// CursorPage 游标分页结果。
type CursorPage struct {
	Items []BatchRecord `json:"items"`
	After string        `json:"after,omitempty"` // 下一页游标（最后一条的 ID）
	HasMore bool        `json:"has_more"`
}

// ValidTransitions 状态机合法转换。
var ValidTransitions = map[BatchStatus][]BatchStatus{
	StatusValidating: {StatusInProgress, StatusFailed},
	StatusInProgress: {StatusFinalizing, StatusFailed, StatusCancelling},
	StatusFinalizing: {StatusCompleted, StatusFailed},
	StatusCancelling: {StatusCancelled},
	StatusCompleted:  {}, // 终态
	StatusFailed:     {}, // 终态
	StatusExpired:    {}, // 终态
	StatusCancelled:  {}, // 终态
}

// IsValidTransition 检查状态转换是否合法。
func IsValidTransition(from, to BatchStatus) bool {
	if from == to {
		return true
	}
	allowed, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// nowUnix 返回当前 Unix 时间戳。
func nowUnix() int64 {
	return time.Now().Unix()
}
