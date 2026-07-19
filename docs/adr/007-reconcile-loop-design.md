# 007 - Reconcile Loop Design: Step Machine vs Convergence Check

## Status
Accepted

## Context
The v1 controller was written as a step machine: each reconcile advances the GPU one phase forward, writes status, and returns. The watch fires on every status write, which triggers the next reconcile immediately. Under a fast simulator (50x+), this caused a reconcile storm - `gpu-drain-1` flipped between `Draining` and `Recovering` dozens of times per second because every `syncStatus` write restarted the loop before `RequeueAfter` could enforce any delay.

This is a design mismatch with how Kubernetes controllers are intended to work. The controller model is level-triggered, not edge-triggered. A reconcile should answer "given everything I can observe right now, what is the desired state and how do I close the gap?" - not "what is the next step in a sequence?"

A second bug amplified the storm: in `handleRemediationPolicyDrain`, the return value of `drainNode` (which is `isStillDraining bool`) was assigned to a variable named `isFullyDrained` and checked with `!isFullyDrained`. When no pods remained on the node (`drainNode` returned `false`), `!false` fired an early return that prevented the phase from ever advancing, so the controller looped in place indefinitely.

## Decision
Two fixes were applied:

1. **Renamed variable to match semantics.** `isFullyDrained` was renamed to `isStillDraining` and the condition was corrected from `!isFullyDrained` to `isStillDraining`. This was the root cause of 7 drain-related test failures.

2. **Applied `GenerationChangedPredicate`.** Added `builder.WithPredicates(predicate.GenerationChangedPredicate{})` in `SetupWithManager`. This filters the watch stream so only spec changes (`.metadata.generation` increments) trigger immediate reconciles. Status writes from the controller itself are ignored. Periodic telemetry polling is handled entirely by `RequeueAfter`.

```go
func (r *GPUHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.GPUHealth{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
        Named("gpuhealth").
        Complete(r)
}
```

The structural lesson: if you own the status subresource, you never need to watch your own status writes. You already know what you wrote.

## Consequences
- The reconcile storm is eliminated; phase transitions are now driven by `RequeueAfter` intervals, not by watch events on self-written status
- A v2 redesign would go further: `Watches()` on `corev1.Node` instead of polling node state, handlers as pure functions returning desired state rather than writing status inline, and a single status write per reconcile at the call site in `Reconcile`; intermediate phases like `Recovering` would only be written when the work is genuinely async
- `GenerationChangedPredicate` is the right default for any polling controller - one that checks external state rather than reacting purely to Kubernetes resource changes
