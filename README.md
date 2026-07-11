# GpuFleetMonitor

[![Build](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/build.yml/badge.svg)](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/build.yml)
[![Test](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/test.yml/badge.svg)](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/test.yml)

A Go microservices project simulating a GPU fleet health monitoring system for 10,000 GPUs, with a React + TypeScript frontend.

## Architecture

```
cmd/
  telemetry/main.go     → HTTP API on :3000  (GPU health queries)
  diagnosis/main.go     → HTTP API on :8081  (diagnose a GPU)
  escalation/main.go    → HTTP API on :8082  (manage escalations)
  worker/main.go        → Temporal worker    (orchestrates workflows)

internal/
  gpu/
    model.go            → GPUHealth, Temperature, Memory, Power structs
    simulator.go        → GetHealth(gpuID) — deterministic simulation
    specs.go            → Per-model specs (power/temp ranges, memory size)
  diagnosis/
    model.go            → Diagnosis, Finding, Severity structs
    analyzer.go         → Analyze(*gpu.GPUHealth) — returns *Diagnosis
    store.go            → Thread-safe in-memory diagnosis store
  escalation/
    model.go            → Escalation struct with Resolve()
    store.go            → Thread-safe in-memory escalation store
  temporal/
    workflows/monitor.go → MonitorGPU workflow
    activities/          → health, diagnosis, escalation activities

web/                    → React + TypeScript frontend (Vite, :5173)
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

## Local Infrastructure

```bash
# Start Temporal server + UI (requires Docker)
docker compose up -d

# Temporal UI: http://localhost:8080
# Temporal server: localhost:7233
```

## Running

```bash
# Telemetry service
go run ./cmd/telemetry/

# Stream all GPUs via SSE (used by the frontend)
curl -H "Accept: text/event-stream" http://localhost:3000/v1/gpus

# Get all GPUs as JSON
curl http://localhost:3000/v1/gpus

# Get a single GPU
curl http://localhost:3000/v1/gpus/GPU-00001
curl http://localhost:3000/v1/gpus/GPU-00005   # critical GPU

# Diagnosis service
go run ./cmd/diagnosis/

# Test it
curl -X POST http://localhost:8081/v1/diagnose/GPU-00005
curl http://localhost:8081/v1/diagnose/diag-GPU-00005
curl http://localhost:8081/v1/diagnoses

# Escalation service
go run ./cmd/escalation/

# Test it
curl -s -X POST http://localhost:8082/v1/escalations/esc-001 \
  -H "Content-Type: application/json" \
  -d '{"id":"esc-001","gpu_id":"GPU-00005","diagnosis_id":"diag-GPU-00005","severity":"critical","status":"open","created_at":"2026-07-08T00:00:00Z"}'
curl http://localhost:8082/v1/escalations/esc-001
curl http://localhost:8082/v1/escalations
curl -X PUT http://localhost:8082/v1/escalations/esc-001/resolve
```

## Testing

```bash
go test ./...
go test ./... -race                        # with race detector
go test ./internal/gpu/ -v                 # verbose output
go test ./internal/diagnosis/ -v
go test ./internal/temporal/workflows/ -v  # shows Temporal event log
```

## Frontend

Vite + React 19 + TypeScript + MUI. Proxies `/api` → `http://localhost:3000` in dev.

- Fleet summary stat cards (Healthy / Warning / Critical counts)
- Virtualized GPU table (10,000 rows via `react-virtuoso`)
- SSE streaming: rows arrive progressively, sorted by GPU ID on completion
- Light/dark theme toggle with `localStorage` persistence

```bash
cd web
npm install
npm run dev   # http://localhost:5173
```

## What's Done

- [x] `internal/gpu` — model, simulator, specs
- [x] `cmd/telemetry` — `GET /v1/gpus/{id}`, `GET /v1/gpus` (worker pool, 100 concurrent; content negotiation: SSE or JSON)
- [x] `internal/diagnosis` — model, analyzer, store
- [x] `cmd/diagnosis` — `POST /v1/diagnose/{gpu_id}`, `GET /v1/diagnose/{id}`, `GET /v1/diagnoses`
- [x] `internal/escalation` — model, store
- [x] `cmd/escalation` — `POST /v1/escalations/{id}`, `GET /v1/escalations/{id}`, `GET /v1/escalations`, `PUT /v1/escalations/{id}/resolve`
- [x] `internal/temporal/workflows` — `MonitorGPU` workflow
- [x] `internal/temporal/activities` — `GetHealth`, `Diagnose`, `Escalate` activities
- [x] `cmd/worker/main.go` — Temporal worker on task queue `gpu-monitor`
- [x] Tests — `internal/gpu` (including `AllIDs`), `internal/diagnosis`, `internal/escalation`, `internal/temporal` (activities + workflow)
- [x] CI — GitHub Actions on push/PR (build, vet, test with race detector)
- [x] `web/` — React + TypeScript frontend (Vite) — fleet summary + 10,000-row virtualized GPU table with SSE streaming

## What's Next

- Add diagnoses and escalations views to the frontend
- Persist diagnosis and escalation stores (database backend)
- Fleet-wide scan: trigger `MonitorGPU` for all 10,000 GPUs in parallel
- Expose workflow status via HTTP API

## Dependencies

- `github.com/go-chi/chi/v5 v5.1.0` — HTTP router
- `go.temporal.io/sdk v1.46.0` — Temporal workflow SDK
- `vite` + `react` + `typescript` — frontend toolchain
