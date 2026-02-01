package tasks

import (
	"sync"
	"time"
)

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

type Store struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewStore() *Store {
	return &Store{tasks: make(map[string]*Task)}
}

func (s *Store) Put(t *Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[t.ID] = t
}

func (s *Store) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	return t, ok
}
