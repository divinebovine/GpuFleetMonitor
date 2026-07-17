package escalation

import (
	"fmt"
	"sync"
	"testing"
)

func TestSaveAndGet(t *testing.T) {
	s := NewStore()
	expected := Escalation{ID: testEscID, GPUID: testEscGPUID}
	s.Save(expected)

	actual, ok := s.GetByID(testEscID)
	if !ok {
		t.Fatal("expected to find escalation, got nothing")
	}
	if actual.ID != expected.ID {
		t.Errorf("expected ID %s, got %s", expected.ID, actual.ID)
	}
	if actual.GPUID != expected.GPUID {
		t.Errorf("expected GPUID %s, got %s", expected.GPUID, actual.GPUID)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	s := NewStore()

	_, ok := s.GetByID("does-not-exist")
	if ok {
		t.Error("expected not found, but got ok=true")
	}
}

func TestList(t *testing.T) {
	s := NewStore()
	for i := range 10 {
		s.Save(Escalation{ID: fmt.Sprintf("esc-%d", i), GPUID: fmt.Sprintf("GPU-%05d", i+1)})
	}

	all := s.List()
	if len(all) != 10 {
		t.Errorf("expected 10 escalations, got %d", len(all))
	}
}

func TestSaveThreadSafe(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup

	for i := range 100 {
		wg.Go(func() {
			s.Save(Escalation{ID: fmt.Sprintf("esc-%d", i), GPUID: fmt.Sprintf("GPU-%05d", i+1)})
		})
	}

	wg.Wait()

	all := s.List()
	if len(all) != 100 {
		t.Errorf("expected 100 escalations, got %d", len(all))
	}
}
