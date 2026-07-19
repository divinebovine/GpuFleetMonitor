package gpu

import "sync"

type SimulationConfig struct {
	mu                     sync.RWMutex
	SpeedMultiplier        int64
	HealthyToWarningRate   float64
	WarningToCriticalRate  float64
	WarningToHealthyRate   float64
	CriticalToWarningRate  float64
	RecoveryWarningRate    float64
	ReplacementWarningRate float64
}

type SimulationSettings struct {
	SpeedMultiplier        int64   `json:"speed_multiplier"`
	HealthyToWarningRate   float64 `json:"healthy_to_warning_rate"`
	WarningToCriticalRate  float64 `json:"warning_to_critical_rate"`
	WarningToHealthyRate   float64 `json:"warning_to_healthy_rate"`
	CriticalToWarningRate  float64 `json:"critical_to_warning_rate"`
	RecoveryWarningRate    float64 `json:"recovery_warning_rate"`
	ReplacementWarningRate float64 `json:"replacement_warning_rate"`
}

var defaults *SimulationSettings = &SimulationSettings{
	SpeedMultiplier:        1,
	HealthyToWarningRate:   0.0005,
	WarningToCriticalRate:  0.01,
	WarningToHealthyRate:   0.005,
	CriticalToWarningRate:  0.003,
	RecoveryWarningRate:    0.10,
	ReplacementWarningRate: 0.02,
}

var Config *SimulationConfig = &SimulationConfig{
	SpeedMultiplier:        defaults.SpeedMultiplier,
	HealthyToWarningRate:   defaults.HealthyToWarningRate,
	WarningToCriticalRate:  defaults.WarningToCriticalRate,
	WarningToHealthyRate:   defaults.WarningToHealthyRate,
	CriticalToWarningRate:  defaults.CriticalToWarningRate,
	RecoveryWarningRate:    defaults.RecoveryWarningRate,
	ReplacementWarningRate: defaults.ReplacementWarningRate,
}

func (s *SimulationConfig) Get() *SimulationSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &SimulationSettings{
		SpeedMultiplier:        s.SpeedMultiplier,
		HealthyToWarningRate:   s.HealthyToWarningRate,
		WarningToCriticalRate:  s.WarningToCriticalRate,
		WarningToHealthyRate:   s.WarningToHealthyRate,
		CriticalToWarningRate:  s.CriticalToWarningRate,
		RecoveryWarningRate:    s.RecoveryWarningRate,
		ReplacementWarningRate: s.ReplacementWarningRate,
	}
}

func (s *SimulationConfig) Set(cfg *SimulationSettings) *SimulationSettings {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.SpeedMultiplier = cfg.SpeedMultiplier
	s.HealthyToWarningRate = cfg.HealthyToWarningRate
	s.WarningToCriticalRate = cfg.WarningToCriticalRate
	s.WarningToHealthyRate = cfg.WarningToHealthyRate
	s.CriticalToWarningRate = cfg.CriticalToWarningRate
	s.RecoveryWarningRate = cfg.RecoveryWarningRate
	s.ReplacementWarningRate = cfg.ReplacementWarningRate

	return &SimulationSettings{
		SpeedMultiplier:        s.SpeedMultiplier,
		HealthyToWarningRate:   s.HealthyToWarningRate,
		WarningToCriticalRate:  s.WarningToCriticalRate,
		WarningToHealthyRate:   s.WarningToHealthyRate,
		CriticalToWarningRate:  s.CriticalToWarningRate,
		RecoveryWarningRate:    s.RecoveryWarningRate,
		ReplacementWarningRate: s.ReplacementWarningRate,
	}
}

func (s *SimulationConfig) Reset() *SimulationSettings {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.SpeedMultiplier = defaults.SpeedMultiplier
	s.HealthyToWarningRate = defaults.HealthyToWarningRate
	s.WarningToCriticalRate = defaults.WarningToCriticalRate
	s.WarningToHealthyRate = defaults.WarningToHealthyRate
	s.CriticalToWarningRate = defaults.CriticalToWarningRate

	return &SimulationSettings{
		SpeedMultiplier:        s.SpeedMultiplier,
		HealthyToWarningRate:   s.HealthyToWarningRate,
		WarningToCriticalRate:  s.WarningToCriticalRate,
		WarningToHealthyRate:   s.WarningToHealthyRate,
		CriticalToWarningRate:  s.CriticalToWarningRate,
		RecoveryWarningRate:    s.RecoveryWarningRate,
		ReplacementWarningRate: s.ReplacementWarningRate,
	}
}
