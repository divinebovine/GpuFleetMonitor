package gpu

import (
	"testing"
)

func TestGetHealth(t *testing.T) {
	// GPU health statuses are deterministic
	// This is a known critical GPU
	health, err := GetHealth("GPU-00005")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.HealthStatus != StatusCritical {
		t.Errorf("unexpected health status, expected critical, got %s", health.HealthStatus)
	}
}

func TestGetHealthWarning(t *testing.T) {
	health, err := GetHealth("GPU-00023")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.HealthStatus != StatusWarning {
		t.Errorf("expected warning, got %s", health.HealthStatus)
	}
}

func TestGetHealthHealthy(t *testing.T) {
	health, err := GetHealth("GPU-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.HealthStatus != StatusHealthy {
		t.Errorf("expected healthy, got %s", health.HealthStatus)
	}
}

func TestInvalidID(t *testing.T) {
	health, err := GetHealth("invalid-gpu-id")

	if err == nil {
		t.Fatalf("unexpected critical, expected invalid GPUID, got GPUID: %s, NodeID: %s, Slot: %d", health.GPUID, health.NodeID, health.Slot)
	}
}

func TestOutOfRangeGPUID(t *testing.T) {
	health, err := GetHealth("GPU-99999")

	if err == nil {
		t.Fatalf("unexpected critical, expected GPU ID to be out of range, got GPUID: %s, NodeID: %s, Slot: %d", health.GPUID, health.NodeID, health.Slot)
	}
}
