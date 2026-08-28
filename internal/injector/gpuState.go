package injector

import "github.com/divinebovine/GpuFleetMonitor/internal/gpu"

type GPUState struct {
	EntityID    uint
	Model       string
	Status      gpu.HealthStatus
	FailureType gpu.FailureType
	Values      map[FieldID]float64 // FieldID -> current simulated value
}

// NewGPUStates builds the initial (all-Healthy) state for a node's fake
// GPUs given their entity IDs and model.
func NewGPUStates(entities []uint, model string) []GPUState {
	out := make([]GPUState, 0, len(entities))

	for _, e := range entities {
		state := GPUState{
			e,
			model,
			gpu.StatusHealthy,
			gpu.FailureTypeNone,
			make(map[FieldID]float64),
		}

		specs, err := gpu.SpecForModel(model)
		if err == nil {
			state.Values[FieldPowerLimit] = specs.MaxPowerWatts
		}

		out = append(out, state)
	}

	return out
}
