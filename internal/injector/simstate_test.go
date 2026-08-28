package injector

import (
	"math"
	"slices"
	"testing"

	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

func TestNewGpuStates(t *testing.T) {
	entities := []uint{1, 2, 3, 4, 5}
	model := "H100"
	states := NewGPUStates(entities, model)

	if len(states) != len(entities) {
		t.Errorf("gpu state count does not matche expected %d, got %d", len(entities), len(states))
	}

	for _, state := range states {
		if state.Model != model {
			t.Errorf("model does not match expected %s, got %s", model, state.Model)
		}

		if state.Status != gpu.StatusHealthy {
			t.Errorf("status does not match expected %s, got %s", gpu.StatusHealthy, state.Status)
		}

		if state.FailureType != gpu.FailureTypeNone {
			t.Errorf("failure type does not match expected %s, got %s", gpu.FailureTypeNone, state.FailureType)
		}
	}
}

func TestAdvanceStateChanges(t *testing.T) {
	cases := []struct {
		states                []GPUState
		cfg                   *gpu.SimulationSettings
		expectedHealthyCount  int
		expectedWarningCount  int
		expectedCriticalCount int
		expectedFailureTypes  []gpu.FailureType
	}{
		{
			// test no state changes
			states: []GPUState{
				createGPUState(gpu.StatusHealthy, gpu.FailureTypeNone),
				createGPUState(gpu.StatusWarning, gpu.FailureTypePower),
				createGPUState(gpu.StatusCritical, gpu.FailureTypeGPUThermal),
			},
			cfg:                   createCfg(0, 0, 0, 0, 0),
			expectedHealthyCount:  1,
			expectedWarningCount:  1,
			expectedCriticalCount: 1,
			expectedFailureTypes:  []gpu.FailureType{gpu.FailureTypePower, gpu.FailureTypeGPUThermal},
		},
		{
			// test healthy to warning and warning to critical
			states: []GPUState{
				createGPUState(gpu.StatusHealthy, gpu.FailureTypeNone),
				createGPUState(gpu.StatusWarning, gpu.FailureTypePower),
				createGPUState(gpu.StatusCritical, gpu.FailureTypeGPUThermal),
			},
			cfg:                   createCfg(1.0, 1.0, 0, 0, 0),
			expectedHealthyCount:  0,
			expectedWarningCount:  1,
			expectedCriticalCount: 2,
			expectedFailureTypes: []gpu.FailureType{
				gpu.FailureTypeMemoryThermal,
				gpu.FailureTypeGPUThermal,
				gpu.FailureTypePower,
				gpu.FailureTypeECCSingle,
			},
		},
		{
			// test warning to healthy
			states: []GPUState{
				createGPUState(gpu.StatusHealthy, gpu.FailureTypeNone),
				createGPUState(gpu.StatusWarning, gpu.FailureTypePower),
				createGPUState(gpu.StatusCritical, gpu.FailureTypeGPUThermal),
			},
			cfg:                   createCfg(0, 0, 1.0, 0, 0),
			expectedHealthyCount:  2,
			expectedWarningCount:  0,
			expectedCriticalCount: 1,
			expectedFailureTypes:  []gpu.FailureType{gpu.FailureTypePower, gpu.FailureTypeGPUThermal},
		},
		{
			// test critical to warning
			states: []GPUState{
				createGPUState(gpu.StatusHealthy, gpu.FailureTypeNone),
				createGPUState(gpu.StatusWarning, gpu.FailureTypePower),
				createGPUState(gpu.StatusCritical, gpu.FailureTypeECCSingle),
			},
			cfg:                   createCfg(0, 0, 0, 1.0, 0),
			expectedHealthyCount:  1,
			expectedWarningCount:  2,
			expectedCriticalCount: 0,
			expectedFailureTypes:  []gpu.FailureType{gpu.FailureTypePower, gpu.FailureTypeECCSingle},
		},
		{
			// test that ECC Single failures never exit warning status
			states: []GPUState{
				createGPUState(gpu.StatusWarning, gpu.FailureTypeECCSingle),
				createGPUState(gpu.StatusWarning, gpu.FailureTypeECCSingle),
				createGPUState(gpu.StatusWarning, gpu.FailureTypeECCSingle),
			},
			cfg:                   createCfg(0, 0, 1.0, 0.0, 0),
			expectedHealthyCount:  0,
			expectedWarningCount:  3,
			expectedCriticalCount: 0,
			expectedFailureTypes:  []gpu.FailureType{gpu.FailureTypeECCSingle},
		},
	}

	for _, c := range cases {
		_ = Advance(c.states, c.cfg)
		actualHealthyCount := 0
		actualWarningCount := 0
		actualCriticalCount := 0
		for _, state := range c.states {
			switch state.Status {
			case gpu.StatusHealthy:
				actualHealthyCount++
			case gpu.StatusWarning:
				actualWarningCount++
			case gpu.StatusCritical:
				actualCriticalCount++
			}

			if state.Status == gpu.StatusHealthy && state.FailureType != gpu.FailureTypeNone {
				t.Errorf("advance result failure type for %s should be %s, got %s", state.Status, gpu.FailureTypeNone, state.FailureType)
			}

			if state.Status != gpu.StatusHealthy && !slices.Contains(c.expectedFailureTypes, state.FailureType) {
				t.Errorf("advance result failure type for %s should be a value in %v, got %s", state.Status, c.expectedFailureTypes, state.FailureType)
			}
		}

		if c.expectedHealthyCount != actualHealthyCount {
			t.Errorf("advance failed mutating healthy states, expected %d healthy states, got %d", c.expectedHealthyCount, actualHealthyCount)
		}
		if c.expectedWarningCount != actualWarningCount {
			t.Errorf("advance failed mutating warning states, expected %d healthy states, got %d", c.expectedWarningCount, actualWarningCount)
		}
		if c.expectedCriticalCount != actualCriticalCount {
			t.Errorf("advance failed mutating critical states, expected %d healthy states, got %d", c.expectedCriticalCount, actualCriticalCount)
		}
	}
}

func TestAdvanceJustFailedHardResult(t *testing.T) {
	cases := []struct {
		states               []GPUState
		cfg                  *gpu.SimulationSettings
		expectedCount        int
		expectedFailureTypes []gpu.FailureType
		tolerance            float64
	}{
		{
			// test no hardware failures
			states:        createGPUStates(gpu.StatusWarning, gpu.FailureTypeNone, 1000),
			cfg:           createCfg(0, 1.0, 0, 0, 0),
			expectedCount: 0,
		},
		{
			// test hardware failures
			states:               createGPUStates(gpu.StatusWarning, gpu.FailureTypeNone, 1000),
			cfg:                  createCfg(0, 1.0, 0, 0, 1.0),
			expectedCount:        1000,
			expectedFailureTypes: []gpu.FailureType{gpu.FailureTypeECCDouble, gpu.FailureTypeGpuFellOffTheBus},
			tolerance:            0.05, // std dev should be 1.6%, but lets be generous to avoid test flakiness
		},
	}

	for _, c := range cases {
		results := Advance(c.states, c.cfg)
		actualCount := len(results)
		if c.expectedCount != actualCount {
			t.Fatalf("expected just failed hard count to be %d, got %d", c.expectedCount, actualCount)
		}

		if c.expectedCount == 0 {
			// testing this case further is unnecessary
			continue
		}

		failureMap := make(map[gpu.FailureType]int)
		for _, result := range results {
			if !slices.Contains(c.expectedFailureTypes, result.FailureType) {
				t.Fatalf("unexpected failure type. expected a value in: %v, got %s", c.expectedFailureTypes, result.FailureType)
			}
			failureMap[result.FailureType]++
		}

		// simulated hardware failures should split evenly amongst possible values
		expectedSplit := 1.0 / float64(len(c.expectedFailureTypes))
		for k, v := range failureMap {
			actualSplit := float64(v) / float64(actualCount)
			if !WithinTolerance(expectedSplit, actualSplit, c.tolerance) {
				t.Errorf("was expecting an even split of failure types for %s at %f, got %f", k, expectedSplit, actualSplit)
			}
		}
	}
}

func TestAdvanceHardwareFailureBoost(t *testing.T) {
	t.Run("respects an explicit hardware failure rate of 0", func(t *testing.T) {
		// an operator setting HardwareFailureRate to 0 should mean hardware
		// failures are fully disabled, even for persistent-failure GPUs —
		// the boost must not silently override an explicit off-switch.
		states := createGPUStates(gpu.StatusWarning, gpu.FailureTypeECCSingle, 1000)
		cfg := createCfg(0, 1.0, 0, 0, 0)

		results := Advance(states, cfg)

		if len(results) != 0 {
			t.Errorf("expected no hardware failures when rate is explicitly 0, got %d", len(results))
		}
	})

	t.Run("boosts persistent failures by approximately 0.1", func(t *testing.T) {
		// a base rate near (but not exactly) 0 still triggers the boost,
		// landing close to 0.1 with no clamping to distort the result —
		// this is what actually pins the boost magnitude down.
		states := createGPUStates(gpu.StatusWarning, gpu.FailureTypeECCSingle, 1000)
		cfg := createCfg(0, 1.0, 0, 0, 0.0000001)

		results := Advance(states, cfg)

		actualRate := float64(len(results)) / float64(len(states))
		if !WithinTolerance(0.1, actualRate, 0.03) {
			t.Errorf("expected boosted hardware failure rate near 0.1, got %f", actualRate)
		}
	})

	t.Run("clamps the boosted rate at 1.0", func(t *testing.T) {
		// 0.95 alone would leave ~50/1000 not hard-failed, but the +0.1 boost
		// clamps to 1.0, so every GPU should hard-fail with no tolerance needed.
		states := createGPUStates(gpu.StatusWarning, gpu.FailureTypeECCSingle, 1000)
		cfg := createCfg(0, 1.0, 0, 0, 0.95)

		results := Advance(states, cfg)

		if len(results) != len(states) {
			t.Errorf("expected all states to hard-fail once the boosted rate clamps to 1.0, got %d/%d", len(results), len(states))
		}
	})
}

func createGPUState(status gpu.HealthStatus, failureType gpu.FailureType) GPUState {
	return createGPUStates(status, failureType, 1)[0]
}

func createGPUStates(status gpu.HealthStatus, failureType gpu.FailureType, count int) []GPUState {
	out := make([]GPUState, 0, count)
	for range count {
		out = append(out, GPUState{
			EntityID:    0,
			Model:       "",
			Status:      status,
			FailureType: failureType,
		})
	}
	return out
}

func createCfg(h2wr float64, w2cr float64, w2hr float64, c2wr float64, hwfr float64) *gpu.SimulationSettings {
	return &gpu.SimulationSettings{
		HealthyToWarningRate:  h2wr,
		WarningToCriticalRate: w2cr,
		WarningToHealthyRate:  w2hr,
		CriticalToWarningRate: c2wr,
		HardwareFailureRate:   hwfr,
	}
}

func WithinTolerance(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}
