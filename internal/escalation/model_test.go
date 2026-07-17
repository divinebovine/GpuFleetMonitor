package escalation

import (
	"testing"
	"time"
)

const (
	testEscID    = "esc-001"
	testEscGPUID = "GPU-00005"
)

func TestResolve(t *testing.T) {
	e := &Escalation{
		ID:     testEscID,
		GPUID:  testEscGPUID,
		Status: StatusOpen,
	}

	ts := time.Now().UTC()
	e.Resolve(ts)

	if e.Status != StatusResolved {
		t.Errorf("expected status resolved, got %s", e.Status)
	}

	if e.ResolvedAt == nil {
		t.Fatal("expected ResolvedAt to be set, got nil")
	}

	if *e.ResolvedAt != ts {
		t.Errorf("expected ResolvedAt %s, got %s", ts, *e.ResolvedAt)
	}
}

func TestResolveDoesNotAffectOtherFields(t *testing.T) {
	e := &Escalation{
		ID:          testEscID,
		GPUID:       testEscGPUID,
		DiagnosisID: "diag-GPU-00005",
		Severity:    "critical",
		Status:      StatusOpen,
	}

	e.Resolve(time.Now().UTC())

	if e.ID != testEscID {
		t.Errorf("expected ID esc-001, got %s", e.ID)
	}
	if e.GPUID != testEscGPUID {
		t.Errorf("expected GPUID GPU-00005, got %s", e.GPUID)
	}
	if e.DiagnosisID != "diag-GPU-00005" {
		t.Errorf("expected DiagnosisID diag-GPU-00005, got %s", e.DiagnosisID)
	}
	if e.Severity != "critical" {
		t.Errorf("expected severity critical, got %s", e.Severity)
	}
}
