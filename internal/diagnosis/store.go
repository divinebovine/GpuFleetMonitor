package diagnosis

import (
	"maps"
	"slices"
	"sync"
)

type Store struct {
	mu   sync.Mutex
	data map[string]*Diagnosis
}

func NewStore() *Store {
	return &Store{data: make(map[string]*Diagnosis)}
}

func (s *Store) Save(d *Diagnosis) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[d.ID] = d
}

func (s *Store) GetByID(id string) (*Diagnosis, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.data[id]
	return d, ok
}

func (s *Store) List() []*Diagnosis {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Collect(maps.Values(s.data))
}
