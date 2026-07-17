# GpuFleetMonitor

[![Build](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/build.yml/badge.svg)](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/build.yml)
[![Test](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/test.yml/badge.svg)](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/test.yml)

A Go microservices project simulating a GPU fleet health monitoring system for 10,000 GPUs, with a React + TypeScript frontend and a Kubernetes operator for automated GPU health remediation.

## Architecture

```
cmd/
  telemetry/main.go     → HTTP API on :3000  (GPU health queries, simulation settings)
  diagnosis/main.go     → HTTP API on :8081  (diagnose a GPU)
  escalation/main.go    → HTTP API on :8082  (manage escalations)
  worker/main.go        → Temporal worker    (orchestrates workflows)
  operator/main.go      → Kubernetes operator (GPU health CRD controller)

internal/
  gpu/
    model.go            → GPUHealth, Temperature, Memory, Power structs
    simulator.go        → GetHealth(gpuID) — probabilistic simulation with tunable rates
    config.go           → SimulationConfig — runtime-tunable rates + speed multiplier
    ticker.go           → DefaultTicker — drives state transitions at configurable speed
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
  controller/
    gpuhealth_controller.go → GPUHealth reconciler — state machine (Healthy→Warning→Critical→Draining→Recovering→Healthy)

api/v1alpha1/
  gpuhealth_types.go    → GPUHealth CRD: spec, status, phases, conditions, findings
  groupversion_info.go  → API group: gpu.nvidia.com/v1alpha1

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
- Health status is probabilistic: GPUs transition between Healthy/Warning/Critical states over time via configurable rates
- Temperature thresholds (`GPUCoreWarning`, `GPUCoreCritical`, `MemoryWarning`, `MemoryCritical`) are populated per-model by the simulator and carried on the `Temperature` struct — the analyzer and controller read them from telemetry rather than hardcoding values
- Simulation speed and transition probabilities are tunable at runtime via `PUT /v1/simulation/settings`

## Local Infrastructure

```bash
# Start Temporal server + UI (requires Docker)
docker compose up -d

# Temporal UI: http://localhost:8080
# Temporal server: localhost:7233
```

## Running

```bash
# Telemetry service (GPU health API + simulation settings)
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

# Simulation settings
curl http://localhost:3000/v1/simulation/settings
curl -X PUT http://localhost:3000/v1/simulation/settings \
  -H "Content-Type: application/json" \
  -d '{"speed_multiplier":10,"healthy_to_warning_rate":0.05,"warning_to_critical_rate":0.1,"warning_to_healthy_rate":0.05}'
curl -X POST http://localhost:3000/v1/simulation/settings/reset

# Kubernetes operator (requires kind cluster + CRDs installed)
make install                       # install CRDs into cluster
./bin/manager                      # run the operator

# Apply a sample GPUHealth CR
kubectl apply -f config/samples/gpu_v1alpha1_gpuhealth.yaml
kubectl get gh -w                  # watch phase transitions
kubectl describe gh gpuhealth-00001
```

## Testing

```bash
go test ./...
go test ./... -race                        # with race detector
go test ./internal/gpu/ -v                 # verbose output
go test ./internal/diagnosis/ -v
go test ./internal/temporal/workflows/ -v  # shows Temporal event log

# Controller suite requires envtest binaries
make setup-envtest
make test                                  # runs full suite with KUBEBUILDER_ASSETS set
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

