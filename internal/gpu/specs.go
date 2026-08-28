package gpu

import (
	"fmt"
)

// ranges will have an overlap since they are floats
type Range64 struct {
	Min float64
	Max float64
}

var gpuTempRanges = map[HealthStatus]Range64{
	StatusHealthy:  {Min: 40.0, Max: 75.0},
	StatusWarning:  {Min: 75.0, Max: 84.0},
	StatusCritical: {Min: 84.0, Max: 95.0},
}

var memoryTempRanges = map[HealthStatus]Range64{
	StatusHealthy:  {Min: 35.0, Max: 70.0},
	StatusWarning:  {Min: 70.0, Max: 80.0},
	StatusCritical: {Min: 80.0, Max: 90.0},
}

type GPUSpec struct {
	Model         string
	MaxPowerWatts float64
	MemoryBytes   uint64

	// ranges indexed by 0=healthy, 1=warning, 2=critical
	Power                   map[HealthStatus]Range64
	GpuTemperatureRanges    map[HealthStatus]Range64
	MemoryTemperatureRanges map[HealthStatus]Range64
}

const GB = 1024 * 1024 * 1024

var gpuSpecs = map[string]GPUSpec{
	ModelH100: {
		Model:         ModelH100,
		MaxPowerWatts: 700.0,
		MemoryBytes:   80 * GB,
		Power: map[HealthStatus]Range64{
			StatusHealthy:  {Min: 400.0, Max: 650.0},
			StatusWarning:  {Min: 650.0, Max: 680.0},
			StatusCritical: {Min: 680.0, Max: 700.0},
		},
		GpuTemperatureRanges:    gpuTempRanges,
		MemoryTemperatureRanges: memoryTempRanges,
	},
	ModelA100: {
		Model:         ModelA100,
		MaxPowerWatts: 400.0,
		MemoryBytes:   80 * GB,
		Power: map[HealthStatus]Range64{
			StatusHealthy:  {Min: 200.0, Max: 350.0},
			StatusWarning:  {Min: 350.0, Max: 370.0},
			StatusCritical: {Min: 370.0, Max: 400.0},
		},
		GpuTemperatureRanges:    gpuTempRanges,
		MemoryTemperatureRanges: memoryTempRanges,
	},
	ModelV100: {
		Model:         ModelV100,
		MaxPowerWatts: 300.0,
		MemoryBytes:   32 * GB,
		Power: map[HealthStatus]Range64{
			StatusHealthy:  {Min: 150.0, Max: 270.0},
			StatusWarning:  {Min: 270.0, Max: 290.0},
			StatusCritical: {Min: 290.0, Max: 300.0},
		},
		GpuTemperatureRanges:    gpuTempRanges,
		MemoryTemperatureRanges: memoryTempRanges,
	},
	ModelA30: {
		Model:         ModelA30,
		MaxPowerWatts: 165.0,
		MemoryBytes:   24 * GB,
		Power: map[HealthStatus]Range64{
			StatusHealthy:  {Min: 80.0, Max: 145.0},
			StatusWarning:  {Min: 145.0, Max: 160.0},
			StatusCritical: {Min: 160.0, Max: 165.0},
		},
		GpuTemperatureRanges:    gpuTempRanges,
		MemoryTemperatureRanges: memoryTempRanges,
	},
}

func SpecForModel(model string) (GPUSpec, error) {
	gpuSpec, ok := gpuSpecs[model]

	if !ok {
		return GPUSpec{}, fmt.Errorf("%s not found in GPU specs", model)
	}

	return gpuSpec, nil
}
