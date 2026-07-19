# GpuFleetMonitor

[![Build](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/build.yml/badge.svg)](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/build.yml)
[![Test](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/test.yml/badge.svg)](https://github.com/divinebovine/GpuFleetMonitor/actions/workflows/test.yml)

A Go microservices project simulating a GPU fleet health monitoring system for 10,000 GPUs, with a React + TypeScript frontend and a Kubernetes operator for automated GPU health remediation.

> Built as a learning project to explore Go idioms, the standard library, and the broader ecosystem - covering HTTP services, Temporal workflows, Kubernetes operators, and React frontends connected by SSE.

## Demo

Spin up the full stack - Kubernetes (k3s), Temporal, all services, and the frontend - with a single command. No local tooling required beyond Docker.

```bash
docker compose -f hack/demo/docker-compose.yml up -d
```

Once running (allow ~2 minutes for k3s and Temporal to initialize):

| URL | What you'll see |
|---|---|
| http://localhost:5173 | Fleet dashboard - 10,000 GPUs streaming live health state |
| http://localhost:8080 | Temporal UI - MonitorGPU workflow executions |
| http://localhost:8082/v1/escalations | Escalation records (JSON) |

Watch the operator manage 12 GPUs across 3 nodes (runs kubectl inside the k3s container - no local kubeconfig needed):

```bash
# Watch phase transitions in real time
docker compose -f hack/demo/docker-compose.yml exec k3s-server kubectl get gpuhealths -w

# Inspect conditions and findings on a specific GPU
docker compose -f hack/demo/docker-compose.yml exec k3s-server kubectl describe gpuhealth gpu-drain-1

# Watch node cordon state
docker compose -f hack/demo/docker-compose.yml exec k3s-server kubectl get nodes -w
```

**What the 12 GPUs demonstrate:**
- `gpu-node-1` (GPU-00001–00004) - `Drain` policy: operator cordons the node and evicts pods when a GPU goes Critical
- `gpu-node-2` (GPU-00005–00008) - `Escalate` policy: operator sets `EscalationRequired` condition and Temporal triggers the escalation workflow
- `gpu-node-3` (GPU-00009–00012) - `None` policy: operator observes and records findings without automated action

Speed up the simulator to trigger critical events faster - either via the dashboard's settings drawer at http://localhost:5173 or directly:

```bash
curl -X PUT http://localhost:3000/v1/simulation/settings \
  -H "Content-Type: application/json" \
  -d '{"speed_multiplier":50,"warning_to_critical_rate":0.8}'
```

Tear down:

```bash
docker compose -f hack/demo/docker-compose.yml down -v
```

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
    simulator.go        → GetHealth, DegradeToWarning, WorsenToCritical, RecoverToHealthy, StepBackToWarning - simulation transitions and failure-type semantics
    config.go           → SimulationConfig - runtime-tunable rates + speed multiplier
    ticker.go           → DefaultTicker - drives state transitions at configurable speed
    specs.go            → Per-model specs (power/temp ranges, memory size)
  diagnosis/
    model.go            → Diagnosis, Finding, Severity structs
    analyzer.go         → Analyze(*gpu.GPUHealth) - returns *Diagnosis
    store.go            → Thread-safe in-memory diagnosis store
  escalation/
    model.go            → Escalation struct with Resolve()
    store.go            → Thread-safe in-memory escalation store
  temporal/
    workflows/monitor.go → MonitorGPU workflow
    activities/          → health, diagnosis, escalation activities
  controller/
    gpuhealth_controller.go → GPUHealth reconciler - state machine (Healthy→Warning→Critical→Draining→Recovering→Healthy)

api/v1alpha1/
  gpuhealth_types.go    → GPUHealth CRD: spec, status, phases, conditions, findings
  groupversion_info.go  → API group: gpu.nvidia.com/v1alpha1

web/                    → React + TypeScript frontend (Vite, :5173)
```

## GPU Simulation

- 10,000 GPUs across 2,500 nodes (4 GPUs per node)
- IDs: `GPU-00001` through `GPU-10000`
- Node IDs: `NODE-0001` through `NODE-1000`
- Models by GPU number range:
  - 1–2000: H100 (80GB, 700W TDP)
  - 2001–5000: A100 (80GB, 400W TDP)
  - 5001–7000: V100 (32GB, 300W TDP)
  - 7001–10000: A30 (24GB, 165W TDP)
- Temperature thresholds (`GPUCoreWarning`, `GPUCoreCritical`, `MemoryWarning`, `MemoryCritical`) are populated per-model by the simulator and carried on the `Temperature` struct - the analyzer and controller read them from telemetry rather than hardcoding values
- Simulation speed and transition probabilities are tunable at runtime via `PUT /v1/simulation/settings`

### Failure Types

Every non-healthy GPU carries a `failure_type` that drives its behavior through the simulation and determines what remediation can fix it:

| Type | Appears as | Self-heals? | Resolved by |
|---|---|---|---|
| `thermal` | Warning or Critical, high temperature, throttling=true | Yes (Warning→Healthy, Critical→Warning) | Drain (Recover) |
| `power` | Warning or Critical, power_capped=true | Yes (Warning→Healthy, Critical→Warning) | Drain (Recover) |
| `ecc_single` | Warning or Critical, correctable ECC errors present | No - Warning→Healthy path is disabled; stays at Critical if it worsens | Hardware replacement |
| `ecc_double` | Critical only, uncorrectable ECC errors present | No - Critical→Warning path is disabled | Hardware replacement |

`ecc_single` and `ecc_double` are distinct hardware phenomena, not a progression. Single-bit errors are corrected in hardware and accumulate over time. Double-bit errors (two bits flipped in the same word) are uncorrectable and are caused by separate events - cosmic ray strikes, severe cell damage - independent of the single-bit error count.

When a healthy GPU first degrades to Warning, the failure type is assigned with a weighted distribution reflecting real-world failure rates: thermal 50%, power 30%, ecc_single 20%. When any Warning GPU worsens to Critical, there is a 5% chance it develops an `ecc_double` event regardless of the existing failure type; otherwise the failure type carries forward unchanged. This is the only way `ecc_double` enters the simulation - it never starts at Warning.

Telemetry metrics reflect the failure type: thermal failures show elevated temperatures and throttling=true at normal power draw; power failures show power_capped=true at normal temperature; ECC failures show the corresponding hardware error counters with normal temperature and power. The analyzer's findings are consistent with the failure type.

### Operator-Triggered State Changes

Two simulation endpoints let the operator (or the k8s controller) reset a GPU's state after an intervention:

- `PUT /v1/simulation/gpus/{id}/recover` - called after a node drain completes
  - `thermal`/`power`: resets to Healthy; `recovery_warning_rate` (default 10%) chance of landing at Warning instead if the issue wasn't fully resolved
  - `ecc_single`: drain doesn't fix memory errors - GPU stays at Warning with `ecc_single` failure type
  - `ecc_double`: returns HTTP 409 - drain cannot fix uncorrectable ECC errors, hardware replacement is required

- `PUT /v1/simulation/gpus/{id}/replace` - called after hardware replacement is confirmed
  - Always resets to Healthy; `replacement_warning_rate` (default 2%) chance of landing at Warning if the replacement unit arrives with a defect

### Drain Escalation

The k8s controller's drain flow calls `recoverGPU` after the node is fully drained. If that call returns 409 (meaning the GPU has `ecc_double` failure type), the controller skips `PhaseRecovering` and escalates directly to `PhaseReplacing`. The drain was a prerequisite for safe hardware access and the node is already clear of workloads, so the controller pivots without consuming an additional remediation attempt.

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
  -d '{
    "speed_multiplier": 10,
    "healthy_to_warning_rate": 0.05,
    "warning_to_critical_rate": 0.1,
    "warning_to_healthy_rate": 0.05,
    "critical_to_warning_rate": 0.01,
    "recovery_warning_rate": 0.10,
    "replacement_warning_rate": 0.02
  }'
curl -X POST http://localhost:3000/v1/simulation/settings/reset

# Operator-triggered GPU state changes (simulation endpoints)
curl -X PUT http://localhost:3000/v1/simulation/gpus/GPU-00001/recover   # returns 204, or 409 if ecc_double
curl -X PUT http://localhost:3000/v1/simulation/gpus/GPU-00001/replace   # always returns 204

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
make test                                  # full suite (generates, vets, downloads envtest binaries, runs with KUBEBUILDER_ASSETS set)
make test-race                             # race detector on non-controller packages (controller requires envtest, has no meaningful concurrency)

go test ./internal/gpu/ -v                 # verbose output
go test ./internal/diagnosis/ -v
go test ./internal/temporal/workflows/ -v  # shows Temporal event log
```

## Frontend

Vite + React 19 + TypeScript + MUI. Proxies `/api` → `http://localhost:3000` in dev.

- Fleet summary stat cards (Healthy / Warning / Critical counts)
- Virtualized GPU table (10,000 rows via `react-virtuoso`) with Status, Failure type, and Actions columns
  - Thermal/power-capped GPUs show a green **Recover** button - triggers a drain simulation via `PUT /v1/simulation/gpus/{id}/recover`
  - ECC single/double-bit GPUs show a red **Replace** button - triggers a hardware replacement simulation via `PUT /v1/simulation/gpus/{id}/replace`
  - Healthy GPUs show no action buttons
- SSE streaming: rows arrive progressively, sorted by GPU ID on completion; automatically reconnects after the backend restarts
- Simulation settings drawer (gear icon): runtime control over all transition rates and operator action outcomes
- Light/dark theme toggle with `localStorage` persistence

```bash
cd web
npm install
npm run dev   # http://localhost:5173
```

## What's Done

- [x] `internal/gpu` - model, simulator, specs, probabilistic state machine, runtime config
- [x] `cmd/telemetry` - `GET /v1/gpus/{id}`, `GET /v1/gpus` (SSE + JSON); `GET|PUT /v1/simulation/settings`, `POST /v1/simulation/settings/reset`; `PUT /v1/simulation/gpus/{id}/recover` (204 or 409 for ecc_double), `PUT /v1/simulation/gpus/{id}/replace`
- [x] `internal/diagnosis` - model, analyzer (finding codes: `GPUThermalThrottle`, `MemoryThermalThrottle`, `ECCSingleBitError`, `ECCDoubleBitError`, `PowerCapped`, `LowUtilization`), store
- [x] `cmd/diagnosis` - `POST /v1/diagnose/{gpu_id}`, `GET /v1/diagnose/{id}`, `GET /v1/diagnoses`
- [x] `internal/escalation` - model, store
- [x] `cmd/escalation` - `POST /v1/escalations/{id}`, `GET /v1/escalations/{id}`, `GET /v1/escalations`, `PUT /v1/escalations/{id}/resolve`
- [x] `internal/temporal/workflows` - `MonitorGPU` workflow
- [x] `internal/temporal/activities` - `GetHealth`, `Diagnose`, `Escalate` activities
- [x] `cmd/worker/main.go` - Temporal worker on task queue `gpu-monitor`
- [x] Tests - `internal/gpu`, `internal/diagnosis`, `internal/escalation`, `internal/temporal` (activities + workflow)
- [x] CI - GitHub Actions on push/PR (build, vet, test with race detector)
- [x] `web/` - React + TypeScript frontend (Vite) - fleet summary + 10,000-row virtualized GPU table with SSE streaming + Failure type column + operator Recover/Replace action buttons + simulation settings drawer
- [x] `api/v1alpha1` - `GPUHealth` CRD (cluster-scoped, `gpu.nvidia.com/v1alpha1`)
  - Phases: `Healthy → Warning → Critical → Draining → Recovering → Healthy` and `→ Replacing → Rejoining → Healthy` and `→ Failed`
  - Conditions: `GPUHealthy`, `RemediationInProgress`, `EscalationRequired`
  - Finding types: `GPUThermalThrottle`, `MemoryThermalThrottle`, `ECCSingleBitError`, `ECCDoubleBitError`, `PowerCapped`, `LowUtilization`, `XIDError`, `MemoryLeak`
  - Finding severities: `Warning`, `Critical`
  - `ReplacementStartedAt *metav1.Time` tracks when hardware replacement began
  - `ReplacementTimeoutSeconds` (default 1800s, min 60s) - escalates to Failed if node doesn't cycle through NotReady within the timeout, anchored to `ReplacementStartedAt`
  - `LastTransitionTime *metav1.Time` written on every phase change
  - `RemediationPolicy` enum: `None`, `Drain`, `Replace`, `Escalate`; `MaxRemediationAttempts` (1–100, default 3)
- [x] `internal/controller` - `GPUHealthReconciler` - full state machine across all phases
  - Polls telemetry every 30s; status writes debounced via `syncStatus` (`reflect.DeepEqual` diff against a pre-mutation snapshot - no write if nothing changed)
  - `RemediationPolicyDrain`: cordons node, evicts pods via the Eviction API (skipping DaemonSets and terminal pods; PDB-blocked evictions requeue gracefully); on drain complete calls `PUT /v1/simulation/gpus/{id}/recover`
    - 204: transitions to Recovering - thermal/power issues resolved by draining workloads
    - 409: escalates directly to Replacing - drain revealed unrecoverable ECC double-bit errors; node is already clear so the controller pivots without consuming an additional remediation attempt
  - `RemediationPolicyReplace`: cordons node, records findings, sets `ReplacementStartedAt` when node goes NotReady, escalates to Failed if the node doesn't cycle through NotReady within `ReplacementTimeoutSeconds`, transitions to Rejoining once node returns Ready; on return calls `PUT /v1/simulation/gpus/{id}/replace`, then uncordons and returns to Healthy after telemetry confirms recovery
  - `RemediationPolicyEscalate`: sets `ConditionEscalationRequired`, pages human
  - `RemediationPolicyNone`: observes and records findings without automated action
  - `mergeFindings`: ring-buffer dedup capped at 100 entries - existing findings move to tail on update so high-frequency findings don't crowd out others; ECC counts use the telemetry hardware counter directly, temperature/power counts increment per observation
  - Attempt counter resets on spec change (`observedGeneration < generation`); transitions to Failed after `maxRemediationAttempts`
  - RBAC markers for `gpuhealths`, `pods`, `pods/eviction`, `nodes`
  - `--telemetry-url` CLI flag (defaults to `http://localhost:3000`)
- [x] `internal/controller` tests - envtest suite with `httptest.Server` standing in for the telemetry service

## Lessons Learned: Kubernetes Controller Design

Building the operator surfaced a pattern mismatch that's easy to fall into when learning controller-runtime.

### Reconcile loops are convergence checks, not step machines

The v1 controller treats the reconcile loop as a conveyor belt: each phase transition writes status, the watch fires, and the next reconcile advances to the next phase. The problem is that every `syncStatus` write triggers an immediate watch-based reconcile, bypassing the `RequeueAfter` delays the handlers return. Under a fast-cycling simulation this caused a reconcile storm where `gpu-drain-1` was flipping between `Draining` and `Recovering` dozens of times per second.

The Kubernetes controller model is level-triggered, not edge-triggered. A reconcile should answer "given everything I can observe right now, what is the desired state and how do I close the gap?" A well-designed controller reaches the same outcome whether it runs once or a hundred times in a row.

### Polling controllers should not react to their own status writes

The fix was one line in `SetupWithManager`:

```go
For(&v1alpha1.GPUHealth{}, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
```

`GenerationChangedPredicate` filters the watch stream so only spec changes (`.metadata.generation` increments) trigger immediate reconciles. Status updates that the controller itself writes are ignored. Periodic telemetry polling is handled entirely by `RequeueAfter`. This is the right pattern for any controller that polls an external system rather than reacting purely to Kubernetes resource changes.

If you own the status subresource, you never need to watch your own status writes. You already know what you wrote.

### Cordon and drain are node operations, not GPU operations

The `GPUHealth` CRD is scoped to individual GPUs, but cordon, drain, and uncordon are operations on the node those GPUs live on. This mismatch created two concrete failures in the demo.

**Concurrent update conflicts.** The demo puts 4 GPUs per k8s node. When multiple GPUs on the same node go critical simultaneously, their reconcilers all try to cordon the node at the same time: each reads the node object, sets `Spec.Unschedulable = true`, and calls `Update`. The first write wins; the others get a `409 Conflict` because the `ResourceVersion` changed between their read and write. The controller treats a conflict as an error, increments `RemediationAttempts`, and requeues. In the demo `gpu-drain-2` hit `MaxRemediationAttempts = 3` and entered `PhaseFailed` entirely from conflict retries -- no actual drain was ever attempted.

**Premature uncordon.** Each reconciler independently decides when its GPU is healthy enough to uncordon. If `gpu-drain-1` recovers while `gpu-drain-2` is still evicting pods, `gpu-drain-1` calls `uncordonNode` and the scheduler starts placing workloads back on the node before the drain completes.

Both failures are symptoms of the same root cause: the wrong unit. Remediation was modeled at the GPU level but executed at the node level. The right design is a `NodeHealth` CRD: one CR per node, carrying per-GPU status in a list on the status subresource. A single reconciler owns the node, coordinates cordon across all its GPUs, and only uncordons when all of them are in a safe state. There is no shared object to race on.

### What v2 would look like

Three structural improvements for a second version:

**Watch external resources instead of polling for state that Kubernetes already tracks.** The Replace flow polls `isNodeReady` on every tick. A better design adds a `Watches()` on `corev1.Node`, filtered to the node in `spec.nodeName`, and maps node events back to the owning `GPUHealth`. The operator reacts immediately when a node goes `NotReady` or comes back `Ready`, with no polling loop or 15-second requeue.

**Handlers as pure functions, one status write per reconcile.** In v1, each handler calls `syncStatus` inline before returning. In v2, handlers return a value `(nextPhase, conditions, requeueAfter, error)` and `Reconcile` writes status once at the end after all decisions are made. Handlers become deterministic functions of `(currentStatus, telemetry, nodeState)` with no side effects, which makes them unit-testable without envtest.

**Collapse transitions that can be determined synchronously.** If the drain is already complete when a Draining reconcile fires (no pods on the node), there is no reason to write `Recovering` and wait for the next reconcile to check telemetry. The recovery check can run in the same reconcile. Intermediate phases only need to be written when the work is genuinely async: evictions in flight, hardware replacement in progress. Writing a phase to etcd is meaningful when it represents a durable checkpoint; for transitions that resolve in the same tick it is just noise that triggers extra reconciles.

## What's Next

- **Operator v2 (`NodeHealth` CRD):** replace per-GPU `GPUHealth` CRs with a per-node `NodeHealth` CRD -- one CR per k8s node, per-GPU detail in the status list. A single reconciler owns the node, eliminating the cordon conflict race and premature uncordon. Pair with `Watches()` on `corev1.Node`, pure-function handlers, and one status write per reconcile (see ADR 008).
- Add ADRs (`docs/adr/`) for CRD scope, remediationPolicy enum, two-category observability design
- Persist diagnosis and escalation stores (PostgreSQL)
- Add diagnoses and escalations views to the frontend
- Fleet-wide scan: trigger `MonitorGPU` for all 10,000 GPUs in parallel
- Simulate XID errors: add `XIDErrorRate` to `SimulationConfig` and inject discrete XID error events in the ticker
- Simulate memory leaks: add `MemoryLeakRate` to `SimulationConfig` and slowly increment a leaked-bytes counter on affected GPUs; flag in the analyzer when it crosses a threshold
- Wire operator escalation to the escalation service: when the operator sets `EscalationRequired`, call `POST /v1/escalations/{id}` so the Temporal `MonitorGPU` workflow or an external alerting system can act on it

## Dependencies

- `github.com/go-chi/chi/v5 v5.1.0` - HTTP router
- `go.temporal.io/sdk v1.46.0` - Temporal workflow SDK
- `sigs.k8s.io/controller-runtime` - Kubernetes operator framework
- `k8s.io/api`, `k8s.io/apimachinery` - Kubernetes API types
- `vite` + `react` + `typescript` - frontend toolchain
