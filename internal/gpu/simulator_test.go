package gpu

import (
	"context"
	"testing"
)

func TestGetHealth(t *testing.T) {
	// GPU health statuses are deterministic
	// This is a known critical GPU
	health, err := GetHealth(context.Background(), "GPU-00005")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.HealthStatus != StatusCritical {
		t.Errorf("unexpected health status, expected critical, got %s", health.HealthStatus)
	}
}

func TestGetHealthWarning(t *testing.T) {
	health, err := GetHealth(context.Background(), "GPU-00023")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.HealthStatus != StatusWarning {
		t.Errorf("expected warning, got %s", health.HealthStatus)
	}
}

func TestGetHealthHealthy(t *testing.T) {
	health, err := GetHealth(context.Background(), "GPU-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.HealthStatus != StatusHealthy {
		t.Errorf("expected healthy, got %s", health.HealthStatus)
	}
}

func TestInvalidID(t *testing.T) {
	health, err := GetHealth(context.Background(), "invalid-gpu-id")

	if err == nil {
		t.Fatalf("unexpected critical, expected invalid GPUID, got GPUID: %s, NodeID: %s, Slot: %d", health.GPUID, health.NodeID, health.Slot)
	}
}

func TestOutOfRangeGPUID(t *testing.T) {
	health, err := GetHealth(context.Background(), "GPU-99999")

	if err == nil {
		t.Fatalf("unexpected critical, expected GPU ID to be out of range, got GPUID: %s, NodeID: %s, Slot: %d", health.GPUID, health.NodeID, health.Slot)
	}
}

func TestAllIDs(t *testing.T) {
	ids := AllIDs()

	if len(ids) != int(TotalGpus) {
		t.Fatalf("expected %d IDs, got %d", TotalGpus, len(ids))
	}
	if ids[0] != "GPU-00001" {
		t.Errorf("expected first ID to be GPU-00001, got %s", ids[0])
	}
	if ids[len(ids)-1] != "GPU-10000" {
		t.Errorf("expected last ID to be GPU-10000, got %s", ids[len(ids)-1])
	}
}
