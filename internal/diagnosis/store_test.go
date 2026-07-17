package diagnosis

import (
	"fmt"
	"sync"
	"testing"
)

const testDiagGPUID = "GPU-00001"

func TestSaveAndGet(t *testing.T) {
	s := NewStore()
	expected := &Diagnosis{ID: "diag-001", GPUID: testDiagGPUID}
	s.Save(expected)

	actual, ok := s.GetByID(expected.ID)
	if !ok {
		t.Fatalf("expected to find diagnosis, but got nothing")
	}

	if actual.GPUID != expected.GPUID {
		t.Errorf("expected GPUID: %s, got GPUID: %s", expected.GPUID, actual.GPUID)
	}

	if actual.ID != expected.ID {
		t.Errorf("expected ID: %s, got ID: %s", expected.ID, actual.ID)
	}
}

func TestSaveThreadSafe(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup

	for i := range 100 {
		wg.Go(func() {
			d := &Diagnosis{ID: fmt.Sprintf("diag-%d", i), GPUID: fmt.Sprintf("GPU-%d", i)}
			s.Save(d)
		})
	}

	wg.Wait()

	diagnoses := s.List()
	if len(diagnoses) != 100 {
		t.Errorf("expected 100 diagnoses, but got %d", len(diagnoses))
	}
}

func TestList(t *testing.T) {
	s := NewStore()
	for i := range 100 {
		d := &Diagnosis{ID: fmt.Sprintf("diag-%d", i), GPUID: fmt.Sprintf("GPU-%d", i)}
		s.Save(d)
	}

	diagnoses := s.List()
	if len(diagnoses) != 100 {
		t.Errorf("expected 100 diagnoses, but got %d", len(diagnoses))
	}
}
