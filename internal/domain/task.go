package domain

import "time"

type Status string

const (
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

type Task struct {
	ID              string            `json:"id"`
	UserID          string            `json:"userId,omitempty"`
	SpecCode        string            `json:"specCode"`
	SourceObjectKey string            `json:"sourceObjectKey"`
	Status          Status            `json:"status"`
	ProcessedUrls   map[string]string `json:"processedUrls"`
	ErrorMsg        string            `json:"errorMsg,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}
