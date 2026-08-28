# 014 - `GPUState` Holds Both Categorical State and Field Values, Keyed by DCGM Field ID

## Status
Accepted

## Context
The injector needs two kinds of information per simulated GPU: categorical state (`Status`, `FailureType`, driving the probabilistic transition machine) and the current simulated value of each DCGM field (temperature, power, etc.). The natural first split was two parallel data structures — `GPUState` for the former, and a separate map keyed by entity ID for the latter — updated by two separate functions (`Advance`, and a value-stepping function) each tick.

That split introduces avoidable bookkeeping: `tick.go` would need to iterate `states []GPUState` and a second, separately-indexed collection in lockstep, matching them up by `EntityID` on every access, with no compiler-enforced guarantee the two stay aligned.

## Decision
`GPUState` carries both: `Status`/`FailureType` for categorical state, and `Values map[FieldID]float64` for the current simulated value of every field tracked for that GPU. Field values are keyed by the same `FieldID` type used throughout `metrics.go`/`config.go`, not given individual named struct fields — adding a new tracked field is a new map entry, not a new struct field plus new code paths everywhere that touches `GPUState`.

The *logic* stays split even though the *data* didn't: `Advance` only reads/writes `Status`/`FailureType`, and `AdvanceValues`/`advanceThermal`/`advancePowerUsage` only read/write `Values` (reading the already-updated `Status`/`FailureType` as input). Both operate on the same `[]GPUState` slice, called in sequence each tick, rather than one function trying to own both concerns.

## Consequences
- `tick.go` iterates one slice once per tick; there's no second collection to keep indexed/synchronized by entity ID
- `Advance`'s existing tests are unaffected by the value-generation logic living on the same struct, since it never reads or writes `Values`
- `NewGPUStates` is responsible for seeding any field that needs a non-zero starting value before the first tick (e.g. `FieldPowerLimit`, seeded from the model's spec) — a field left unseeded and never written by `Advance`/`AdvanceValues` will simply never appear in `/metrics` output, since `Collect` only reports what's actually present in `Values`
