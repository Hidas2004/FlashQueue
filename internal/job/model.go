package job

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusRetrying  Status = "retrying"
	StatusDead      Status = "dead"
)

type Job struct {
	ID           string          `db:"id" json:"id"`
	Type         string          `db:"type"json:"type"`
	Payload      json.RawMessage `db:"payload"       json:"payload"`
	Status       Status          `db:"status"        json:"status"`
	Priority     int             `db:"priority"      json:"priority"`
	Version      int             `db:"version"       json:"version"`
	RetryCount   int             `db:"retry_count"   json:"retry_count"`
	MaxRetries   int             `db:"max_retries"   json:"max_retries"`
	ErrorMessage *string         `db:"error_message" json:"error_message,omitempty"`
	NextRetryAt  *time.Time      `db:"next_retry_at" json:"next_retry_at,omitempty"`
	ScheduledAt  *time.Time      `db:"scheduled_at"  json:"scheduled_at,omitempty"`
	StartedAt    *time.Time      `db:"started_at"    json:"started_at,omitempty"`
	CompletedAt  *time.Time      `db:"completed_at"  json:"completed_at,omitempty"`

	CreatedAt time.Time `db:"created_at"    json:"created_at"`
	UpdatedAt time.Time `db:"updated_at"    json:"updated_at"`
}

// DTO
type CreateJobRequest struct {
	Type        string          `json:"type"         binding:"required"`
	Payload     json.RawMessage `json:"payload"      binding:"required"` // Ép buộc client phải gửi payload
	Priority    int             `json:"priority"`
	MaxRetries  int             `json:"max_retries"`  // Nếu không gửi, lát nữa Default sẽ là 3
	ScheduledAt *time.Time      `json:"scheduled_at"` // Lập lịch chạy tương lai
}
