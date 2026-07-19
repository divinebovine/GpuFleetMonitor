export type HealthStatus = "healthy" | "warning" | "critical";

export type FailureType = "none" | "thermal" | "power" | "ecc_single" | "ecc_double";

export type Temperature = {
  gpu_core_celsius: number;
  memory_celsius: number;
  warning_threshold: number;
  critical_threshold: number;
  throttling: boolean;
};

export type Memory = {
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
  utilization: number;
  ecc_single_bit_errors: number;
  ecc_double_bit_errors: number;
};

export type Power = {
  draw_watts: number;
  limit_watts: number;
  utilization: number;
  power_capped: boolean;
};

export type GPUHealth = {
  gpu_id: string;
  node_id: string;
  slot: number;
  model: string;
  status: HealthStatus;
  failure_type: FailureType;
  timestamp: string;
  utilization: number;
  temperature: Temperature;
  memory: Memory;
  power: Power;
};

export type SimulationSettings = {
  speed_multiplier: number;
  healthy_to_warning_rate: number;
  warning_to_critical_rate: number;
  warning_to_healthy_rate: number;
  critical_to_warning_rate: number;
  recovery_warning_rate: number;
  replacement_warning_rate: number;
};
