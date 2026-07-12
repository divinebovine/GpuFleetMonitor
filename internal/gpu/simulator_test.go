package gpu

import (
	"context"
	"testing"
)

func TestGetHealth(t *testing.T) {
	cases := []struct {
		id             string
		expectedStatus HealthStatus
	}{
		{"GPU-00001", StatusHealthy},
		{"GPU-00002", StatusWarning},
		{"GPU-00003", StatusCritical},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			DefaultStore.SetStatus(tc.id, tc.expectedStatus)
			t.Cleanup(func() { DefaultStore.SetStatus(tc.id, StatusHealthy) })
			health, err := GetHealth(context.Background(), tc.id)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if health.HealthStatus != tc.expectedStatus {
				t.Errorf("unexpected health status, expected %s, got %s", tc.expectedStatus, health.HealthStatus)
			}
		})
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
