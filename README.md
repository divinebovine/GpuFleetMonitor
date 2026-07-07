# gpu-monitor

A Go microservices project simulating a GPU fleet health monitoring system for 10,000 GPUs.

## Architecture

```
cmd/
  telemetry/main.go     → HTTP API on :3000  (GPU health queries)
  diagnosis/main.go     → HTTP API on :8081  (diagnose a GPU)
  escalation/main.go    → HTTP API on :8082  (manage escalations)
  worker/main.go        → Temporal worker    (orchestrates workflows)

internal/
  gpu/
    model.go            → GpuHealth, Temperature, Memory, Power structs
    simulator.go        → GetHealth(gpuId) — deterministic simulation
    specs.go            → Per-model specs (power/temp ranges, memory size)
  diagnosis/
    model.go            → Diagnosis, Finding, Severity structs
    analyzer.go         → Analyze(*gpu.GpuHealth) — in progress
    store.go            → In-memory diagnosis store — in progress
  escalation/           → Not started
  temporal/             → Not started
```

## GPU Simulation

- 10,000 GPUs across 1,000 nodes (10 GPUs per node)
- IDs: `GPU-00001` through `GPU-10000`
- Node IDs: `NODE-0001` through `NODE-1000`
- Models by GPU number range:
  - 1–2000: H100 (80GB, 700W TDP)
  - 2001–5000: A100 (80GB, 400W TDP)
  - 5001–7000: V100 (32GB, 300W TDP)
  - 7001–10000: A30 (24GB, 165W TDP)
- Health status is deterministic: hash(gpuId) % 100 → 0–4 = critical, 5–14 = warning, 15–99 = healthy
- Values (temperature, power, memory) are seeded from the same hash so they're consistent across calls

## Running

```bash
export GOTOOLCHAIN=local   # required — system has Go 1.22, go.mod targets 1.22

# Telemetry service
go run ./cmd/telemetry/

# Test it
curl http://localhost:3000/v1/gpus/GPU-00001
curl http://localhost:3000/v1/gpus/GPU-00005   # critical GPU
```

## What's Done

- [x] `internal/gpu` — model, simulator, specs (compiles and runs)
- [x] `cmd/telemetry` — HTTP server with `GET /v1/gpus/{id}` (working)
- [x] `internal/diagnosis/model.go` — Diagnosis and Finding structs

## What's Next

1. `internal/diagnosis/analyzer.go` — analyze a `*gpu.GpuHealth` and return a `*Diagnosis`
   - Check temperature against warning/critical thresholds → findings
   - Check ECC errors → findings
   - Check power utilization → findings
   - Check GPU utilization (very low may indicate stuck GPU) → findings
   - Roll up worst severity across findings
   - Generate a recommendation string

2. `internal/diagnosis/store.go` — thread-safe in-memory store
   - `Save(*Diagnosis)`
   - `GetByID(id string) (*Diagnosis, bool)`
   - `List() []*Diagnosis`
   - Use `sync.Mutex` to protect concurrent access

3. `cmd/diagnosis/main.go` — HTTP server on :8081
   - `POST /v1/diagnose/{gpu_id}` — fetch health, run analyzer, save, return diagnosis
   - `GET /v1/diagnoses/{id}` — retrieve saved diagnosis
   - `GET /v1/diagnoses` — list all diagnoses

4. Escalation service (`internal/escalation/`, `cmd/escalation/`)

5. Temporal worker (`internal/temporal/`, `cmd/worker/`)

## Dependencies

- `github.com/go-chi/chi/v5 v5.1.0` — HTTP router
- Temporal Go SDK — to be added when worker is started
