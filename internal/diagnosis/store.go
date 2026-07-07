package diagnosis

import (
	"maps"
	"slices"
	"sync"
)

/*
type Store struct {
    mu   sync.Mutex
    data map[string]*Diagnosis
}

To use it:
s.mu.Lock()
defer s.mu.Unlock()
// safe to read/write s.data here

defer runs the unlock when the function returns — so you can't forget to unlock even if there's an early return.

Your task — write internal/diagnosis/store.go with:

1. A Store struct with a sync.Mutex and a map[string]*Diagnosis
2. A NewStore() *Store constructor that initializes the map:
func NewStore() *Store {
    return &Store{data: make(map[string]*Diagnosis)}
}

3. Three methods on *Store:
  - Save(d *Diagnosis) — lock, write to map by ID, unlock
  - GetByID(id string) (*Diagnosis, bool) — lock, read from map, unlock, return value and ok
  - List() []*Diagnosis — lock, collect all values into a slice, unlock, return it
*/

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

	diagnosis, ok := s.data[id]
	return diagnosis, ok
}

func (s *Store) List() []*Diagnosis {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Collect(maps.Values(s.data))
}
