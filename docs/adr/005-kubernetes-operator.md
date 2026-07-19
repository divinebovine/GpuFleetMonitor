# 005 - Kubernetes Operator for GPU Health Remediation

## Status
Accepted

## Context
After building out the Temporal workflows and HTTP services, the project pivoted to learning Kubernetes. Adding a GPU health operator was the natural way to explore the Kubernetes API hands-on - covering controller-runtime, custom resource definitions, the reconcile loop model, status subresources, conditions, RBAC markers, and envtest.

## Decision
Implemented a `controller-runtime` based operator with a `GPUHealth` CRD. The operator runs a remediation state machine across all phases (`Healthy -> Warning -> Critical -> Draining -> Recovering -> Healthy`, with `Replacing`, `Rejoining`, `Escalating`, and `Failed` branches). It exercises the core Kubernetes controller concepts: reconcile loops, status writes, node cordoning, pod eviction via the Eviction API, and RBAC markers.

## Consequences
- Covers controller-runtime, CRD schema validation, status subresource, RBAC markers, and envtest in one feature
- Controller integration tests require `envtest`, which downloads matching Kubernetes API server and etcd binaries at test time
- The reconcile loop model turned out to be more subtle than expected - the initial design treated it as a step machine, which caused a reconcile storm under a fast simulator; that lesson is documented in ADR 007
