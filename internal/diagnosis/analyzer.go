package diagnosis

import (
	"fmt"
	"time"

	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

const (
	eccSingleBitCountWarning  = 6
	eccDoubleBitCountCritical = 1
	lowGpuUtilization         = 5.0 // percent
)

func Analyze(health *gpu.GPUHealth, ts time.Time) *Diagnosis {
	var findings = generateFindings(health)

	var diagnosis = new(Diagnosis)
	diagnosis.ID = fmt.Sprintf("diag-%s", health.GPUID)
	diagnosis.GPUID = health.GPUID
	diagnosis.Timestamp = ts
	diagnosis.Severity = GetWorstSeverity(findings)
	diagnosis.Findings = findings
	diagnosis.Recommendation = GetRecommendation(diagnosis.Severity)

	return diagnosis
}

func generateFindings(health *gpu.GPUHealth) []Finding {
	var findings []Finding

	// check gpu core temperature
	switch {
	case health.Temperature.GPUCoreCelsius >= health.Temperature.GPUCoreCriticalThreshold:
		findings = append(findings, Finding{
			Code:        CodeGPUThermalThrottle,
			Description: fmt.Sprintf("GPU Core Temperature Critical - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.GPUCoreCelsius, health.Temperature.GPUCoreCriticalThreshold),
			Severity:    SeverityCritical,
		})
	case health.Temperature.GPUCoreCelsius >= health.Temperature.GPUCoreWarningThreshold:
		findings = append(findings, Finding{
			Code:        CodeGPUThermalThrottle,
			Description: fmt.Sprintf("GPU Core Temperature Warning - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.GPUCoreCelsius, health.Temperature.GPUCoreWarningThreshold),
			Severity:    SeverityMedium,
		})
	}

	// check memory temperature
	switch {
	case health.Temperature.MemoryCriticalThreshold > 0 && health.Temperature.MemoryCelsius >= health.Temperature.MemoryCriticalThreshold:
		findings = append(findings, Finding{
			Code:        CodeMemoryThermalThrottle,
			Description: fmt.Sprintf("Memory temperature critical - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.MemoryCelsius, health.Temperature.MemoryCriticalThreshold),
			Severity:    SeverityCritical,
		})
	case health.Temperature.MemoryWarningThreshold > 0 && health.Temperature.MemoryCelsius >= health.Temperature.MemoryWarningThreshold:
		findings = append(findings, Finding{
			Code:        CodeMemoryThermalThrottle,
			Description: fmt.Sprintf("Memory temperature warning - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.MemoryCelsius, health.Temperature.MemoryWarningThreshold),
			Severity:    SeverityMedium,
		})
	}

	// check memory errors
	if health.Memory.ECCSingleBitErrors >= eccSingleBitCountWarning {
		findings = append(findings, Finding{
			Code:        CodeECCSingleBitError,
			Description: fmt.Sprintf("ECC single bit errors warning - %d errors detected which exceeds the acceptable threshold of %d errors", health.Memory.ECCSingleBitErrors, eccSingleBitCountWarning),
			Severity:    SeverityMedium,
		})
	}

	if health.Memory.ECCDoubleBitErrors >= eccDoubleBitCountCritical {
		findings = append(findings, Finding{
			Code:        CodeECCDoubleBitError,
			Description: fmt.Sprintf("ECC double bit errors critical - %d errors detected which exceeds the acceptable threshold of %d errors", health.Memory.ECCDoubleBitErrors, eccDoubleBitCountCritical),
			Severity:    SeverityCritical,
		})
	}

	// check power
	if health.Power.PowerCapped {
		findings = append(findings, Finding{
			Code:        CodePowerCapped,
			Description: fmt.Sprintf("Power draw high - drawing %.1f watts which exceeds the acceptable limit of %.1f watts", health.Power.DrawWatts, health.Power.LimitWatts),
			Severity:    SeverityHigh,
		})
	}

	// check gpu utililization
	if health.Utilization <= lowGpuUtilization {
		findings = append(findings, Finding{
			Code:        CodeLowUtilization,
			Description: fmt.Sprintf("GPU utilization low - GPU utilization %.1f%% which is under the acceptable limit of %.1f%%", health.Utilization, lowGpuUtilization),
			Severity:    SeverityMedium,
		})
	}

	return findings
}

func GetWorstSeverity(findings []Finding) Severity {
	isWorseThan := func(a Severity, b Severity) bool {
		order := map[Severity]int{
			SeverityLow:      1,
			SeverityMedium:   2,
			SeverityHigh:     3,
			SeverityCritical: 4,
		}
		return order[a] > order[b]
	}

	worstSeverity := SeverityLow
	for _, finding := range findings {
		if isWorseThan(finding.Severity, worstSeverity) {
			worstSeverity = finding.Severity
		}
	}

	return worstSeverity
}

func GetRecommendation(severity Severity) string {
	switch severity {
	default:
		return "No action required. Continue routine monitoring."
	case SeverityMedium:
		return "Flag for review at next maintenance window. Continue monitoring."
	case SeverityHigh:
		return "Schedule maintenance within 24 hours. Monitor closely for deterioration."
	case SeverityCritical:
		return "Immediate intervention required. Remove GPU from service and inspect hardware."
	}
}
