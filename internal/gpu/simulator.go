package gpu

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

var ErrGPUNotFound = errors.New("gpu not found")
var ErrGPUUnrecoverable = errors.New("gpu requires hardware replacement")

const (
	TotalGpus   uint16 = 10000
	GpusPerNode uint16 = 4
)

var gpuIDs []string

func init() {
	gpuIDs = make([]string, TotalGpus)
	for i := range TotalGpus {
		gpuIDs[i] = fmt.Sprintf("GPU-%05d", i+1)
	}

	DefaultStore = NewStore(gpuIDs)
}

func AllIDs() []string {
	return gpuIDs
}

func randomWarningFailure() FailureType {
	switch r := rand.Float64(); {
	case r < 0.50:
		return FailureTypeThermal
	case r < 0.80:
		return FailureTypePower
	default:
		return FailureTypeECCSingle
	}
}

// DegradeToWarning transitions a GPU from Healthy to Warning with a randomly
// chosen failure type. Thermal is most common, power second, ECC single-bit last.
func DegradeToWarning(gpuID string) {
	DefaultStore.SetState(gpuID, StatusWarning, randomWarningFailure())
	DefaultHub.Publish(gpuID)
}

// WorsenToCritical transitions a GPU from Warning to Critical. In rare cases
// (5%) an uncorrectable ECC double-bit error develops regardless of the existing
// failure type; otherwise the failure type carries forward unchanged.
func WorsenToCritical(gpuID string) {
	_, failure, ok := DefaultStore.GetState(gpuID)
	if !ok {
		return
	}
	if rand.Float64() < 0.05 {
		failure = FailureTypeECCDouble
	}
	DefaultStore.SetState(gpuID, StatusCritical, failure)
	DefaultHub.Publish(gpuID)
}

// RecoverToHealthy transitions a GPU from Warning to Healthy. ECC single-bit
// errors cannot self-heal and are silently skipped.
func RecoverToHealthy(gpuID string) {
	_, failure, ok := DefaultStore.GetState(gpuID)
	if !ok || failure == FailureTypeECCSingle {
		return
	}
	DefaultStore.SetState(gpuID, StatusHealthy, FailureTypeNone)
	DefaultHub.Publish(gpuID)
}

// StepBackToWarning transitions a GPU from Critical to Warning. ECC double-bit
// errors are hardware failures that cannot step back on their own.
func StepBackToWarning(gpuID string) {
	_, failure, ok := DefaultStore.GetState(gpuID)
	if !ok || failure == FailureTypeECCDouble {
		return
	}
	DefaultStore.SetState(gpuID, StatusWarning, failure)
	DefaultHub.Publish(gpuID)
}

// Recover simulates a GPU returning to service after a node drain.
// Thermal and power failures resolve with drain. ECC single-bit errors persist
// (the GPU returns to Warning). ECC double-bit errors are unrecoverable via drain.
func Recover(gpuID string) error {
	_, failure, ok := DefaultStore.GetState(gpuID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrGPUNotFound, gpuID)
	}

	switch failure {
	case FailureTypeECCDouble:
		return fmt.Errorf("%w: %s has unrecoverable ECC double-bit errors", ErrGPUUnrecoverable, gpuID)
	case FailureTypeECCSingle:
		// Drain completes, but ECC errors persist. GPU returns to service at Warning.
		DefaultStore.SetState(gpuID, StatusWarning, FailureTypeECCSingle)
		DefaultHub.Publish(gpuID)
		return nil
	default:
		// Thermal and power issues resolve when workloads are drained.
		newStatus := StatusHealthy
		newFailure := FailureTypeNone
		if rand.Float64() < Config.Get().RecoveryWarningRate {
			newStatus = StatusWarning
			newFailure = failure
		}
		DefaultStore.SetState(gpuID, newStatus, newFailure)
		DefaultHub.Publish(gpuID)
		return nil
	}
}

// Replace simulates a physical GPU hardware replacement. Fresh hardware almost always
// starts healthy, but a configurable fraction arrive with a defect already present.
func Replace(gpuID string) error {
	if _, _, ok := DefaultStore.GetState(gpuID); !ok {
		return fmt.Errorf("%w: %s", ErrGPUNotFound, gpuID)
	}
	newStatus := StatusHealthy
	newFailure := FailureTypeNone
	if rand.Float64() < Config.Get().ReplacementWarningRate {
		newStatus = StatusWarning
		newFailure = randomWarningFailure()
	}
	DefaultStore.SetState(gpuID, newStatus, newFailure)
	DefaultHub.Publish(gpuID)
	return nil
}

