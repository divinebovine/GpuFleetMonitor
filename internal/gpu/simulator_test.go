package gpu

import (
	"context"
	"errors"
	"math"
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

func TestRecover(t *testing.T) {
	const id = "GPU-00001"

	t.Run("returns error for unknown GPU", func(t *testing.T) {
		if err := Recover("GPU-99999"); err == nil {
			t.Error("expected error for unknown GPU ID")
		}
	})

	t.Run("returns ErrGPUUnrecoverable for ecc_double", func(t *testing.T) {
		DefaultStore.SetState(id, StatusCritical, FailureTypeECCDouble)
		t.Cleanup(func() { DefaultStore.SetState(id, StatusHealthy, FailureTypeNone) })

		err := Recover(id)
		if err == nil {
			t.Fatal("expected error for ecc_double GPU")
		}
		if !errors.Is(err, ErrGPUUnrecoverable) {
			t.Errorf("expected ErrGPUUnrecoverable, got %v", err)
		}
		if s, _, _ := DefaultStore.GetState(id); s != StatusCritical {
			t.Errorf("expected status unchanged at Critical, got %s", s)
		}
	})

	t.Run("ecc_single stays at warning after recover", func(t *testing.T) {
		DefaultStore.SetState(id, StatusWarning, FailureTypeECCSingle)
		t.Cleanup(func() { DefaultStore.SetState(id, StatusHealthy, FailureTypeNone) })

		if err := Recover(id); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		s, f, _ := DefaultStore.GetState(id)
		if s != StatusWarning {
			t.Errorf("expected Warning, got %s", s)
		}
		if f != FailureTypeECCSingle {
			t.Errorf("expected ecc_single, got %s", f)
		}
	})

	t.Run("always healthy when recovery warning rate is 0", func(t *testing.T) {
		cfg := Config.Get()
		cfg.RecoveryWarningRate = 0.0
		Config.Set(cfg)
		t.Cleanup(func() { Config.Set(Config.Get()); Config.Reset() })

		for range 100 {
			DefaultStore.SetStatus(id, StatusCritical)
			if err := Recover(id); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s, _ := DefaultStore.GetStatus(id); s != StatusHealthy {
				t.Errorf("expected Healthy with rate 0, got %s", s)
			}
		}
		DefaultStore.SetStatus(id, StatusHealthy)
	})

	t.Run("always warning when recovery warning rate is 1", func(t *testing.T) {
		cfg := Config.Get()
		cfg.RecoveryWarningRate = 1.0
		Config.Set(cfg)
		t.Cleanup(func() { Config.Reset() })

		for range 100 {
			DefaultStore.SetStatus(id, StatusCritical)
			if err := Recover(id); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s, _ := DefaultStore.GetStatus(id); s != StatusWarning {
				t.Errorf("expected Warning with rate 1, got %s", s)
			}
		}
		DefaultStore.SetStatus(id, StatusHealthy)
	})

	t.Run("respects configured warning rate", func(t *testing.T) {
		const targetRate = 0.20
		const trials = 10000
		cfg := Config.Get()
		cfg.RecoveryWarningRate = targetRate
		Config.Set(cfg)
		t.Cleanup(func() { Config.Reset() })

		var warnings int
		for range trials {
			DefaultStore.SetStatus(id, StatusCritical)
			if err := Recover(id); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s, _ := DefaultStore.GetStatus(id); s == StatusWarning {
				warnings++
			}
		}
		DefaultStore.SetStatus(id, StatusHealthy)

		if got := float64(warnings) / trials; math.Abs(got-targetRate) > 0.05 {
			t.Errorf("expected warning rate near %.2f, got %.2f", targetRate, got)
		}
	})
}

func TestReplace(t *testing.T) {
	const id = "GPU-00001"

	t.Run("returns error for unknown GPU", func(t *testing.T) {
		if err := Replace("GPU-99999"); err == nil {
			t.Error("expected error for unknown GPU ID")
		}
	})

	t.Run("always healthy when replacement warning rate is 0", func(t *testing.T) {
		cfg := Config.Get()
		cfg.ReplacementWarningRate = 0.0
		Config.Set(cfg)
		t.Cleanup(func() { Config.Reset() })

		for range 100 {
			DefaultStore.SetStatus(id, StatusCritical)
			if err := Replace(id); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s, _ := DefaultStore.GetStatus(id); s != StatusHealthy {
				t.Errorf("expected Healthy with rate 0, got %s", s)
			}
		}
		DefaultStore.SetStatus(id, StatusHealthy)
	})

	t.Run("always warning when replacement warning rate is 1", func(t *testing.T) {
		cfg := Config.Get()
		cfg.ReplacementWarningRate = 1.0
		Config.Set(cfg)
		t.Cleanup(func() { Config.Reset() })

		for range 100 {
			DefaultStore.SetStatus(id, StatusCritical)
			if err := Replace(id); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s, _ := DefaultStore.GetStatus(id); s != StatusWarning {
				t.Errorf("expected Warning with rate 1, got %s", s)
			}
		}
		DefaultStore.SetStatus(id, StatusHealthy)
	})

	t.Run("respects configured warning rate", func(t *testing.T) {
		const targetRate = 0.10
		const trials = 10000
		cfg := Config.Get()
		cfg.ReplacementWarningRate = targetRate
		Config.Set(cfg)
		t.Cleanup(func() { Config.Reset() })

		var warnings int
		for range trials {
			DefaultStore.SetStatus(id, StatusCritical)
			if err := Replace(id); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s, _ := DefaultStore.GetStatus(id); s == StatusWarning {
				warnings++
			}
		}
		DefaultStore.SetStatus(id, StatusHealthy)

		if got := float64(warnings) / trials; math.Abs(got-targetRate) > 0.05 {
			t.Errorf("expected warning rate near %.2f, got %.2f", targetRate, got)
		}
	})
}

func TestDegradeToWarning(t *testing.T) {
	const id = "GPU-00001"
	const trials = 1000
	t.Cleanup(func() { DefaultStore.SetState(id, StatusHealthy, FailureTypeNone) })

	seen := map[FailureType]int{}
	for range trials {
		DefaultStore.SetState(id, StatusHealthy, FailureTypeNone)
		DegradeToWarning(id)
		s, f, _ := DefaultStore.GetState(id)
		if s != StatusWarning {
			t.Errorf("expected Warning, got %s", s)
		}
		if f == FailureTypeNone || f == FailureTypeECCDouble {
			t.Errorf("DegradeToWarning assigned unexpected failure type %s", f)
		}
		seen[f]++
	}

	// Thermal should be most common, then power, then ecc_single
	if seen[FailureTypeThermal] < seen[FailureTypePower] {
		t.Errorf("thermal should be more common than power: thermal=%d power=%d", seen[FailureTypeThermal], seen[FailureTypePower])
	}
	if seen[FailureTypePower] < seen[FailureTypeECCSingle] {
		t.Errorf("power should be more common than ecc_single: power=%d ecc_single=%d", seen[FailureTypePower], seen[FailureTypeECCSingle])
	}
}

func TestWorsenToCritical(t *testing.T) {
	const id = "GPU-00001"
	t.Cleanup(func() { DefaultStore.SetState(id, StatusHealthy, FailureTypeNone) })

	t.Run("always transitions to Critical", func(t *testing.T) {
		DefaultStore.SetState(id, StatusWarning, FailureTypeThermal)
		WorsenToCritical(id)
		if s, _, _ := DefaultStore.GetState(id); s != StatusCritical {
			t.Errorf("expected Critical, got %s", s)
		}
	})

	t.Run("keeps failure type in most cases", func(t *testing.T) {
		const trials = 1000
		kept := 0
		for range trials {
			DefaultStore.SetState(id, StatusWarning, FailureTypeThermal)
			WorsenToCritical(id)
			if _, f, _ := DefaultStore.GetState(id); f == FailureTypeThermal {
				kept++
			}
		}
		// At 5% ecc_double rate, thermal should be kept at least 850/1000 times
		if kept < 850 {
			t.Errorf("expected failure type preserved in most cases, kept=%d/1000", kept)
		}
	})

	t.Run("can develop ecc_double regardless of existing failure type", func(t *testing.T) {
		const trials = 1000
		gotDouble := 0
		for range trials {
			DefaultStore.SetState(id, StatusWarning, FailureTypeThermal)
			WorsenToCritical(id)
			if _, f, _ := DefaultStore.GetState(id); f == FailureTypeECCDouble {
				gotDouble++
			}
		}
		if gotDouble == 0 {
			t.Error("expected some ecc_double outcomes in 1000 trials at 5% rate")
		}
	})
}

func TestRecoverToHealthy(t *testing.T) {
	const id = "GPU-00001"
	t.Cleanup(func() { DefaultStore.SetState(id, StatusHealthy, FailureTypeNone) })

	t.Run("thermal recovers to healthy", func(t *testing.T) {
		DefaultStore.SetState(id, StatusWarning, FailureTypeThermal)
		RecoverToHealthy(id)
		s, f, _ := DefaultStore.GetState(id)
		if s != StatusHealthy {
			t.Errorf("expected Healthy, got %s", s)
		}
		if f != FailureTypeNone {
			t.Errorf("expected failure type None after recovery, got %s", f)
		}
	})

	t.Run("power recovers to healthy", func(t *testing.T) {
		DefaultStore.SetState(id, StatusWarning, FailureTypePower)
		RecoverToHealthy(id)
		if s, _, _ := DefaultStore.GetState(id); s != StatusHealthy {
			t.Errorf("expected Healthy, got %s", s)
		}
	})

	t.Run("ecc_single is not self-healed", func(t *testing.T) {
		DefaultStore.SetState(id, StatusWarning, FailureTypeECCSingle)
		RecoverToHealthy(id)
		s, f, _ := DefaultStore.GetState(id)
		if s != StatusWarning {
			t.Errorf("expected Warning to be preserved for ecc_single, got %s", s)
		}
		if f != FailureTypeECCSingle {
			t.Errorf("expected ecc_single failure type preserved, got %s", f)
		}
	})
}

func TestStepBackToWarning(t *testing.T) {
	const id = "GPU-00001"
	t.Cleanup(func() { DefaultStore.SetState(id, StatusHealthy, FailureTypeNone) })

	t.Run("thermal steps back to warning preserving failure type", func(t *testing.T) {
		DefaultStore.SetState(id, StatusCritical, FailureTypeThermal)
		StepBackToWarning(id)
		s, f, _ := DefaultStore.GetState(id)
		if s != StatusWarning {
			t.Errorf("expected Warning, got %s", s)
		}
		if f != FailureTypeThermal {
			t.Errorf("expected thermal failure type preserved, got %s", f)
		}
	})

	t.Run("power steps back to warning preserving failure type", func(t *testing.T) {
		DefaultStore.SetState(id, StatusCritical, FailureTypePower)
		StepBackToWarning(id)
		if s, f, _ := DefaultStore.GetState(id); s != StatusWarning || f != FailureTypePower {
			t.Errorf("expected Warning/power, got %s/%s", s, f)
		}
	})

	t.Run("ecc_double cannot step back", func(t *testing.T) {
		DefaultStore.SetState(id, StatusCritical, FailureTypeECCDouble)
		StepBackToWarning(id)
		s, f, _ := DefaultStore.GetState(id)
		if s != StatusCritical {
			t.Errorf("expected Critical to be preserved for ecc_double, got %s", s)
		}
		if f != FailureTypeECCDouble {
			t.Errorf("expected ecc_double preserved, got %s", f)
		}
	})
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
