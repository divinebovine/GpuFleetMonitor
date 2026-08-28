# 012 - `GPUState.FailureType` Stays Single-Valued, Multi-Condition Tracking Deferred

## Status
Accepted

## Context
A GPU can only carry one `FailureType` at a time under the current design. A Warning-level condition (e.g. `ECCSingle`) is silently overwritten if the GPU later escalates to a Critical-level override (`ECCDouble`/`GpuFellOffTheBus`, via the hardware-failure roll on Warning→Critical). Real GPU health monitoring doesn't have this constraint — DCGM's own Health API (`HealthSystem` watches are OR-able bitmask flags, and a health check's `Incidents` result is a list) is built around a GPU having multiple simultaneous unhealthy conditions across different subsystems at once, not one dominant cause.

Today the single-value model is harmless: the only thing that clears a hardware-failure override is `Replace()` (a full hardware swap), which legitimately should wipe prior history — new silicon doesn't carry over the old card's accumulated errors. But the assumption would break the moment a hardware failure type gets a recovery path that *isn't* a full replacement (for example, modeling `GpuFellOffTheBus`/XID 79 as sometimes resolvable via a reset rather than always requiring RMA, which NVIDIA's own guidance for that XID allows for). At that point, whatever pre-existing Warning-level condition it overrode would vanish incorrectly instead of being restored.

## Decision
Keep `FailureType` single-valued for now. Don't add multi-condition tracking (e.g. turning it into a slice or set) preemptively — there's no concrete scenario requiring it yet, and doing so would add real complexity (tracking which conditions are independent vs. which supersede each other, and reworking the transition/reporting logic to handle a set instead of a value) with no current payoff.

## Consequences
- Revisit this decision specifically if/when a hardware-failure type gains a non-replacement recovery path — at that point either genuinely track multiple concurrent conditions, find another way to preserve/restore the overridden condition, or explicitly accept the information loss once the concrete scenario is in front of us, rather than deciding it in the abstract now
- Until then, a GPU's `FailureType` should be read as "the single most severe/relevant active condition," not "the only thing currently wrong with this GPU" — this matches how the state machine and metrics reporting already treat it, just worth keeping explicit
