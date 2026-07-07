package gpu

import "time"

type HealthStatus string

const (
	StatusHealthy  HealthStatus = "healthy"
	StatusWarning  HealthStatus = "warning"
	StatusCritical HealthStatus = "critical"
)

type Temperature struct {
	GPUCoreCelsius    float64 `json:"gpu_core_celsius"`
	MemoryCelsius     float64 `json:"memory_celsius"`
	WarningThreshold  float64 `json:"warning_threshold"`
	CriticalThreshold float64 `json:"critical_threshold"`
	Throttling        bool    `json:"throttling"`
}

type Memory struct {
	TotalBytes         uint64  `json:"total_bytes"`
	UsedBytes          uint64  `json:"used_bytes"`
	FreeBytes          uint64  `json:"free_bytes"`
	Utilization        float64 `json:"utilization"`
	ECCSingleBitErrors uint8   `json:"ecc_single_bit_errors"`
	ECCDoubleBitErrors uint8   `json:"ecc_double_bit_errors"`
}

type Power struct {
	DrawWatts   float64 `json:"draw_watts"`
	LimitWatts  float64 `json:"limit_watts"`
	Utilization float64 `json:"utilization"`
	PowerCapped bool    `json:"power_capped"`
}

type GPUHealth struct {
	GPUID        string       `json:"gpu_id"`
	NodeID       string       `json:"node_id"`
	Slot         uint16       `json:"slot"`
	Model        string       `json:"model"`
	HealthStatus HealthStatus `json:"status"`
	Timestamp    time.Time    `json:"time_stamp"`
	Utilization  float64      `json:"utilization"`
	Temperature  Temperature  `json:"temperature"`
	Memory       Memory       `json:"memory"`
	Power        Power        `json:"power"`
}
