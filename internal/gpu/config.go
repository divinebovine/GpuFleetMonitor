package gpu

import "sync"

type SimulationConfig struct {
	mu       sync.RWMutex
	Settings *SimulationSettings
}

type SimulationSettings struct {
	SpeedMultiplier        int64   `json:"speed_multiplier"`
	HealthyToWarningRate   float64 `json:"healthy_to_warning_rate"`
	WarningToCriticalRate  float64 `json:"warning_to_critical_rate"`
	WarningToHealthyRate   float64 `json:"warning_to_healthy_rate"`
	CriticalToWarningRate  float64 `json:"critical_to_warning_rate"`
	RecoveryWarningRate    float64 `json:"recovery_warning_rate"`
	ReplacementWarningRate float64 `json:"replacement_warning_rate"`
	PersistentWarningRate  float64 `json:"persistent_warning_rate"`
	HardwareFailureRate    float64 `json:"hardware_failure_rate"`
}

func (s *SimulationSettings) DeepCopy() *SimulationSettings {
	copy := *s
	return &copy
}

var defaults *SimulationSettings = &SimulationSettings{
	SpeedMultiplier:        1,
	HealthyToWarningRate:   0.0005,
	WarningToCriticalRate:  0.01,
	WarningToHealthyRate:   0.005,
	CriticalToWarningRate:  0.003,
	RecoveryWarningRate:    0.10,
	ReplacementWarningRate: 0.02,
	PersistentWarningRate:  0.05,
	HardwareFailureRate:    0.05,
}

var Config *SimulationConfig = &SimulationConfig{
	Settings: defaults.DeepCopy(),
}

func (s *SimulationConfig) Get() *SimulationSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Settings.DeepCopy()
}

func (s *SimulationConfig) Set(cfg *SimulationSettings) *SimulationSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Settings = cfg.DeepCopy()
	return cfg.DeepCopy()
}

func (s *SimulationConfig) Reset() *SimulationSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Settings = defaults.DeepCopy()
	return defaults.DeepCopy()
}
