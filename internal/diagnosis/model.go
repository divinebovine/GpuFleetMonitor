package diagnosis

import "time"

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

const (
	CodeGPUThermalThrottle    = "GPUThermalThrottle"
	CodeMemoryThermalThrottle = "MemoryThermalThrottle"
	CodeECCSingleBitError     = "ECCSingleBitError"
	CodeECCDoubleBitError     = "ECCDoubleBitError"
	CodePowerCapped           = "PowerCapped"
	CodeLowUtilization        = "LowUtilization"
)

type Finding struct {
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
}

type Diagnosis struct {
	ID             string    `json:"id"`
	GPUID          string    `json:"gpu_id"`
	Timestamp      time.Time `json:"timestamp"`
	Severity       Severity  `json:"severity"`
	Findings       []Finding `json:"findings"`
	Recommendation string    `json:"recommendation"`
}
