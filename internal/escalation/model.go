package escalation

import "time"

type Status string

const (
	StatusOpen     Status = "open"
	StatusResolved Status = "resolved"
)

type Escalation struct {
	ID          string     `json:"id"`
	GPUID       string     `json:"gpu_id"`
	DiagnosisID string     `json:"diagnosis_id"`
	Severity    string     `json:"severity"`
	Status      Status     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
}

func (e *Escalation) Resolve(ts time.Time) {
	e.Status = StatusResolved
	e.ResolvedAt = &ts
}
