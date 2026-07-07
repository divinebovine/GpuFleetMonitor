package gpu

import "testing"

func TestSpecForModelKnownModels(t *testing.T) {
	models := []struct {
		model         string
		maxPowerWatts float64
		memoryBytes   uint64
	}{
		{"H100", 700.0, 80 * GB},
		{"A100", 400.0, 80 * GB},
		{"V100", 300.0, 32 * GB},
		{"A30", 165.0, 24 * GB},
	}

	for _, tc := range models {
		spec, err := SpecForModel(tc.model)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.model, err)
			continue
		}
		if spec.maxPowerWatts != tc.maxPowerWatts {
			t.Errorf("%s: expected maxPowerWatts %.1f, got %.1f", tc.model, tc.maxPowerWatts, spec.maxPowerWatts)
		}
		if spec.memoryBytes != tc.memoryBytes {
			t.Errorf("%s: expected memoryBytes %d, got %d", tc.model, tc.memoryBytes, spec.memoryBytes)
		}
	}
}

func TestSpecForModelUnknown(t *testing.T) {
	_, err := SpecForModel("RTX9090")
	if err == nil {
		t.Error("expected error for unknown model, got nil")
	}
}
