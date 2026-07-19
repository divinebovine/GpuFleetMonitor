# 008 - Node-Level CRD: Replace Per-GPU GPUHealth with Per-Node NodeHealth

## Status
Accepted

## Context
Remediation in a GPU cluster is a node-level operation. When a GPU fails, you cordon the node it lives on, drain that node's pods, and uncordon it after recovery. The v1 operator models this with one `GPUHealth` CR per GPU, giving 4 independent reconcilers per node in the demo cluster. Because each reconciler independently manages the shared node object, two concrete failures emerged in practice.

**Concurrent update conflicts.** When multiple GPUs on the same node go critical simultaneously, their reconcilers all call `cordonNode` at the same time. Each reads the node, sets `Spec.Unschedulable = true`, and writes it back. The first write wins; the others get a `409 Conflict` because the `ResourceVersion` changed between read and write. The conflicts trigger retries with exponential backoff, which drove a stream of error logs and caused `RemediationAttempts` to increment from retry noise rather than actual failed recovery cycles. In one observed run, `gpu-drain-2` reached `MaxRemediationAttempts = 3` and entered `PhaseFailed` entirely from conflict-driven retries — no real recovery was attempted.

**No coordination on uncordon.** Each reconciler independently decides when its GPU has recovered enough to uncordon the node. If `gpu-drain-1` recovers while `gpu-drain-2` is still mid-drain, `gpu-drain-1`'s reconciler uncordons the node before the drain completes. New workloads can be scheduled back onto the node while pods are still being evicted.

Both failures have the same root cause: the unit of the CRD is wrong. The CRD, state machine, and reconcile loop are all scoped to individual GPUs, but the operations they perform — cordon, drain, uncordon — are node-scoped. Modeling the wrong unit forces coordination that the controller framework is not designed to provide.

## Decision
Replace the per-GPU `GPUHealth` CRD with a per-node `NodeHealth` CRD. Each `NodeHealth` CR represents one k8s node and carries the health state of all its GPUs in its status. A single reconciler fires per node, reads telemetry for all GPUs on that node each tick, determines the worst-case status across them, and makes one coordinated decision about cordon, drain, recover, or replace. Only one goroutine owns the node, so there is no shared state contention. Uncordon happens only when the reconciler has verified all GPUs on the node are in an acceptable state.

Per-GPU detail — failure type, individual status, findings — is preserved as a list in the `NodeHealth` status. The dashboard moves from a flat per-GPU row to a node-grouped view where the primary action surface is the node.

## Consequences
- Cordon and uncordon conflicts are eliminated by construction; one reconciler owns one node
- The remediation attempt counter counts actual recovery cycles, not conflict retries
- The premature uncordon race cannot occur; the node reconciler holds the cordon until all GPUs on the node are in a safe state
- The `ecc_double` drain-escalation path simplifies: the node reconciler already sees all GPU failure types before deciding to drain, so it can select the right remediation path without an extra HTTP round-trip after the fact
- Existing `GPUHealth` CRs are replaced, not extended; the schema is not forward-compatible
