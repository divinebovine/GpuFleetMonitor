package injector

import (
	"math/rand/v2"
	"slices"

	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

// Advance applies one tick of the transition logic (in place) mutating the states,
// using the given SimulationSettings for transition rates.
// Returns the subset of states whose FailureType is a hardware failure this tick
// (i.e. a new XID/SXID-worthy event just occurred)
func Advance(states []GPUState, cfg *gpu.SimulationSettings) (justFailedHard []GPUState) {
	out := make([]GPUState, 0)

	for i := range states {
		originalFailureType := states[i].FailureType

		switch states[i].Status {
		case gpu.StatusHealthy:
			handleHealthy(&states[i], cfg)
		case gpu.StatusWarning:
			handleWarning(&states[i], cfg)
		case gpu.StatusCritical:
			handleCritical(&states[i], cfg)
		}

		if originalFailureType != states[i].FailureType &&
			slices.Contains(gpu.HardwareFailures, states[i].FailureType) {
			out = append(out, states[i])
		}
	}

	return out
}

func handleHealthy(state *GPUState, cfg *gpu.SimulationSettings) {
	if rand.Float64() < cfg.HealthyToWarningRate {
		state.Status = gpu.StatusWarning
		if rand.Float64() < cfg.PersistentWarningRate {
			state.FailureType = RandFailureType(gpu.PersistentFailures)
			return
		}

		state.FailureType = RandFailureType(gpu.TransientFailures)
	}
}

func handleWarning(state *GPUState, cfg *gpu.SimulationSettings) {
	if rand.Float64() < cfg.WarningToCriticalRate {
		state.Status = gpu.StatusCritical
		hardwareFailureRate := cfg.HardwareFailureRate
		if slices.Contains(gpu.PersistentFailures, state.FailureType) &&
			cfg.HardwareFailureRate != 0 {
			// increase the hardware failure rate because the gpu has
			// persistent failures and the hardware failure rate is
			// not explicitly set to 0
			hardwareFailureRate = min(1.0, cfg.HardwareFailureRate+0.1)
		}

		if rand.Float64() < hardwareFailureRate {
			state.FailureType = RandFailureType(gpu.HardwareFailures)
		}
		return
	}

	if rand.Float64() < cfg.WarningToHealthyRate &&
		!slices.Contains(gpu.PersistentFailures, state.FailureType) {
		state.Status = gpu.StatusHealthy
		state.FailureType = gpu.FailureTypeNone
	}
}

func handleCritical(state *GPUState, cfg *gpu.SimulationSettings) {
	if rand.Float64() < cfg.CriticalToWarningRate && !slices.Contains(gpu.HardwareFailures, state.FailureType) {
		state.Status = gpu.StatusWarning
	}
}

func RandFailureType(failures []gpu.FailureType) gpu.FailureType {
	count := len(failures)
	return failures[rand.N(count)]
}
