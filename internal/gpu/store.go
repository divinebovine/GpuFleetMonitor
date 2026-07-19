package gpu

import (
	"sync"
)

type gpuState struct {
	status      HealthStatus
	failureType FailureType
}

type Store struct {
	mu     sync.RWMutex
	states map[string]gpuState
}

var DefaultStore *Store

func NewStore(ids []string) *Store {
	states := make(map[string]gpuState, len(ids))
	for _, id := range ids {
		states[id] = gpuState{status: StatusHealthy, failureType: FailureTypeNone}
	}
	return &Store{states: states}
}

func (s *Store) GetState(gpuID string) (HealthStatus, FailureType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[gpuID]
	return state.status, state.failureType, ok
}

func (s *Store) SetState(gpuID string, status HealthStatus, failure FailureType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[gpuID] = gpuState{status: status, failureType: failure}
}

// GetStatus returns just the health status. Kept for backward compatibility with tests.
func (s *Store) GetStatus(gpuID string) (HealthStatus, bool) {
	status, _, ok := s.GetState(gpuID)
	return status, ok
}

// SetStatus changes only the status. Clears the failure type when setting Healthy.
// Kept for backward compatibility with tests; new code should use SetState.
func (s *Store) SetStatus(gpuID string, status HealthStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.states[gpuID]
	failure := existing.failureType
	if status == StatusHealthy {
		failure = FailureTypeNone
	}
	s.states[gpuID] = gpuState{status: status, failureType: failure}
}
