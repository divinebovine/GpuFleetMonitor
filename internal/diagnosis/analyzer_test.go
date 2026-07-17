package diagnosis

import (
	"fmt"
	"testing"
	"time"

	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

const testGPUID = "GPU-00001"

func TestAnalyzeGpuCoreTemp(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreCelsius:           92.0,
			GPUCoreCriticalThreshold: 84.0,
		},
	}
	expectedTime := time.Now().UTC()
	expectedCode := CodeGPUThermalThrottle
	expectedDescription := fmt.Sprintf("GPU Core Temperature Critical - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.GPUCoreCelsius, health.Temperature.GPUCoreCriticalThreshold)
	expectedSeverity := SeverityCritical

	actualDiagnosis := Analyze(health, expectedTime)

	if len(actualDiagnosis.Findings) != 1 {
		t.Fatalf("expected one finding, but got %d", len(actualDiagnosis.Findings))
	}

	actualFinding := actualDiagnosis.Findings[0]

	if actualFinding.Code != expectedCode {
		t.Errorf("expected a finding with code '%s', but got '%s'", expectedCode, actualFinding.Code)
	}

	if actualFinding.Description != expectedDescription {
		t.Errorf("expected a finding with description '%s', but got '%s'", expectedDescription, actualFinding.Description)
	}

	if actualFinding.Severity != expectedSeverity {
		t.Errorf("expected a finding with severity '%s', but got '%s'", expectedSeverity, actualFinding.Severity)
	}

	if actualDiagnosis.Timestamp.Location() != time.UTC {
		t.Errorf("expected UTC, got %s", actualDiagnosis.Timestamp.Location())
	}

	if actualDiagnosis.Timestamp != expectedTime {
		t.Errorf("expected a diagnosis with timestamp %s, but got %s", expectedTime.Format(time.RFC3339), actualDiagnosis.Timestamp.Format(time.RFC3339))
	}
}

func TestAnalyzeGpuCoreTempWarning(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreCelsius:           80.0, // above warning (75), below critical (84)
			GPUCoreWarningThreshold:  75.0,
			GPUCoreCriticalThreshold: 84.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != CodeGPUThermalThrottle {
		t.Errorf("expected GPUThermalThrottle, got %s", f.Code)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("expected medium, got %s", f.Severity)
	}
	expectedDesc := fmt.Sprintf("GPU Core Temperature Warning - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.GPUCoreCelsius, health.Temperature.GPUCoreWarningThreshold)
	if f.Description != expectedDesc {
		t.Errorf("expected description '%s', got '%s'", expectedDesc, f.Description)
	}
}

func TestAnalyzeMemoryTempWarning(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			MemoryCelsius:            90.0, // above warning (85), below critical (95)
			MemoryWarningThreshold:   85.0,
			MemoryCriticalThreshold:  95.0,
			GPUCoreWarningThreshold:  75.0,
			GPUCoreCriticalThreshold: 84.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != CodeMemoryThermalThrottle {
		t.Errorf("expected MemoryThermalThrottle, got %s", f.Code)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("expected medium, got %s", f.Severity)
	}
	expectedDesc := fmt.Sprintf("Memory temperature warning - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.MemoryCelsius, health.Temperature.MemoryWarningThreshold)
	if f.Description != expectedDesc {
		t.Errorf("expected description '%s', got '%s'", expectedDesc, f.Description)
	}
}

func TestAnalyzeMemoryTempCritical(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			MemoryCelsius:            96.0, // above critical (95)
			MemoryWarningThreshold:   85.0,
			MemoryCriticalThreshold:  95.0,
			GPUCoreWarningThreshold:  75.0,
			GPUCoreCriticalThreshold: 84.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != CodeMemoryThermalThrottle {
		t.Errorf("expected MemoryThermalThrottle, got %s", f.Code)
	}
	if f.Severity != SeverityCritical {
		t.Errorf("expected critical, got %s", f.Severity)
	}
}

func TestAnalyzeECCSingleBitErrors(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreWarningThreshold:  75.0,
			GPUCoreCriticalThreshold: 84.0,
		},
		Memory: gpu.Memory{
			ECCSingleBitErrors: 6, // at the warning threshold
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "ECCSingleBitError" {
		t.Errorf("expected ECCSingleBitError, got %s", f.Code)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("expected medium, got %s", f.Severity)
	}
}

func TestAnalyzeECCDoubleBitErrors(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreWarningThreshold:  75.0,
			GPUCoreCriticalThreshold: 84.0,
		},
		Memory: gpu.Memory{
			ECCDoubleBitErrors: 1, // at the critical threshold
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "ECCDoubleBitError" {
		t.Errorf("expected ECCDoubleBitError, got %s", f.Code)
	}
	if f.Severity != SeverityCritical {
		t.Errorf("expected critical, got %s", f.Severity)
	}
}

func TestAnalyzePowerCapped(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreWarningThreshold:  75.0,
			GPUCoreCriticalThreshold: 84.0,
		},
		Power: gpu.Power{
			DrawWatts:   665.0,
			LimitWatts:  700.0,
			PowerCapped: true,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "PowerCapped" {
		t.Errorf("expected PowerCapped, got %s", f.Code)
	}
	if f.Severity != SeverityHigh {
		t.Errorf("expected high, got %s", f.Severity)
	}
}

func TestAnalyzeLowGPUUtilization(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 3.0, // below low threshold (5)
		Temperature: gpu.Temperature{
			GPUCoreWarningThreshold:  75.0,
			GPUCoreCriticalThreshold: 84.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "LowUtilization" {
		t.Errorf("expected LowUtilization, got %s", f.Code)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("expected medium, got %s", f.Severity)
	}
}

func TestAnalyzeNoFindings(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       testGPUID,
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreCelsius:           60.0,
			GPUCoreWarningThreshold:  75.0,
			GPUCoreCriticalThreshold: 84.0,
			MemoryCelsius:            55.0,
		},
		Memory: gpu.Memory{
			ECCSingleBitErrors: 0,
			ECCDoubleBitErrors: 0,
		},
		Power: gpu.Power{
			Utilization: 70.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 0 {
		t.Errorf("expected no findings, got %d: %+v", len(d.Findings), d.Findings)
	}
	if d.Severity != SeverityLow {
		t.Errorf("expected low severity, got %s", d.Severity)
	}
}

func TestGetWorstSeverityRanksCorrectly(t *testing.T) {
	severities := []Severity{
		SeverityLow,
		SeverityMedium,
		SeverityHigh,
		SeverityCritical,
	}
	findings := make([]Finding, 0, len(severities))

	for _, value := range severities {
		findings = append(findings, Finding{Severity: value})
		actualSeverity := GetWorstSeverity(findings)
		if actualSeverity != value {
			t.Errorf("expected %s, but got %s", value, actualSeverity)
		}
	}
}

func TestGetRecommendationRecommendsCorrectly(t *testing.T) {
	expectedSeverityRs := map[Severity]string{
		SeverityLow:      "No action required. Continue routine monitoring.",
		SeverityMedium:   "Flag for review at next maintenance window. Continue monitoring.",
		SeverityHigh:     "Schedule maintenance within 24 hours. Monitor closely for deterioration.",
		SeverityCritical: "Immediate intervention required. Remove GPU from service and inspect hardware.",
	}

	for severity, expectedR := range expectedSeverityRs {
		actualR := GetRecommendation(severity)
		if actualR != expectedR {
			t.Errorf("expected %s, but got %s", actualR, expectedR)
		}
	}
}
