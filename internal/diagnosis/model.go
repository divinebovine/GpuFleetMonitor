package diagnosis

import "time"

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Finding struct {
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
}

type Diagnosis struct {
	ID             string    `json:"id"`
	GpuId          string    `json:"gpu_id"`
	Timestamp      time.Time `json:"timestamp"`
	Severity       Severity  `json:"severity"`
	Findings       []Finding `json:"findings"`
	Recommendation string    `json:"recommendation"`
}
