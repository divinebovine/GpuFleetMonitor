package diagnosis

import (
	"fmt"
	"testing"
	"time"

	"github.com/divinebovine/gpu-monitor/internal/gpu"
)

func TestAnalyzeGpuCoreTemp(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreCelsius:    92.0,
			CriticalThreshold: 84.0,
		},
	}
	expectedTime := time.Now().UTC()
	expectedCode := "HIGH_TEMPERATURE"
	expectedDescription := fmt.Sprintf("GPU Core Temperature Critical - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.GPUCoreCelsius, health.Temperature.CriticalThreshold)
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
		GPUID:       "GPU-00001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreCelsius:    80.0, // above warning (75), below critical (84)
			WarningThreshold:  75.0,
			CriticalThreshold: 84.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "HIGH_TEMPERATURE" {
		t.Errorf("expected HIGH_TEMPERATURE, got %s", f.Code)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("expected medium, got %s", f.Severity)
	}
	expectedDesc := fmt.Sprintf("GPU Core Temperature Warning - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.GPUCoreCelsius, health.Temperature.WarningThreshold)
	if f.Description != expectedDesc {
		t.Errorf("expected description '%s', got '%s'", expectedDesc, f.Description)
	}
}

func TestAnalyzeMemoryTempWarning(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			MemoryCelsius:     90.0, // above warning (85), below critical (95)
			WarningThreshold:  75.0,
			CriticalThreshold: 84.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "HIGH_TEMPERATURE" {
		t.Errorf("expected HIGH_TEMPERATURE, got %s", f.Code)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("expected medium, got %s", f.Severity)
	}
	expectedDesc := fmt.Sprintf("Memory core temperature warning - %.1f°C detected which exceeds the acceptable threshold of %.1f°C", health.Temperature.MemoryCelsius, float64(memoryTempWarning))
	if f.Description != expectedDesc {
		t.Errorf("expected description '%s', got '%s'", expectedDesc, f.Description)
	}
}

func TestAnalyzeMemoryTempCritical(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			MemoryCelsius:     96.0, // above critical (95)
			WarningThreshold:  75.0,
			CriticalThreshold: 84.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "HIGH_TEMPERATURE" {
		t.Errorf("expected HIGH_TEMPERATURE, got %s", f.Code)
	}
	if f.Severity != SeverityCritical {
		t.Errorf("expected critical, got %s", f.Severity)
	}
}

func TestAnalyzeECCSingleBitErrors(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			WarningThreshold:  75.0,
			CriticalThreshold: 84.0,
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
	if f.Code != "ECC_SINGLE_BIT_ERRORS" {
		t.Errorf("expected ECC_SINGLE_BIT_ERRORS, got %s", f.Code)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("expected medium, got %s", f.Severity)
	}
}

func TestAnalyzeECCDoubleBitErrors(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			WarningThreshold:  75.0,
			CriticalThreshold: 84.0,
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
	if f.Code != "ECC_DOUBLE_BIT_ERROR" {
		t.Errorf("expected ECC_DOUBLE_BIT_ERROR, got %s", f.Code)
	}
	if f.Severity != SeverityCritical {
		t.Errorf("expected critical, got %s", f.Severity)
	}
}

func TestAnalyzeHighPowerUtilization(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			WarningThreshold:  75.0,
			CriticalThreshold: 84.0,
		},
		Power: gpu.Power{
			DrawWatts:   665.0,
			LimitWatts:  700.0,
			Utilization: 95.0, // at the high threshold
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "POWER_LIMIT_APPROACHED" {
		t.Errorf("expected POWER_LIMIT_APPROACHED, got %s", f.Code)
	}
	if f.Severity != SeverityHigh {
		t.Errorf("expected high, got %s", f.Severity)
	}
}

func TestAnalyzeLowGPUUtilization(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		Utilization: 3.0, // below low threshold (5)
		Temperature: gpu.Temperature{
			WarningThreshold:  75.0,
			CriticalThreshold: 84.0,
		},
	}
	ts := time.Now().UTC()
	d := Analyze(health, ts)

	if len(d.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(d.Findings))
	}
	f := d.Findings[0]
	if f.Code != "LOW_GPU_UTILIZATION" {
		t.Errorf("expected LOW_GPU_UTILIZATION, got %s", f.Code)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("expected medium, got %s", f.Severity)
	}
}

func TestAnalyzeNoFindings(t *testing.T) {
	health := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreCelsius:    60.0,
			WarningThreshold:  75.0,
			CriticalThreshold: 84.0,
			MemoryCelsius:     55.0,
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
	var findings []Finding

	severities := []Severity{
		SeverityLow,
		SeverityMedium,
		SeverityHigh,
		SeverityCritical,
	}

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
