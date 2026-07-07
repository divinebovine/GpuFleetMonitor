package gpu

import (
	"fmt"
)

// ranges will have an overlap since they are floats
type range64 struct {
	Min float64
	Max float64
}

var defaultTempRanges = map[HealthStatus]range64{
	StatusHealthy:  {Min: 40.0, Max: 75.0},
	StatusWarning:  {Min: 75.0, Max: 84.0},
	StatusCritical: {Min: 84.0, Max: 95.0},
}

type GPUSpec struct {
	model         string
	maxPowerWatts float64
	memoryBytes   uint64

	// ranges indexed by 0=healthy, 1=warning, 2=critical
	power       map[HealthStatus]range64
	temperature map[HealthStatus]range64
}

const GB = 1024 * 1024 * 1024

var gpuSpecs = map[string]GPUSpec{
	"H100": {
		model:         "H100",
		maxPowerWatts: 700.0,
		memoryBytes:   80 * GB,
		power: map[HealthStatus]range64{
			StatusHealthy:  {Min: 400.0, Max: 650.0},
			StatusWarning:  {Min: 650.0, Max: 680.0},
			StatusCritical: {Min: 680.0, Max: 700.0},
		},
		temperature: defaultTempRanges,
	},
	"A100": {
		model:         "A100",
		maxPowerWatts: 400.0,
		memoryBytes:   80 * GB,
		power: map[HealthStatus]range64{
			StatusHealthy:  {Min: 200.0, Max: 350.0},
			StatusWarning:  {Min: 350.0, Max: 370.0},
			StatusCritical: {Min: 370.0, Max: 400.0},
		},
		temperature: defaultTempRanges,
	},
	"V100": {
		model:         "V100",
		maxPowerWatts: 300.0,
		memoryBytes:   32 * GB,
		power: map[HealthStatus]range64{
			StatusHealthy:  {Min: 150.0, Max: 270.0},
			StatusWarning:  {Min: 270.0, Max: 290.0},
			StatusCritical: {Min: 290.0, Max: 300.0},
		},
		temperature: defaultTempRanges,
	},
	"A30": {
		model:         "A30",
		maxPowerWatts: 165.0,
		memoryBytes:   24 * GB,
		power: map[HealthStatus]range64{
			StatusHealthy:  {Min: 80.0, Max: 145.0},
			StatusWarning:  {Min: 145.0, Max: 160.0},
			StatusCritical: {Min: 160.0, Max: 165.0},
		},
		temperature: defaultTempRanges,
	},
}

func SpecForModel(model string) (GPUSpec, error) {
	gpuSpec, ok := gpuSpecs[model]

	if !ok {
		return GPUSpec{}, fmt.Errorf("%s not found in GPU specs", model)
	}

	return gpuSpec, nil
}
