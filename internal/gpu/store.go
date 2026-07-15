package gpu

import (
	"sync"
)

type Store struct {
	mu     sync.RWMutex
	states map[string]HealthStatus
}

var DefaultStore *Store

func NewStore(ids []string) *Store {
	var states = make(map[string]HealthStatus)
	for _, id := range ids {
		states[id] = StatusHealthy
	}

	return &Store{states: states}
}

func (s *Store) GetStatus(gpuID string) (HealthStatus, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h, ok := s.states[gpuID]
	return h, ok
}

func (s *Store) SetStatus(gpuID string, status HealthStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.states[gpuID] = status
}
