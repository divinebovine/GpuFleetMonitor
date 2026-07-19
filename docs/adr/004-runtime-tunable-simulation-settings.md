# 004 - Runtime-Tunable Simulation Settings

## Status
Accepted

## Context
The simulator originally used hardcoded transition probabilities and a fixed tick rate. Demonstrating the system during a live demo required driving GPUs into Warning and Critical states, triggering drain cycles, and showing recovery - all of which happen too slowly at realistic rates to be useful in a short session.

Three approaches were considered:

1. **Restart with different config** - pass flags or environment variables and restart the telemetry service
2. **Test-only overrides** - inject different rates only in tests, keep production behavior fixed
3. **Runtime-mutable settings endpoint** - expose a `PUT /v1/simulation/settings` endpoint that changes rates and speed in a running process

Restarting breaks active SSE connections and clears in-memory state. Test-only overrides don't help with live demos. A runtime API lets the demo operator speed up the clock and tune transition rates while the frontend stays connected.

## Decision
Added `SimulationConfig` as a runtime-mutable struct protected by `sync.RWMutex`. A `DefaultTicker` drives state transitions at a rate scaled by `speed_multiplier`. The telemetry service exposes:

- `GET /v1/simulation/settings` - read current config
- `PUT /v1/simulation/settings` - update any combination of speed and transition rates
- `POST /v1/simulation/settings/reset` - restore defaults

The frontend exposes these controls in a settings drawer alongside the GPU table, so the demo operator doesn't need curl access.

## Consequences
- Demo can be fast-forwarded to 50x or higher without restarting any service or dropping client connections
- Settings are in-memory and reset on process restart, which is fine for a simulator - no persistence is needed
- The `speed_multiplier` interacts with the Kubernetes operator's `RequeueAfter` delays: at very high multipliers the operator can't keep up with how fast GPUs cycle through states, which exposed the reconcile storm bug documented in ADR 007