- [x] `internal/gpu` — model, simulator, specs, probabilistic state machine, runtime config
- [x] `cmd/telemetry` — `GET /v1/gpus/{id}`, `GET /v1/gpus` (SSE + JSON); `GET|PUT /v1/simulation/settings`, `POST /v1/simulation/settings/reset`
- [x] `internal/diagnosis` — model, analyzer (finding codes: `GPUThermalThrottle`, `MemoryThermalThrottle`, `ECCSingleBitError`, `ECCDoubleBitError`, `PowerCapped`, `LowUtilization`), store
- [x] `cmd/diagnosis` — `POST /v1/diagnose/{gpu_id}`, `GET /v1/diagnose/{id}`, `GET /v1/diagnoses`
- [x] `internal/escalation` — model, store
- [x] `cmd/escalation` — `POST /v1/escalations/{id}`, `GET /v1/escalations/{id}`, `GET /v1/escalations`, `PUT /v1/escalations/{id}/resolve`
- [x] `internal/temporal/workflows` — `MonitorGPU` workflow
- [x] `internal/temporal/activities` — `GetHealth`, `Diagnose`, `Escalate` activities
- [x] `cmd/worker/main.go` — Temporal worker on task queue `gpu-monitor`
- [x] Tests — `internal/gpu`, `internal/diagnosis`, `internal/escalation`, `internal/temporal` (activities + workflow)
- [x] CI — GitHub Actions on push/PR (build, vet, test with race detector)
- [x] `web/` — React + TypeScript frontend (Vite) — fleet summary + 10,000-row virtualized GPU table with SSE streaming + simulation settings drawer
- [x] `api/v1alpha1` — `GPUHealth` CRD (cluster-scoped, `gpu.nvidia.com/v1alpha1`)
  - Phases: `Healthy → Warning → Critical → Draining → Recovering → Healthy` and `→ Replacing → Rejoining → Healthy` and `→ Failed`
  - Conditions: `GPUHealthy`, `RemediationInProgress`, `EscalationRequired`
  - Finding types: `GPUThermalThrottle`, `MemoryThermalThrottle`, `ECCSingleBitError`, `ECCDoubleBitError`, `PowerCapped`, `LowUtilization`, `XIDError`, `MemoryLeak`
  - Finding severities: `Warning`, `Critical`
  - `ReplacementStartedAt *metav1.Time` tracks when hardware replacement began
  - `LastTransitionTime *metav1.Time` written on every phase change
  - `RemediationPolicy` enum: `None`, `Drain`, `Replace`, `Escalate`; `MaxRemediationAttempts` (1–100, default 3)
- [x] `internal/controller` — `GPUHealthReconciler` — full state machine across all phases
  - Polls telemetry every 30s; status writes debounced via `syncStatus` (`reflect.DeepEqual` diff against a pre-mutation snapshot — no write if nothing changed)
  - `RemediationPolicyDrain`: cordons node, waits for pod eviction (skipping DaemonSets and terminal pods), transitions to Recovering, uncordons on recovery
  - `RemediationPolicyReplace`: cordons node, records findings, sets `ReplacementStartedAt` when node goes NotReady, transitions to Rejoining once node returns Ready, uncordons and returns to Healthy after telemetry confirms recovery
  - `RemediationPolicyEscalate`: sets `ConditionEscalationRequired`, pages human
  - `RemediationPolicyNone`: observes and records findings without automated action
  - `mergeFindings`: ring-buffer dedup capped at 100 entries — existing findings move to tail on update so high-frequency findings don't crowd out others; ECC counts use the telemetry hardware counter directly, temperature/power counts increment per observation
  - Attempt counter resets on spec change (`observedGeneration < generation`); transitions to Failed after `maxRemediationAttempts`
  - RBAC markers for `gpuhealths`, `pods`, `nodes`
  - `--telemetry-url` CLI flag (defaults to `http://localhost:3000`)
- [x] `internal/controller` tests — envtest suite with `httptest.Server` standing in for the telemetry service

## What's Next

- Add ADRs (`docs/adr/`) for CRD scope, remediationPolicy enum, two-category observability design
- Persist diagnosis and escalation stores (PostgreSQL)
- Add diagnoses and escalations views to the frontend
- Fleet-wide scan: trigger `MonitorGPU` for all 10,000 GPUs in parallel
- DCGM exporter / NVML event stream simulator for XID error injection

## Dependencies

- `github.com/go-chi/chi/v5 v5.1.0` — HTTP router
- `go.temporal.io/sdk v1.46.0` — Temporal workflow SDK
- `sigs.k8s.io/controller-runtime` — Kubernetes operator framework
- `k8s.io/api`, `k8s.io/apimachinery` — Kubernetes API types
- `vite` + `react` + `typescript` — frontend toolchain
