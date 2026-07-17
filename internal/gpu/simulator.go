package gpu

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

const (
	TotalGpus   uint16 = 10000
	GpusPerNode uint16 = 10
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

func GetHealth(_ context.Context, gpuID string) (*GPUHealth, error) {
	var id uint16
	_, err := fmt.Sscanf(gpuID, "GPU-%d", &id)

	if err != nil {
		return nil, err
	}

	if id > (TotalGpus) || id < 1 {
		return nil, fmt.Errorf("Invalid GPU ID: ID must be between 1 - %d", TotalGpus)
	}

	nodeID := ((id - 1) / GpusPerNode) + 1
	slot := (id - 1) % GpusPerNode

	// Pick a model based on the id range
	// 1-2000 		-> H100
	// 2001-5000	-> A100
	// 5001-7000	-> V100
	// 7001-10000	-> A30
	var model string
	switch {
	case id < 2001:
		model = "H100"
	case id < 5001:
		model = "A100"
	case id < 7001:
		model = "V100"
	case id < 10001:
		model = "A30"
	}

	status, ok := DefaultStore.GetStatus(gpuID)
	if !ok {
		return nil, fmt.Errorf("cannot find gpu %s in store", gpuID)
	}

	var gpuUtilization float64
	var memoryUtilization float64
	var memoryEccSingleBitErrors uint8
	var memoryEccDoubleBitErrors uint8
	switch status {
	case StatusCritical:
		gpuUtilization = rand.Float64() * 30            // 0-30%, gpu may be stuck
		memoryUtilization = (rand.Float64() * 10) + 90  // 90-100%, memory leak
		memoryEccSingleBitErrors = uint8(rand.IntN(20)) // 0-20 single bit errors
		memoryEccDoubleBitErrors = uint8(rand.IntN(3))  // 0-3 double bit errors
	case StatusWarning:
		gpuUtilization = (rand.Float64() * 10) + 90    // 90-100%, gpu is pegged
		memoryUtilization = (rand.Float64() * 20) + 70 // 70-90%, memory pressure
		memoryEccSingleBitErrors = uint8(rand.IntN(5)) // 0-5 single bit errors
		memoryEccDoubleBitErrors = 0                   // 0 double bit errors
	default:
		gpuUtilization = (rand.Float64() * 50) + 40    // 40-90%, normal load
		memoryUtilization = (rand.Float64() * 50) + 20 // 20-70%, normal memory load
		memoryEccSingleBitErrors = 0                   // 0 single bit errors
		memoryEccDoubleBitErrors = 0                   // 0 double bit errors
	}

	spec, err := SpecForModel(model)
	if err != nil {
		return nil, err
	}

	temperature := new(Temperature)
	tMin, tMax := spec.temperature[status].Min, spec.temperature[status].Max
	temperature.GPUCoreCelsius = tMin + (rand.Float64() * (tMax - tMin))
	temperature.MemoryCelsius = temperature.GPUCoreCelsius - (10 + rand.Float64()*5)
	temperature.GPUCoreWarningThreshold = spec.temperature[StatusWarning].Min
	temperature.GPUCoreCriticalThreshold = spec.temperature[StatusCritical].Min
	temperature.MemoryWarningThreshold = 85.0
	temperature.MemoryCriticalThreshold = 95.0
	temperature.Throttling = temperature.GPUCoreCelsius >= temperature.GPUCoreWarningThreshold ||
		temperature.MemoryCelsius >= temperature.MemoryWarningThreshold

	memory := new(Memory)
	memory.TotalBytes = spec.memoryBytes
	memory.UsedBytes = uint64(float64(spec.memoryBytes) * memoryUtilization * .01)
	memory.FreeBytes = memory.TotalBytes - memory.UsedBytes
	memory.Utilization = memoryUtilization
	memory.ECCSingleBitErrors = memoryEccSingleBitErrors
	memory.ECCDoubleBitErrors = memoryEccDoubleBitErrors

	power := new(Power)
	pMin, pMax := spec.power[status].Min, spec.power[status].Max
	power.DrawWatts = pMin + (rand.Float64() * (pMax - pMin))
	power.LimitWatts = spec.maxPowerWatts
	power.Utilization = power.DrawWatts / power.LimitWatts * 100
	power.PowerCapped = power.DrawWatts >= power.LimitWatts

	gpuHealth := new(GPUHealth)
	gpuHealth.GPUID = gpuID
	gpuHealth.NodeID = fmt.Sprintf("NODE-%04d", nodeID)
	gpuHealth.Slot = slot
	gpuHealth.Model = model
	gpuHealth.HealthStatus = status
	gpuHealth.Timestamp = time.Now().UTC()
	gpuHealth.Utilization = gpuUtilization
	gpuHealth.Temperature = *temperature
	gpuHealth.Memory = *memory
	gpuHealth.Power = *power

	return gpuHealth, nil
}
