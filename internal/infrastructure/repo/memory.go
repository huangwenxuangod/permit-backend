package repo

import (
	"sync"
	"permit-backend/internal/domain"
)

type MemoryTaskRepo struct {
	mu sync.RWMutex
	m  map[string]*domain.Task
}

func NewMemoryTaskRepo() *MemoryTaskRepo {
	return &MemoryTaskRepo{m: make(map[string]*domain.Task)}
}

func (r *MemoryTaskRepo) Put(t *domain.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[t.ID] = t
	return nil
}

func (r *MemoryTaskRepo) Get(id string) (*domain.Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.m[id]
	return t, ok
}

type MemoryOrderRepo struct {
	mu sync.RWMutex
	m  map[string]*domain.Order
}

func NewMemoryOrderRepo() *MemoryOrderRepo {
	return &MemoryOrderRepo{m: make(map[string]*domain.Order)}
}

func (r *MemoryOrderRepo) Put(o *domain.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[o.OrderID] = o
	return nil
}

func (r *MemoryOrderRepo) Get(id string) (*domain.Order, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.m[id]
	return o, ok
}

func (r *MemoryOrderRepo) List(page, pageSize int) ([]domain.Order, int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]domain.Order, 0, len(r.m))
	for _, o := range r.m {
		all = append(all, *o)
	}
	total := len(all)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return all[start:end], total
}
