package injector

import "github.com/prometheus/client_golang/prometheus"

type FieldID uint16

const (
	FieldMemTemp    FieldID = 140
	FieldGPUTemp    FieldID = 150
	FieldPowerUsage FieldID = 155
	FieldPowerLimit FieldID = 164
	FieldECCSingle  FieldID = 310
	FieldECCDouble  FieldID = 311

	MemoryTempName = "DCGM_FI_DEV_MEMORY_TEMP_CELSIUS"
	GPUTempName    = "DCGM_FI_DEV_GPU_TEMP"
	PowerUsageName = "DCGM_FI_DEV_BOARD_POWER_WATTS"
	PowerLimitName = "DCGM_FI_DEV_BOARD_POWER_LIMIT_ENFORCED_WATTS"
	ECCSingleName  = "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL"
	ECCDoubleName  = "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL"

	MemoryTempDesc = "Current memory temperature"
	GPUTempDesc    = "Current GPU temperature"
	PowerUsageDesc = "Current power usage in watts"
	PowerLimitDesc = "Effective power limit enforced for the device"
	ECCSingleDesc  = "Total single bit volatile ECC errors"
	ECCDoubleDesc  = "Total double bit volatile ECC errors"

	Celsius = "celsius"
	Watts   = "watts"

	LabelGPU       = "gpu"
	LabelModelName = "modelName"
)

type simMetric struct {
	fieldID   FieldID
	desc      *prometheus.Desc
	valueType prometheus.ValueType
}

var defaultLabels = prometheus.UnconstrainedLabels([]string{LabelGPU, LabelModelName})

var simMetrics = []simMetric{
	{
		FieldMemTemp,
		prometheus.V2.NewDesc(
			MemoryTempName,
			MemoryTempDesc,
			defaultLabels,
			nil,
			prometheus.WithUnit(Celsius)),
		prometheus.GaugeValue,
	},
	{
		FieldGPUTemp,
		prometheus.V2.NewDesc(
			GPUTempName,
			GPUTempDesc,
			defaultLabels,
			nil,
			prometheus.WithUnit(Celsius)),
		prometheus.GaugeValue,
	},
	{
		FieldPowerUsage,
		prometheus.V2.NewDesc(
			PowerUsageName,
			PowerUsageDesc,
			defaultLabels,
			nil,
			prometheus.WithUnit("watts")),
		prometheus.GaugeValue,
	}, {
		FieldPowerLimit,
		prometheus.V2.NewDesc(
			PowerLimitName,
			PowerLimitDesc,
			defaultLabels,
			nil,
			prometheus.WithUnit("watts")),
		prometheus.GaugeValue,
	},

	{
		FieldECCSingle,
		prometheus.V2.NewDesc(
			ECCSingleName,
			ECCSingleDesc,
			defaultLabels,
			nil),
		prometheus.CounterValue,
	},
	{
		FieldECCDouble,
		prometheus.V2.NewDesc(
			ECCDoubleName,
			ECCDoubleDesc,
			defaultLabels,
			nil),
		prometheus.CounterValue,
	},
}
