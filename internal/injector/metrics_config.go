package injector

import "github.com/prometheus/client_golang/prometheus"

type metricConfig struct {
	fieldID   uint16
	desc      *prometheus.Desc
	valueType prometheus.ValueType
}

const (
	labelGPU       = "gpu"
	labelModelName = "modelName"
)

var metricsConfig = []metricConfig{
	{
		140,
		prometheus.V2.NewDesc(
			"DCGM_FI_DEV_MEMORY_TEMP_CELSIUS",
			"Current memory temperature",
			prometheus.UnconstrainedLabels([]string{labelGPU, labelModelName}),
			nil,
			prometheus.WithUnit("celsius")),
		prometheus.GaugeValue,
	},
	{
		150,
		prometheus.V2.NewDesc(
			"DCGM_FI_DEV_GPU_TEMP",
			"Current GPU temperature",
			prometheus.UnconstrainedLabels([]string{labelGPU, labelModelName}),
			nil,
			prometheus.WithUnit("celsius")),
		prometheus.GaugeValue,
	},
	{
		155,
		prometheus.V2.NewDesc(
			"DCGM_FI_DEV_BOARD_POWER_WATTS",
			"Current power usage in watts",
			prometheus.UnconstrainedLabels([]string{labelGPU, labelModelName}),
			nil,
			prometheus.WithUnit("watts")),
		prometheus.GaugeValue,
	},
	{
		310,
		prometheus.V2.NewDesc(
			"DCGM_FI_DEV_ECC_SBE_VOL_TOTAL",
			"Total single bit volatile ECC errors",
			prometheus.UnconstrainedLabels([]string{labelGPU, labelModelName}),
			nil),
		prometheus.CounterValue,
	},
	{
		311,
		prometheus.V2.NewDesc(
			"DCGM_FI_DEV_ECC_DBE_VOL_TOTAL",
			"Total double bit volatile ECC errors",
			prometheus.UnconstrainedLabels([]string{labelGPU, labelModelName}),
			nil),
		prometheus.CounterValue,
	},
}

func initValues(config []metricConfig) map[uint16]map[uint]float64 {
	out := make(map[uint16]map[uint]float64)
	for _, c := range config {
		out[c.fieldID] = make(map[uint]float64)
	}
	return out
}

func initDescs(config []metricConfig) map[uint16]*prometheus.Desc {
	out := make(map[uint16]*prometheus.Desc)
	for _, c := range config {
		out[c.fieldID] = c.desc
	}
	return out
}

func initValueTypes(config []metricConfig) map[uint16]prometheus.ValueType {
	out := make(map[uint16]prometheus.ValueType)
	for _, c := range config {
		out[c.fieldID] = c.valueType
	}
	return out
}
