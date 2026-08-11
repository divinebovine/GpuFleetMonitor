package fleet

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadingFleetConfig(t *testing.T) {
	configs := []struct {
		expectedToLoad bool
		path           string
	}{
		{true, "../../hack/demo/fleet-config.yaml"},
		{false, "non-existant.yaml"},
	}

	for _, tc := range configs {
		config, err := LoadConfig(tc.path)
		if tc.expectedToLoad && config == nil {
			t.Errorf("expected config to load for %s, but config was nil", tc.path)
		}

		if !tc.expectedToLoad && !errors.Is(err, ErrConfigNotFound) {
			t.Errorf("expected config to fail to load for %s and for err to be ErrConfigNotFound, got %v", tc.path, err)
		}
	}
}

func TestMalforedFleetConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("nodeGroups: [this is not: valid: yaml]"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}

	if !errors.Is(err, ErrConfigMalformed) {
		t.Errorf("expected error for malformed YAML to be ErrConfigMalformed, got %v", err)
	}
}
