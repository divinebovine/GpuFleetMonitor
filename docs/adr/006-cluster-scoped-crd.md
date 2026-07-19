# 006 - GPUHealth CRD as Cluster-Scoped

## Status
Accepted

## Context
When defining the `GPUHealth` CRD, scope had to be chosen: namespace-scoped or cluster-scoped.

GPUs are node-attached hardware. Kubernetes nodes are cluster-scoped resources. A namespace-scoped `GPUHealth` would require either picking an arbitrary namespace (e.g. `gpu-system`) or establishing a convention for which namespace owns each GPU's CR. Cross-namespace references are awkward in Kubernetes - a controller acting on a node has no natural namespace to operate from, and `kubectl get gpuhealths` without a `-n` flag would return nothing, matching neither the UX of node commands nor the mental model of fleet-wide hardware.

Persistent volumes, storage classes, and nodes themselves are all cluster-scoped for the same reason: they represent physical or cluster-wide infrastructure that doesn't belong to any one namespace.

## Decision
Made `GPUHealth` cluster-scoped via `+kubebuilder:resource:scope=Cluster`. RBAC uses a `ClusterRole` and `ClusterRoleBinding`.

## Consequences
- `kubectl get gpuhealths` and `kubectl get gh` work without `-n`, matching the UX of `kubectl get nodes`
- A single `ClusterRole` covers all `GPUHealth` resources - no per-namespace role binding needed
- There is no namespace isolation: in a multi-tenant cluster, all tenants with read access to the CRD can see all GPU health objects; access control is purely at the ClusterRole level
