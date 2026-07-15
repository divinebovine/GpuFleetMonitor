/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GPUHealthSpec defines the desired state of GPUHealth.
type GPUHealthSpec struct {
	// nodeName is the Kubernetes node that hosts this GPU.
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// gpuID is the hardware identifier of the GPU (e.g. "GPU-00001").
	// +kubebuilder:validation:Required
	GPUID string `json:"gpuID"`

	// remediationPolicy controls how the operator responds when this GPU enters a critical state.
	// +kubebuilder:default=None
	RemediationPolicy RemediationPolicy `json:"remediationPolicy"`

	// maxRemediationAttempts is the number of automated remediation cycles to attempt
	// before escalating to human intervention, regardless of remediationPolicy.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	// +optional
	MaxRemediationAttempts int32 `json:"maxRemediationAttempts,omitempty"`
}

// RemediationPolicy defines how the operator responds to a GPU entering a critical state.
// +kubebuilder:validation:Enum=None;Drain;Replace;Escalate
type RemediationPolicy string

const (
	// RemediationPolicyNone observes the GPU but takes no automated action.
	RemediationPolicyNone RemediationPolicy = "None"
	// RemediationPolicyDrain cordons the node and drains workloads, then waits for recovery.
	RemediationPolicyDrain RemediationPolicy = "Drain"
	// RemediationPolicyReplace drains the node and marks the GPU for hardware replacement.
	RemediationPolicyReplace RemediationPolicy = "Replace"
	// RemediationPolicyEscalate pages an operator immediately without attempting automated remediation.
	RemediationPolicyEscalate RemediationPolicy = "Escalate"
)

// GPUHealthStatus defines the observed state of GPUHealth.
type GPUHealthStatus struct {
	// phase is the current position of this GPU in the health state machine.
	// +optional
	Phase GPUPhase `json:"phase,omitempty"`

	// conditions reflect the current state of the GPUHealth resource using
	// standard Kubernetes condition types.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// findings contains diagnostic observations recorded by the operator.
	// Capped at 100 entries; oldest findings are evicted when the limit is reached.
	// +kubebuilder:validation:MaxItems=100
	// +optional
	Findings []Finding `json:"findings,omitempty"`

	// remediationAttempts tracks how many automated remediation cycles have been attempted.
	// When this reaches spec.maxRemediationAttempts, the operator sets EscalationRequired.
	// +optional
	RemediationAttempts int32 `json:"remediationAttempts,omitempty"`

	// lastTransitionTime is when the phase last changed.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// observedGeneration is the .metadata.generation that this status was computed from.
	// Used to detect whether status is stale relative to the current spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// GPUPhase represents where a GPU sits in the health state machine.
//
// State transitions:
//
//	Healthy -> Warning -> Critical -> Draining -> Recovering -> Healthy
//	                           |
//	                           |-> Replacing -> Rejoining -> Healthy
//	                           |
//	                           |-> Failed  (human intervention required)
//
// +kubebuilder:validation:Enum=Healthy;Warning;Critical;Draining;Recovering;Replacing;Rejoining;Failed
type GPUPhase string

const (
	// PhaseHealthy indicates the GPU is operating within normal parameters.
	PhaseHealthy GPUPhase = "Healthy"
	// PhaseWarning indicates metrics are drifting (thermal, power, single-bit ECC errors).
	PhaseWarning GPUPhase = "Warning"
	// PhaseCritical indicates the GPU has serious errors (XID, double-bit ECC) requiring action.
	PhaseCritical GPUPhase = "Critical"
	// PhaseDraining indicates the operator has cordoned the node and is migrating workloads.
	PhaseDraining GPUPhase = "Draining"
	// PhaseRecovering indicates the drain is complete and diagnostics are running.
	PhaseRecovering GPUPhase = "Recovering"
	// PhaseReplacing indicates diagnostics failed and hardware replacement is in progress.
	PhaseReplacing GPUPhase = "Replacing"
	// PhaseRejoining indicates replacement is complete and the GPU is being re-validated.
	PhaseRejoining GPUPhase = "Rejoining"
	// PhaseFailed indicates automated remediation was exhausted; human intervention required.
	PhaseFailed GPUPhase = "Failed"
)

// Condition type constants for GPUHealth resources.
const (
	// ConditionGPUHealthy is True when the GPU is functioning within normal parameters.
	ConditionGPUHealthy = "GPUHealthy"
	// ConditionRemediationInProgress is True while the operator is actively remediating.
	ConditionRemediationInProgress = "RemediationInProgress"
	// ConditionEscalationRequired is True when automated remediation has been exhausted.
	ConditionEscalationRequired = "EscalationRequired"
)

// Finding records a specific diagnostic observation made by the operator.
type Finding struct {
	// type identifies the category of problem observed.
	Type FindingType `json:"type"`

	// severity indicates how serious this finding is.
	// +kubebuilder:validation:Enum=Warning;Critical
	Severity string `json:"severity"`

	// message is a human-readable description of the finding.
	Message string `json:"message"`

	// count is how many times this finding has been observed since it was first recorded.
	// +kubebuilder:validation:Minimum=1
	Count int32 `json:"count"`

	// observedAt is the timestamp when this finding was first recorded.
	ObservedAt metav1.Time `json:"observedAt"`
}

// FindingType identifies the category of a diagnostic observation.
// +kubebuilder:validation:Enum=XIDError;ECCSingleBitError;ECCDoubleBitError;ThermalThrottle;MemoryLeak;PowerCapped;Unknown
type FindingType string

const (
	FindingXIDError          FindingType = "XIDError"
	FindingECCSingleBitError FindingType = "ECCSingleBitError"
	FindingECCDoubleBitError FindingType = "ECCDoubleBitError"
	FindingThermalThrottle   FindingType = "ThermalThrottle"
	FindingMemoryLeak        FindingType = "MemoryLeak"
	FindingPowerCapped       FindingType = "PowerCapped"
	FindingUnknown           FindingType = "Unknown"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=gh
// +kubebuilder:printcolumn:name="GPU ID",type="string",JSONPath=".spec.gpuID"
// +kubebuilder:printcolumn:name="Node",type="string",JSONPath=".spec.nodeName"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Policy",type="string",JSONPath=".spec.remediationPolicy"
// +kubebuilder:printcolumn:name="Attempts",type="integer",JSONPath=".status.remediationAttempts"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GPUHealth is the Schema for the gpuhealths API.
type GPUHealth struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of GPUHealth
	// +required
	Spec GPUHealthSpec `json:"spec"`

	// status defines the observed state of GPUHealth
	// +optional
	Status GPUHealthStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// GPUHealthList contains a list of GPUHealth
type GPUHealthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []GPUHealth `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &GPUHealth{}, &GPUHealthList{})
		return nil
	})
}
