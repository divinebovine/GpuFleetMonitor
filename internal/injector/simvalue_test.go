package injector

import (
	"testing"

	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

func TestAdvancePowerUsageInstantaneousValuesWithinSpec(t *testing.T) {
	cases := []struct {
		state            GPUState
		expectedRangeKey gpu.HealthStatus
	}{
		{
			state: GPUState{
				FailureType: gpu.FailureTypeNone,
				Status:      gpu.StatusHealthy,
				Values:      make(map[FieldID]float64),
			},
			expectedRangeKey: gpu.StatusHealthy,
		},
		{
			state: GPUState{
				FailureType: gpu.FailureTypeGPUThermal,
				Status:      gpu.StatusWarning,
				Values:      make(map[FieldID]float64),
			},
			expectedRangeKey: gpu.StatusHealthy,
		},
		{
			state: GPUState{
				FailureType: gpu.FailureTypePower,
				Status:      gpu.StatusWarning,
				Values:      make(map[FieldID]float64),
			},
			expectedRangeKey: gpu.StatusWarning,
		},
		{
			state: GPUState{
				FailureType: gpu.FailureTypePower,
				Status:      gpu.StatusCritical,
				Values:      make(map[FieldID]float64),
			},
			expectedRangeKey: gpu.StatusCritical,
		},
	}

	spec := gpu.GPUSpec{
		Power: map[gpu.HealthStatus]gpu.Range64{
			gpu.StatusHealthy:  {Min: 30, Max: 60},
			gpu.StatusWarning:  {Min: 60, Max: 80},
			gpu.StatusCritical: {Min: 80, Max: 100},
		},
	}

	for _, c := range cases {
		advancePowerUsage(&c.state, spec)
		powerUsage := c.state.Values[FieldPowerUsage]
		expectedRange := spec.Power[c.expectedRangeKey]
		if powerUsage < expectedRange.Min || powerUsage > expectedRange.Max {
			t.Errorf("expected a power usage between %f and %f, got %f", expectedRange.Min, expectedRange.Max, powerUsage)
		}
	}
}

func TestNextBoundedStep(t *testing.T) {
	t.Run("steps up toward target when below range", func(t *testing.T) {
		stepSize := 5.0
		target := gpu.Range64{Min: 75, Max: 84}
		fuzz := gpu.Range64{Min: 0.7, Max: 0.7}

		curVal := 30.0
		for range 200 {
			result := nextBoundedStep(curVal, target, stepSize, fuzz)
			if result < curVal+ThermalStepSize-0.7 || result > curVal+ThermalStepSize+0.7 {
				t.Fatalf("expected result within one step of %f, got %f", curVal, result)
			}
		}
	})

	t.Run("steps down toward target when above range", func(t *testing.T) {
		stepSize := 5.0
		target := gpu.Range64{Min: 75, Max: 84}
		fuzz := gpu.Range64{Min: 0.7, Max: 0.7}

		curVal := 100.0
		for range 200 {
			result := nextBoundedStep(curVal, target, stepSize, fuzz)
			if result < curVal-stepSize-fuzz.Min || result > curVal-stepSize+fuzz.Max {
				t.Fatalf("expected result within one step of %f, got %f", curVal, result)
			}
		}
	})

	t.Run("wanders near the current value once inside the range instead of resampling the whole range", func(t *testing.T) {
		stepSize := 5.0
		target := gpu.Range64{Min: 75, Max: 84}
		fuzz := gpu.Range64{Min: 0.7, Max: 0.7}
		curVal := 76.0 // near the low edge, so a full-range resample would be detectable
		for range 500 {
			result := nextBoundedStep(curVal, target, stepSize, fuzz)
			if result < target.Min || result > target.Max {
				t.Fatalf("expected result within target range [%f, %f], got %f", target.Min, target.Max, result)
			}
			if result > curVal+stepSize+fuzz.Max {
				t.Fatalf("expected result to stay near %f, got %f -- looks like a full-range resample instead of a bounded step", curVal, result)
			}
		}
	})

	t.Run("falls back to a value within target when a step would overshoot it entirely", func(t *testing.T) {
		stepSize := 5.0
		target := gpu.Range64{Min: 50, Max: 52} // narrower than a single step can safely cross
		fuzz := gpu.Range64{Min: 0.7, Max: 0.7}
		curVal := 49.0
		for range 200 {
			result := nextBoundedStep(curVal, target, stepSize, fuzz)
			if result < target.Min || result > target.Max {
				t.Fatalf("expected overshoot fallback to land within [%f, %f], got %f", target.Min, target.Max, result)
			}
		}
	})

	t.Run("respects a step size and fuzz range other than the thermal defaults", func(t *testing.T) {
		// deliberately far from ThermalStepSize/ThermalFuzz to prove the
		// function is genuinely parameterized, not just nominally --
		// nothing else in this suite exercises a different tuning.
		stepSize := 20.0
		fuzz := gpu.Range64{Min: -2.0, Max: 2.0}
		target := gpu.Range64{Min: 200, Max: 400}
		curVal := 210.0 // near the low edge, so a full-range resample would be detectable
		for range 500 {
			result := nextBoundedStep(curVal, target, stepSize, fuzz)
			if result < target.Min || result > target.Max {
				t.Fatalf("expected result within target range [%f, %f], got %f", target.Min, target.Max, result)
			}
			if result > curVal+stepSize+fuzz.Max {
				t.Fatalf("expected result to stay within one step (stepSize=%f) of %f, got %f", stepSize, curVal, result)
			}
		}
	})
}

func TestAdvanceThermalUsesTheRangeMatchingItsField(t *testing.T) {
	// deliberately disjoint ranges so a field reading from the wrong map
	// is unambiguous, rather than coincidentally landing in a shared range.
	spec := gpu.GPUSpec{
		GpuTemperatureRanges: map[gpu.HealthStatus]gpu.Range64{
			gpu.StatusHealthy: {Min: 100, Max: 110},
		},
		MemoryTemperatureRanges: map[gpu.HealthStatus]gpu.Range64{
			gpu.StatusHealthy: {Min: 10, Max: 20},
		},
	}

	t.Run("FieldGPUTemp reads from GpuTemperatureRanges", func(t *testing.T) {
		state := GPUState{FailureType: gpu.FailureTypeNone, Status: gpu.StatusHealthy, Values: make(map[FieldID]float64)}
		advanceThermal(FieldGPUTemp, &state, spec)
		result := state.Values[FieldGPUTemp]
		if result < 100 || result > 110 {
			t.Fatalf("expected FieldGPUTemp to use GpuTemperatureRanges [100,110], got %f", result)
		}
	})

	t.Run("FieldMemTemp reads from MemoryTemperatureRanges", func(t *testing.T) {
		state := GPUState{FailureType: gpu.FailureTypeNone, Status: gpu.StatusHealthy, Values: make(map[FieldID]float64)}
		advanceThermal(FieldMemTemp, &state, spec)
		result := state.Values[FieldMemTemp]
		if result < 10 || result > 20 {
			t.Fatalf("expected FieldMemTemp to use MemoryTemperatureRanges [10,20], got %f", result)
		}
	})
}

func TestAdvanceThermalIgnoresStatusWhenNotImplicated(t *testing.T) {
	spec := gpu.GPUSpec{
		GpuTemperatureRanges: map[gpu.HealthStatus]gpu.Range64{
			gpu.StatusHealthy:  {Min: 40, Max: 75},
			gpu.StatusCritical: {Min: 84, Max: 95},
		},
	}

	// GPU status is Critical, but because of a Power failure, not a thermal
	// one -- GPU temp should still report healthy-range values.
	state := GPUState{
		FailureType: gpu.FailureTypePower,
		Status:      gpu.StatusCritical,
		Values:      make(map[FieldID]float64),
	}
	advanceThermal(FieldGPUTemp, &state, spec)

	result := state.Values[FieldGPUTemp]
	healthy := spec.GpuTemperatureRanges[gpu.StatusHealthy]
	if result < healthy.Min || result > healthy.Max {
		t.Fatalf("expected FieldGPUTemp to stay in the healthy range [%f,%f] since a Power failure doesn't implicate temperature, got %f", healthy.Min, healthy.Max, result)
	}
}

func TestAdvanceThermalIgnoresUnrelatedFieldID(t *testing.T) {
	state := GPUState{
		FailureType: gpu.FailureTypeNone,
		Status:      gpu.StatusHealthy,
		Values:      map[FieldID]float64{FieldPowerUsage: 42},
	}
	advanceThermal(FieldPowerUsage, &state, gpu.GPUSpec{})

	if got := state.Values[FieldPowerUsage]; got != 42 {
		t.Fatalf("expected advanceThermal to leave an unrelated field untouched, got %f", got)
	}
}

func TestAdvanceValues(t *testing.T) {
	t.Run("sets memory temp, gpu temp, and power usage within spec for each state", func(t *testing.T) {
		states := []GPUState{
			{EntityID: 1, Model: gpu.ModelH100, Status: gpu.StatusHealthy, FailureType: gpu.FailureTypeNone, Values: make(map[FieldID]float64)},
			{EntityID: 2, Model: gpu.ModelA30, Status: gpu.StatusCritical, FailureType: gpu.FailureTypePower, Values: make(map[FieldID]float64)},
		}

		AdvanceValues(states)

		for _, s := range states {
			spec, err := gpu.SpecForModel(s.Model)
			if err != nil {
				t.Fatalf("unexpected error getting spec for %s: %v", s.Model, err)
			}

			memTemp, ok := s.Values[FieldMemTemp]
			if !ok {
				t.Errorf("entity %d: expected FieldMemTemp to be set", s.EntityID)
			}
			healthyMemRange := spec.MemoryTemperatureRanges[gpu.StatusHealthy]
			if memTemp < healthyMemRange.Min || memTemp > healthyMemRange.Max {
				t.Errorf("entity %d: expected mem temp in healthy range [%f,%f] (not thermally failing), got %f", s.EntityID, healthyMemRange.Min, healthyMemRange.Max, memTemp)
			}

			gpuTemp, ok := s.Values[FieldGPUTemp]
			if !ok {
				t.Errorf("entity %d: expected FieldGPUTemp to be set", s.EntityID)
			}
			healthyGpuRange := spec.GpuTemperatureRanges[gpu.StatusHealthy]
			if gpuTemp < healthyGpuRange.Min || gpuTemp > healthyGpuRange.Max {
				t.Errorf("entity %d: expected gpu temp in healthy range [%f,%f] (not thermally failing), got %f", s.EntityID, healthyGpuRange.Min, healthyGpuRange.Max, gpuTemp)
			}

			power, ok := s.Values[FieldPowerUsage]
			if !ok {
				t.Errorf("entity %d: expected FieldPowerUsage to be set", s.EntityID)
			}
			expectedPowerStatus := gpu.StatusHealthy
			if s.FailureType == gpu.FailureTypePower {
				expectedPowerStatus = s.Status
			}
			expectedPowerRange := spec.Power[expectedPowerStatus]
			if power < expectedPowerRange.Min || power > expectedPowerRange.Max {
				t.Errorf("entity %d: expected power in range [%f,%f], got %f", s.EntityID, expectedPowerRange.Min, expectedPowerRange.Max, power)
			}
		}
	})

	t.Run("skips states with an unrecognized model without panicking", func(t *testing.T) {
		states := []GPUState{
			{EntityID: 1, Model: "NotARealModel", Status: gpu.StatusHealthy, FailureType: gpu.FailureTypeNone, Values: make(map[FieldID]float64)},
		}

		AdvanceValues(states)

		if len(states[0].Values) != 0 {
			t.Errorf("expected values to be left untouched for an unrecognized model, got %v", states[0].Values)
		}
	})

	t.Run("advances each state independently using its own model's spec", func(t *testing.T) {
		states := []GPUState{
			{EntityID: 1, Model: gpu.ModelH100, Status: gpu.StatusHealthy, FailureType: gpu.FailureTypeNone, Values: make(map[FieldID]float64)},
			{EntityID: 2, Model: gpu.ModelA30, Status: gpu.StatusHealthy, FailureType: gpu.FailureTypeNone, Values: make(map[FieldID]float64)},
		}

		AdvanceValues(states)

		h100Spec, _ := gpu.SpecForModel(gpu.ModelH100)
		a30Spec, _ := gpu.SpecForModel(gpu.ModelA30)

		h100Power := states[0].Values[FieldPowerUsage]
		a30Power := states[1].Values[FieldPowerUsage]

		h100Range := h100Spec.Power[gpu.StatusHealthy]
		a30Range := a30Spec.Power[gpu.StatusHealthy]

		if h100Power < h100Range.Min || h100Power > h100Range.Max {
			t.Errorf("expected H100 entity's power to be drawn from the H100 spec [%f,%f], got %f", h100Range.Min, h100Range.Max, h100Power)
		}
		if a30Power < a30Range.Min || a30Power > a30Range.Max {
			t.Errorf("expected A30 entity's power to be drawn from the A30 spec [%f,%f], got %f", a30Range.Min, a30Range.Max, a30Power)
		}
		// H100 and A30 healthy power ranges don't overlap (400-650 vs 80-145),
		// so either value landing in the other's range would prove
		// cross-contamination between states in the same slice.
		if h100Power >= a30Range.Min && h100Power <= a30Range.Max {
			t.Errorf("H100 entity's power (%f) fell inside the A30 range -- possible state cross-contamination", h100Power)
		}
	})
}
