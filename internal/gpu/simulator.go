package gpu

import (
	"fmt"
	"hash/fnv"
	"math/rand"
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
}

func AllIDs() []string {
	return gpuIDs
}

func GetHealth(gpuID string) (*GPUHealth, error) {
	// extract gpu id as an integer
	var id uint16
	_, err := fmt.Sscanf(gpuID, "GPU-%d", &id)

	if err != nil {
		return nil, err
	}

	// sanity check
	if id > (TotalGpus) || id < 1 {
		return nil, fmt.Errorf("Invalid GPU ID: ID must be between 1 - %d", TotalGpus)
	}

	// Use the hash of the gpu id to generate a seed for random number generation
	hash := fnv.New32a()
	hash.Write([]byte(gpuID))
	seed := hash.Sum32()
	rng := rand.New(rand.NewSource(int64(seed)))

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

	// Use the seed, based on the gpu id, to deterministically set the health status
	// and additional values that correlate with the health status
	statusRoll := seed % 100
	var healthStatus HealthStatus
	var gpuUtilization float64
	var memoryUtilization float64
	var memoryEccSingleBitErrors uint8
	var memoryEccDoubleBitErrors uint8
	switch {
	case statusRoll <= 4:
		// critial
		healthStatus = StatusCritical
		gpuUtilization = rng.Float64() * 30            // 0-30%, gpu may be stuck
		memoryUtilization = (rng.Float64() * 10) + 90  // 90-100%, memory leak
		memoryEccSingleBitErrors = uint8(rng.Intn(20)) // 0-20 single bit errors
		memoryEccDoubleBitErrors = uint8(rng.Intn(3))  // 0-3 double bit errors
	case statusRoll >= 5 && statusRoll <= 14:
		// warning
		healthStatus = StatusWarning
		gpuUtilization = (rng.Float64() * 10) + 90    // 90-100%, gpu is pegged
		memoryUtilization = (rng.Float64() * 20) + 70 // 70-90%, memory pressure
		memoryEccSingleBitErrors = uint8(rng.Intn(5)) // 0-5 single bit errors
		memoryEccDoubleBitErrors = 0                  // 0 double bit errors
	default:
		// healthy
		healthStatus = StatusHealthy
		gpuUtilization = (rng.Float64() * 50) + 40    // 40-90%, normal load
		memoryUtilization = (rng.Float64() * 50) + 20 // 20-70%, normal memory load
		memoryEccSingleBitErrors = 0                  // 0 single bit errors
		memoryEccDoubleBitErrors = 0                  // 0 double bit errors
	}

	// Get the specs for the model
	spec, err := SpecForModel(model)
	if err != nil {
		return nil, err
	}

	temperature := new(Temperature)
	tMin, tMax := spec.temperature[healthStatus].Min, spec.temperature[healthStatus].Max
	temperature.GPUCoreCelsius = tMin + (rng.Float64() * (tMax - tMin))
	temperature.MemoryCelsius = temperature.GPUCoreCelsius - (10 + rng.Float64()*5)
	temperature.WarningThreshold = spec.temperature[StatusWarning].Min
	temperature.CriticalThreshold = spec.temperature[StatusCritical].Min
	temperature.Throttling = temperature.GPUCoreCelsius >= temperature.WarningThreshold

	memory := new(Memory)
	memory.TotalBytes = spec.memoryBytes
	memory.UsedBytes = uint64(float64(spec.memoryBytes) * memoryUtilization * .01)
	memory.FreeBytes = memory.TotalBytes - memory.UsedBytes
	memory.Utilization = memoryUtilization
	memory.ECCSingleBitErrors = memoryEccSingleBitErrors
	memory.ECCDoubleBitErrors = memoryEccDoubleBitErrors

	power := new(Power)
	pMin, pMax := spec.power[healthStatus].Min, spec.power[healthStatus].Max
	power.DrawWatts = pMin + (rng.Float64() * (pMax - pMin))
	power.LimitWatts = spec.maxPowerWatts
	power.Utilization = power.DrawWatts / power.LimitWatts * 100
	power.PowerCapped = power.DrawWatts >= power.LimitWatts

	gpuHealth := new(GPUHealth)
	gpuHealth.GPUID = gpuID
	gpuHealth.NodeID = fmt.Sprintf("NODE-%04d", nodeID)
	gpuHealth.Slot = slot
	gpuHealth.Model = model
	gpuHealth.HealthStatus = healthStatus
	gpuHealth.Timestamp = time.Now().UTC()
	gpuHealth.Utilization = gpuUtilization
	gpuHealth.Temperature = *temperature
	gpuHealth.Memory = *memory
	gpuHealth.Power = *power

	return gpuHealth, nil
}
