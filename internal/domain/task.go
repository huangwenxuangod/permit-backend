package domain

import "time"

type Status string

const (
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

type TaskSpec struct {
	Code     string `json:"code"`
	WidthPx  int    `json:"widthPx"`
	HeightPx int    `json:"heightPx"`
	DPI      int    `json:"dpi"`
}

type SpecDef struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	WidthPx  int      `json:"widthPx"`
	HeightPx int      `json:"heightPx"`
	DPI      int      `json:"dpi"`
	BgColors []string `json:"bgColors"`
}

type Task struct {
	ID              string            `json:"id"`
	UserID          string            `json:"userId,omitempty"`
	SpecCode        string            `json:"specCode"`
	Spec            TaskSpec          `json:"spec"`
	ItemID          int               `json:"itemId,omitempty"`
	Watermark       bool              `json:"watermark,omitempty"`
	Beauty          int               `json:"beauty,omitempty"`
	Enhance         int               `json:"enhance,omitempty"`
	SourceObjectKey string            `json:"sourceObjectKey"`
	Status          Status            `json:"status"`
	BaselineUrl     string            `json:"baselineUrl,omitempty"`
	AvailableColors []string          `json:"availableColors,omitempty"`
	ProcessedUrls   map[string]string `json:"processedUrls"`
	LayoutUrls      map[string]string `json:"layoutUrls,omitempty"`
	ErrorMsg        string            `json:"errorMsg,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

type DownloadTokenStatus string

const (
	DownloadTokenActive  DownloadTokenStatus = "active"
	DownloadTokenUsed    DownloadTokenStatus = "used"
	DownloadTokenExpired DownloadTokenStatus = "expired"
	DownloadTokenRevoked DownloadTokenStatus = "revoked"
)

type DownloadToken struct {
	Token     string              `json:"token"`
	TaskID    string              `json:"taskId"`
	UserID    string              `json:"userId"`
	Status    DownloadTokenStatus `json:"status"`
	ExpiresAt time.Time           `json:"expiresAt"`
	CreatedAt time.Time           `json:"createdAt"`
	UsedAt    time.Time           `json:"usedAt,omitempty"`
}
