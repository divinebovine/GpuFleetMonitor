package injector

import (
	"math/rand/v2"

	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

const (
	ThermalStepSize = 5.0 // 5 degrees celsius per tick
)

// Thermal Fuzz varies step size for a more realistic appearance
var ThermalFuzz = gpu.Range64{Min: -0.7, Max: 0.7}

func AdvanceValues(states []GPUState) {
	for i := range states {
		spec, err := gpu.SpecForModel(states[i].Model)
		if err != nil {
			continue
		}

		advanceThermal(FieldMemTemp, &states[i], spec)
		advanceThermal(FieldGPUTemp, &states[i], spec)
		advancePowerUsage(&states[i], spec)
	}
}

func advanceThermal(fieldID FieldID, state *GPUState, spec gpu.GPUSpec) {
	// thermal values do not jump instantaneously, they ramp up or down
	var failureType gpu.FailureType
	var tempRanges map[gpu.HealthStatus]gpu.Range64
	switch fieldID {
	case FieldMemTemp:
		failureType = gpu.FailureTypeMemoryThermal
		tempRanges = spec.MemoryTemperatureRanges
	case FieldGPUTemp:
		failureType = gpu.FailureTypeGPUThermal
		tempRanges = spec.GpuTemperatureRanges
	default:
		// do nothing for now
		return
	}

	if state.FailureType != failureType {
		// use healthy values if the failure type does not match what is being advanced
		rng := tempRanges[gpu.StatusHealthy]
		state.Values[fieldID] = randomFromRange(rng)
		return
	}

	// Failing thermally, so values need to ramp to the
	// range for the health over several ticks then fluctuate within
	// that range.
	curVal := state.Values[fieldID]
	target := tempRanges[state.Status]
	state.Values[fieldID] = nextBoundedStep(curVal, target, ThermalStepSize, ThermalFuzz)
}

func advancePowerUsage(state *GPUState, spec gpu.GPUSpec) {
	// Power jumps instantaneously
	var target gpu.Range64
	if state.FailureType != gpu.FailureTypePower {
		target = spec.Power[gpu.StatusHealthy]
	} else {
		target = spec.Power[state.Status]
	}

	state.Values[FieldPowerUsage] = randomFromRange(target)
}

func randomFromRange(r gpu.Range64) float64 {
	return r.Min + rand.Float64()*(r.Max-r.Min)
}

func nextBoundedStep(curVal float64, target gpu.Range64, stepSize float64, fuzz gpu.Range64) float64 {
	f := randomFromRange(fuzz)
	if curVal >= target.Min && curVal <= target.Max {
		stepRange := gpu.Range64{
			Min: max(target.Min, curVal-stepSize-f),
			Max: min(target.Max, curVal+stepSize+f),
		}
		return randomFromRange(stepRange)
	} else if curVal < target.Min {
		step := curVal + stepSize + f
		if step < target.Max {
			return step
		}
	} else if curVal > target.Max {
		step := curVal - stepSize - f
		if step > target.Min {
			return step
		}
	}
	return randomFromRange(target)
}
