package escalation

import (
	"maps"
	"slices"
	"sync"
)

type Store struct {
	mu   sync.Mutex
	data map[string]Escalation
}

func NewStore() *Store {
	return &Store{data: make(map[string]Escalation)}
}

func (s *Store) Save(e Escalation) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[e.ID] = e
}

func (s *Store) GetByID(id string) (Escalation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[id]
	return e, ok
}

func (s *Store) List() []Escalation {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Collect(maps.Values(s.data))
}