func GetHealth(_ context.Context, gpuID string) (*GPUHealth, error) {
	var id uint16
	_, err := fmt.Sscanf(gpuID, "GPU-%d", &id)

	if err != nil {
		return nil, err
	}

	if id > (TotalGpus) || id < 1 {
		return nil, fmt.Errorf("invalid GPU ID: must be between 1 and %d", TotalGpus)
	}

	nodeID := ((id - 1) / GpusPerNode) + 1
	slot := (id - 1) % GpusPerNode

	var model string
	switch {
	case id < 2001:
		model = ModelH100
	case id < 5001:
		model = ModelA100
	case id < 7001:
		model = ModelV100
	case id < 10001:
		model = ModelA30
	}

	status, failure, ok := DefaultStore.GetState(gpuID)
	if !ok {
		return nil, fmt.Errorf("cannot find gpu %s in store", gpuID)
	}

	spec, err := SpecForModel(model)
	if err != nil {
		return nil, err
	}

	// Temperature is driven by the failure type, not just status.
	// Thermal failures produce high temps; other failures run at normal temps.
	tempStatus := StatusHealthy
	if failure == FailureTypeThermal {
		tempStatus = status
	}
	temperature := new(Temperature)
	tMin, tMax := spec.temperature[tempStatus].Min, spec.temperature[tempStatus].Max
	temperature.GPUCoreCelsius = tMin + (rand.Float64() * (tMax - tMin))
	temperature.MemoryCelsius = temperature.GPUCoreCelsius - (10 + rand.Float64()*5)
	temperature.GPUCoreWarningThreshold = spec.temperature[StatusWarning].Min
	temperature.GPUCoreCriticalThreshold = spec.temperature[StatusCritical].Min
	temperature.MemoryWarningThreshold = 85.0
	temperature.MemoryCriticalThreshold = 95.0
	temperature.Throttling = failure == FailureTypeThermal

	// Power draw is elevated for power-cap failures; normal otherwise.
	powerStatus := StatusHealthy
	if failure == FailureTypePower {
		powerStatus = status
	}
	power := new(Power)
	pMin, pMax := spec.power[powerStatus].Min, spec.power[powerStatus].Max
	power.DrawWatts = pMin + (rand.Float64() * (pMax - pMin))
	power.LimitWatts = spec.maxPowerWatts
	power.Utilization = power.DrawWatts / power.LimitWatts * 100
	power.PowerCapped = failure == FailureTypePower

	// ECC errors and utilization are failure-type-specific.
	var gpuUtilization float64
	var memoryUtilization float64
	var memoryEccSingleBitErrors uint8
	var memoryEccDoubleBitErrors uint8

	switch failure {
	case FailureTypeThermal:
		if status == StatusCritical {
			gpuUtilization = rand.Float64() * 20           // 0-20%, throttled to a crawl
			memoryUtilization = (rand.Float64() * 20) + 40 // 40-60%
		} else {
			gpuUtilization = (rand.Float64() * 30) + 50    // 50-80%, starting to throttle
			memoryUtilization = (rand.Float64() * 20) + 40 // 40-60%
		}
	case FailureTypePower:
		if status == StatusCritical {
			gpuUtilization = rand.Float64() * 30           // 0-30%, hard capped
			memoryUtilization = (rand.Float64() * 20) + 50 // 50-70%
		} else {
			gpuUtilization = (rand.Float64() * 10) + 80    // 80-90%, pegged at cap
			memoryUtilization = (rand.Float64() * 20) + 50 // 50-70%
		}
	case FailureTypeECCSingle:
		gpuUtilization = (rand.Float64() * 10) + 80         // 80-90%, still running
		memoryUtilization = (rand.Float64() * 20) + 60      // 60-80%
		memoryEccSingleBitErrors = uint8(rand.IntN(10) + 1) // 1-10 correctable errors
	case FailureTypeECCDouble:
		gpuUtilization = rand.Float64() * 30               // 0-30%, effectively failed
		memoryUtilization = (rand.Float64() * 10) + 90     // 90-100%, memory corrupted
		memoryEccSingleBitErrors = uint8(rand.IntN(20))    // many single-bit errors too
		memoryEccDoubleBitErrors = uint8(rand.IntN(3) + 1) // 1-3 uncorrectable errors
	default:
		gpuUtilization = (rand.Float64() * 50) + 40    // 40-90%, normal load
		memoryUtilization = (rand.Float64() * 50) + 20 // 20-70%, normal memory load
	}

	memory := new(Memory)
	memory.TotalBytes = spec.memoryBytes
	memory.UsedBytes = uint64(float64(spec.memoryBytes) * memoryUtilization * .01)
	memory.FreeBytes = memory.TotalBytes - memory.UsedBytes
	memory.Utilization = memoryUtilization
	memory.ECCSingleBitErrors = memoryEccSingleBitErrors
	memory.ECCDoubleBitErrors = memoryEccDoubleBitErrors

	gpuHealth := new(GPUHealth)
	gpuHealth.GPUID = gpuID
	gpuHealth.NodeID = fmt.Sprintf("NODE-%04d", nodeID)
	gpuHealth.Slot = slot
	gpuHealth.Model = model
	gpuHealth.HealthStatus = status
	gpuHealth.FailureType = failure
	gpuHealth.Timestamp = time.Now().UTC()
	gpuHealth.Utilization = gpuUtilization
	gpuHealth.Temperature = *temperature
	gpuHealth.Memory = *memory
	gpuHealth.Power = *power

	return gpuHealth, nil
}
