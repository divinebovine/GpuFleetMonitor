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

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/divinebovine/GpuFleetMonitor/api/v1alpha1"

	gpuModel "github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

const (
	reasonGPUCritical    = "GPUCritical"
	reasonGPURecovering  = "GPURecovering"
	reasonGPUHealthy     = "GPUHealthy"
	reasonGPURejoining   = "GPURejoining"
	reasonGPUDegraded    = "GPUDegraded"
	reasonGPUOperational = "GPUOperational"
	reasonGPUFailed      = "GPUFailed"
)

// GPUHealthReconciler reconciles a GPUHealth object
type GPUHealthReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	TelemetryURL string
}

// +kubebuilder:rbac:groups=gpu.nvidia.com,resources=gpuhealths,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gpu.nvidia.com,resources=gpuhealths/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gpu.nvidia.com,resources=gpuhealths/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;update;patch
func (r *GPUHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var gpuCR v1alpha1.GPUHealth
	err := r.Get(ctx, req.NamespacedName, &gpuCR)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var result ctrl.Result
	observingPhases := []v1alpha1.GPUPhase{
		v1alpha1.PhaseNewCR,
		v1alpha1.PhaseHealthy,
		v1alpha1.PhaseWarning,
		v1alpha1.PhaseCritical,
	}

	if slices.Contains(observingPhases, gpuCR.Status.Phase) {
		result, err = r.handleObserving(ctx, &gpuCR)
	} else {
		switch gpuCR.Status.Phase {
		case v1alpha1.PhaseFailed:
			result, err = r.handleFailed(ctx, &gpuCR)
		case v1alpha1.PhaseDraining:
			result, err = r.handleDraining(ctx, &gpuCR)
		case v1alpha1.PhaseRecovering:
			result, err = r.handleRecovering(ctx, &gpuCR)
		case v1alpha1.PhaseReplacing:
			result, err = r.handleReplacing(ctx, &gpuCR)
		case v1alpha1.PhaseRejoining:
			result, err = r.handleRejoining(ctx, &gpuCR)
		default:
			return ctrl.Result{}, fmt.Errorf("unexpected phase encountered: %s", gpuCR.Status.Phase)
		}
	}

	logger.Info("reconciled", "gpu", gpuCR.Spec.GPUID, "phase", gpuCR.Status.Phase)
	return result, err
}

func (r *GPUHealthReconciler) handleFailed(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	if gpuCR.Status.ObservedGeneration < gpuCR.Generation {
		// spec has changed, and since the previous spec is not saved somewhere to compare against
		// and attempting to store the previous spec is more trouble than its worth for this simulation
		// then it is safe to assume that this gives the green light to restart remediation
		logger := log.FromContext(ctx)
		logger.Info("spec has changed returning gpu to service", "gpu", gpuCR.Spec.GPUID)

		return r.returnFailedToService(ctx, gpuCR, "GPU node transitioned from failed to healthy due to spec change")
	}

	telemetry, err := r.fetchTelemetry(ctx, gpuCR)

	if err != nil {
		return ctrl.Result{}, err
	}

	if telemetry.HealthStatus == gpuModel.StatusHealthy {
		// The gpu somehow entered the healthy state after it was set to failed
		// return it to service
		logger := log.FromContext(ctx)
		logger.Info("gpu transitioned from failed but telemetry indicates that it is healthy, returning it to service", "gpu", gpuCR.Spec.GPUID)

		return r.returnFailedToService(ctx, gpuCR, "GPU node transitioned from failed to healthy based on telemetry")
	}

	// node is still in a failed state
	// check back in slowly and see if a spec change has occurred or the gpu goes healthy on its own.
	return ctrl.Result{RequeueAfter: 120 * time.Second}, nil
}

func (r *GPUHealthReconciler) returnFailedToService(ctx context.Context, gpuCR *v1alpha1.GPUHealth, message string) (ctrl.Result, error) {
	gpuCR.Status.RemediationAttempts = 0 // reset attempts
	gpuCR.Status.Phase = v1alpha1.PhaseHealthy
	meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionGPUHealthy,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gpuCR.Generation,
		Reason:             reasonGPUHealthy,
		Message:            message,
	})

	if err := r.syncStatus(ctx, gpuCR); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}

	if err := r.uncordonNode(ctx, gpuCR); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed uncordoning %s", gpuCR.UID)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleDraining(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	if isStillDraining, err := r.drainNode(ctx, gpuCR); err != nil {
		return ctrl.Result{}, err
	} else if isStillDraining {
		// check again a little quicker than usual
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger := log.FromContext(ctx)
	if err := r.recoverGPU(ctx, gpuCR); err != nil {
		if errors.Is(err, gpuModel.ErrGPUUnrecoverable) {
			// Drain revealed a hardware failure that workload eviction cannot fix.
			// Escalate directly to hardware replacement without cycling back through
			// the remediation attempt counter — the drain itself was the attempt.
			logger.Info("GPU has unrecoverable hardware failure, escalating drain to replacement", "gpu", gpuCR.Spec.GPUID)
			gpuCR.Status.Phase = v1alpha1.PhaseReplacing
			meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
				Type:               v1alpha1.ConditionRemediationInProgress,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: gpuCR.Generation,
				Reason:             reasonGPUCritical,
				Message:            "GPU drain revealed unrecoverable hardware failure, escalating to replacement",
			})
			if err := r.syncStatus(ctx, gpuCR); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		logger.Error(err, "failed to trigger simulation recovery after drain", "gpu", gpuCR.Spec.GPUID)
	}

	gpuCR.Status.Phase = v1alpha1.PhaseRecovering
	meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionRemediationInProgress,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gpuCR.Generation,
		Reason:             reasonGPURecovering,
		Message:            "GPU node drain complete, entering recovery",
	})

	if err := r.syncStatus(ctx, gpuCR); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleRecovering(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	telemetry, err := r.fetchTelemetry(ctx, gpuCR)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		// Not good, remediation failed, try again
		return r.handleRemediation(ctx, gpuCR)
	case gpuModel.StatusWarning:
		gpuCR.Status.Phase = v1alpha1.PhaseRecovering
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             reasonGPURecovering,
			Message:            "GPU is recovering",
		})

		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
	case gpuModel.StatusHealthy:
		if err := r.uncordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		gpuCR.Status.Phase = v1alpha1.PhaseHealthy
		gpuCR.Status.RemediationAttempts = 0
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             reasonGPUHealthy,
			Message:            "GPU is Healthy",
		})
		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unexpected health status returned by telemetry: %s", telemetry.HealthStatus)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleReplacing(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	// ensure the node is still cordoned
	if err := r.cordonNode(ctx, gpuCR); err != nil {
		return ctrl.Result{}, err
	}

	ready, err := r.isNodeReady(ctx, gpuCR)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		if gpuCR.Status.ReplacementStartedAt == nil {
			// node has transitioned from being ready to now being not ready
			// this is the signal that the node has gone down for replacement
			ts := metav1.NewTime(time.Now())
			gpuCR.Status.ReplacementStartedAt = &ts
			if err := r.syncStatus(ctx, gpuCR); err != nil {
				return ctrl.Result{}, err
			}
		}

		deadline := gpuCR.Status.ReplacementStartedAt.Add(
			time.Duration(gpuCR.Spec.ReplacementTimeoutSeconds) * time.Second)

		if time.Now().After(deadline) {
			// this remediation attempt has failed, go back to remediation
			gpuCR.Status.ReplacementStartedAt = nil
			return r.handleRemediation(ctx, gpuCR)
		}

		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	logger := log.FromContext(ctx)

	if gpuCR.Status.ReplacementStartedAt != nil {
		// node has transitioned from not being not ready to ready with a
		// replacementStartedAt timestamp, this is the signal that the node
		// has was taken down for replacement and it is now back
		if err := r.replaceGPU(ctx, gpuCR); err != nil {
			logger.Error(err, "failed to trigger simulation reset after hardware replacement", "gpu", gpuCR.Spec.GPUID)
		}
		gpuCR.Status.ReplacementStartedAt = nil
		gpuCR.Status.Phase = v1alpha1.PhaseRejoining
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             reasonGPURejoining,
			Message:            "GPU is rejoining",
		})

		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Node is ready with no replacement in progress. In a real cluster this is
	// where we wait for the hardware team to cycle the node. In the simulator
	// the replace API call is the entire replacement, so trigger it immediately.
	logger.Info("triggering simulated GPU hardware replacement", "gpu", gpuCR.Spec.GPUID)
	if err := r.replaceGPU(ctx, gpuCR); err != nil {
		logger.Error(err, "failed to trigger simulation reset after hardware replacement", "gpu", gpuCR.Spec.GPUID)
	}
	gpuCR.Status.Phase = v1alpha1.PhaseRejoining
	meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionRemediationInProgress,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gpuCR.Generation,
		Reason:             reasonGPURejoining,
		Message:            "GPU hardware replaced, rejoining fleet",
	})
	if err := r.syncStatus(ctx, gpuCR); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleRejoining(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	telemetry, err := r.fetchTelemetry(ctx, gpuCR)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		return r.handleRemediation(ctx, gpuCR)
	case gpuModel.StatusWarning:
		if gpuCR.Status.LastTransitionTime == nil {
			// Should not happen unless a manually created CR is created with Phase=Rejoining and no LastTransitionTime
			// is specified. In case that happens, set lastTransitionTime to now
			err := fmt.Errorf("%s attempting to rejoin but is missing LastTransitionTime value", gpuCR.Spec.GPUID)
			logger := log.FromContext(ctx)
			logger.Error(err, "recoverable error: LastTransitionTime is nil, setting it to now")
			now := metav1.Now()
			gpuCR.Status.LastTransitionTime = &now
		}
		deadline := gpuCR.Status.LastTransitionTime.Add(time.Duration(gpuCR.Spec.RejoiningTimeoutSeconds) * time.Second)
		if deadline.After(time.Now()) {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// rejoining has timed out, move the node to failed
		gpuCR.Status.Phase = v1alpha1.PhaseFailed
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUFailed,
			Message:            "GPU has failed to rejoin within the allowed timeout",
		})
		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
	case gpuModel.StatusHealthy:
		if err := r.uncordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		gpuCR.Status.Phase = v1alpha1.PhaseHealthy
		gpuCR.Status.RemediationAttempts = 0
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUHealthy,
			Message:            "GPU is operating within normal parameters",
		})
		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unexpected health status returned by telemetry: %s", telemetry.HealthStatus)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleObserving(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	telemetry, err := r.fetchTelemetry(ctx, gpuCR)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		// There may be multiple attempts at remediation so findings are added before remediation begins to prevent duplication
		gpuCR.Status.Findings = mergeFindings(gpuCR.Status.Findings, telemetry, 100)
		return r.handleRemediation(ctx, gpuCR)
	case gpuModel.StatusWarning:
		gpuCR.Status.Phase = v1alpha1.PhaseWarning
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionFalse,
			Reason:             reasonGPUDegraded,
			Message:            "GPU metrics are outside normal parameters",
		})
		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	case gpuModel.StatusHealthy:
		gpuCR.Status.RemediationAttempts = 0
		gpuCR.Status.Phase = v1alpha1.PhaseHealthy
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUOperational,
			Message:            "GPU is operating within normal parameters",
		})
		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	default:
		return ctrl.Result{}, fmt.Errorf("unexpected health status returned by telemetry: %s", telemetry.HealthStatus)
	}
}

func (r *GPUHealthReconciler) handleRemediation(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	switch gpuCR.Spec.RemediationPolicy {
	case v1alpha1.RemediationPolicyDrain:
		return r.handleRemediationPolicyDrain(ctx, gpuCR)
	case v1alpha1.RemediationPolicyReplace:
		return r.handleRemediationPolicyReplacing(ctx, gpuCR)
	case v1alpha1.RemediationPolicyEscalate:
		return r.handleRemediationPolicyEscalate(ctx, gpuCR)
	case v1alpha1.RemediationPolicyNone:
		return r.handleRemediationPolicyNone(ctx, gpuCR)
	default:
		return ctrl.Result{}, fmt.Errorf("unexpected remediation policy: %s", gpuCR.Spec.RemediationPolicy)
	}
}

func (r *GPUHealthReconciler) handleRemediationPolicyDrain(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	if isStillDraining, err := r.drainNode(ctx, gpuCR); err != nil {
		return ctrl.Result{}, err
	} else if isStillDraining {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if gpuCR.Status.ObservedGeneration < gpuCR.Generation {
		// reset remediation policies on spec change
		gpuCR.Status.RemediationAttempts = 0
	}

	gpuCR.Status.RemediationAttempts++

	if gpuCR.Status.RemediationAttempts >= gpuCR.Spec.MaxRemediationAttempts {
		// transition to failed
		gpuCR.Status.Phase = v1alpha1.PhaseFailed
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUFailed,
			Message:            "GPU has failed and requires replacement",
		})

		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
		}

		return r.handleFailed(ctx, gpuCR)
	}

	gpuCR.Status.Phase = v1alpha1.PhaseDraining
	meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionRemediationInProgress,
		ObservedGeneration: gpuCR.Generation,
		Status:             metav1.ConditionTrue,
		Reason:             reasonGPUCritical,
		Message:            "GPU is experiencing critical errors and requires draining",
	})

	if err := r.syncStatus(ctx, gpuCR); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleRemediationPolicyReplacing(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	if err := r.cordonNode(ctx, gpuCR); err != nil {
		return ctrl.Result{}, err
	}

	if gpuCR.Status.ObservedGeneration < gpuCR.Generation {
		// reset remediation policies on spec change
		gpuCR.Status.RemediationAttempts = 0
	}

	gpuCR.Status.RemediationAttempts++

	if gpuCR.Status.RemediationAttempts >= gpuCR.Spec.MaxRemediationAttempts {
		// transition to failed
		gpuCR.Status.Phase = v1alpha1.PhaseFailed
		meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUFailed,
			Message:            "GPU has failed and requires replacement",
		})

		if err := r.syncStatus(ctx, gpuCR); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
		}

		return r.handleFailed(ctx, gpuCR)
	}

	gpuCR.Status.Phase = v1alpha1.PhaseReplacing
	meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionRemediationInProgress,
		ObservedGeneration: gpuCR.Generation,
		Status:             metav1.ConditionTrue,
		Reason:             reasonGPUCritical,
		Message:            "GPU is experiencing critical errors and requires replacing",
	})
	if err := r.syncStatus(ctx, gpuCR); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleRemediationPolicyEscalate sets EscalationRequired and pages a human without
// taking automated action. The node is not cordoned — workloads continue running
// until an operator decides how to proceed.
func (r *GPUHealthReconciler) handleRemediationPolicyEscalate(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	gpuCR.Status.Phase = v1alpha1.PhaseCritical
	meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionEscalationRequired,
		ObservedGeneration: gpuCR.Generation,
		Status:             metav1.ConditionTrue,
		Reason:             reasonGPUCritical,
		Message:            "GPU is experiencing critical errors and requires escalating the issue",
	})

	if err := r.syncStatus(ctx, gpuCR); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleRemediationPolicyNone records that the GPU is critical and continues
// observing without taking any automated action. The node is not cordoned and
// no remediation attempts are counted. Use this policy when human review is
// preferred over automated intervention.
func (r *GPUHealthReconciler) handleRemediationPolicyNone(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	gpuCR.Status.Phase = v1alpha1.PhaseCritical
	meta.SetStatusCondition(&gpuCR.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionGPUHealthy,
		ObservedGeneration: gpuCR.Generation,
		Status:             metav1.ConditionFalse,
		Reason:             reasonGPUCritical,
		Message:            "GPU is experiencing critical errors",
	})
	if err := r.syncStatus(ctx, gpuCR); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// isNodeReady reports whether the Node hosting the GPU referenced by gpuCR has
// a Ready condition with status True. It returns an error if the Node cannot
// be fetched, e.g. because it does not exist.
func (r *GPUHealthReconciler) isNodeReady(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (bool, error) {
	var node corev1.Node
	if err := r.Get(ctx, client.ObjectKey{Name: gpuCR.Spec.NodeName}, &node); err != nil {
		return false, err
	}

	ready := slices.ContainsFunc(node.Status.Conditions, func(c corev1.NodeCondition) bool {
		return c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue
	})
	return ready, nil
}

// cordonNode marks the node hosting this GPU as unschedulable, preventing new pods from being scheduled on it.
// It is idempotent — if the node is already cordoned, no update is made.
func (r *GPUHealthReconciler) cordonNode(ctx context.Context, gpuCR *v1alpha1.GPUHealth) error {
	var node corev1.Node
	if err := r.Get(ctx, client.ObjectKey{Name: gpuCR.Spec.NodeName}, &node); err != nil {
		return err
	}

	if !node.Spec.Unschedulable {
		node.Spec.Unschedulable = true
		if err := r.Update(ctx, &node); err != nil {
			// A conflict means another GPU reconciler on the same node already
			// cordoned it. The node will be unschedulable either way.
			if !apierrors.IsConflict(err) {
				return err
			}
		}
	}

	return nil
}

// uncordonNode marks the node hosting this GPU as schedulable, allowing new pods to be scheduled on it.
// It is idempotent — if the node is already uncordoned, no update is made.
func (r *GPUHealthReconciler) uncordonNode(ctx context.Context, gpuCR *v1alpha1.GPUHealth) error {
	var node corev1.Node
	if err := r.Get(ctx, client.ObjectKey{Name: gpuCR.Spec.NodeName}, &node); err != nil {
		return err
	}

	if node.Spec.Unschedulable {
		node.Spec.Unschedulable = false
		if err := r.Update(ctx, &node); err != nil {
			if !apierrors.IsConflict(err) {
				return err
			}
		}
	}

	return nil
}

// drainNode ensures the node is cordoned and evicts all non-DaemonSet, non-terminal pods.
// It is idempotent and safe to call on every reconcile tick.
// Returns false, nil when the node is fully drained and the caller may advance the phase.
// Returns true, nil when evictions are in-flight and the caller should requeue.
// Returns true, err when the cordon or eviction failed and drain state is unknown.
func (r *GPUHealthReconciler) drainNode(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (bool, error) {
	// ensure the node is cordoned
	if err := r.cordonNode(ctx, gpuCR); err != nil {
		return true, err
	}

	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.MatchingFields{"spec.nodeName": gpuCR.Spec.NodeName}); err != nil {
		return true, err
	}

	// filter pods removing DaemonSets and pods that have already succeeded or failed
	podList.Items = slices.DeleteFunc(podList.Items, func(pod corev1.Pod) bool {
		daemonSet := slices.ContainsFunc(pod.OwnerReferences, func(ownerRef metav1.OwnerReference) bool {
			return ownerRef.Kind == "DaemonSet"
		})
		return daemonSet ||
			pod.Status.Phase == corev1.PodSucceeded ||
			pod.Status.Phase == corev1.PodFailed ||
			pod.Status.Phase == corev1.PodUnknown
	})

	if len(podList.Items) == 0 {
		// no pods remaining to evict
		return false, nil
	}

	for _, pod := range podList.Items {
		// remaining pods need to be evicted
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}

		if err := r.Create(ctx, eviction); err != nil {
			if apierrors.IsTooManyRequests(err) {
				// skip this pod and let the eviction be handled by the next tick
				continue
			} else if apierrors.IsNotFound(err) {
				// pod is already gone, so it doesn't matter
				continue
			}
			// this is unexpected, error needs to be surfaced
			return true, err
		}
	}

	// eviction requests are async, let the next tick determine if there are any left to evict
	return true, nil
}

// recoverGPU triggers the simulator to resolve a GPU's state after a successful drain.
// Returns gpuModel.ErrGPUUnrecoverable if the GPU has a hardware failure that drain cannot fix.
func (r *GPUHealthReconciler) recoverGPU(ctx context.Context, gpuCR *v1alpha1.GPUHealth) error {
	url := fmt.Sprintf("%s/v1/simulation/gpus/%s/recover", r.TelemetryURL, gpuCR.Spec.GPUID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	if err != nil {
		return fmt.Errorf("building recover request for %s: %w", gpuCR.Spec.GPUID, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("recover request for %s: %w", gpuCR.Spec.GPUID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("recover request for %s: %w", gpuCR.Spec.GPUID, gpuModel.ErrGPUUnrecoverable)
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("recover request for %s: unexpected status %d", gpuCR.Spec.GPUID, resp.StatusCode)
	}
	return nil
}

// replaceGPU triggers the simulator to reset a GPU's state after hardware replacement.
func (r *GPUHealthReconciler) replaceGPU(ctx context.Context, gpuCR *v1alpha1.GPUHealth) error {
	url := fmt.Sprintf("%s/v1/simulation/gpus/%s/replace", r.TelemetryURL, gpuCR.Spec.GPUID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	if err != nil {
		return fmt.Errorf("building replace request for %s: %w", gpuCR.Spec.GPUID, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("replace request for %s: %w", gpuCR.Spec.GPUID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("replace request for %s: unexpected status %d", gpuCR.Spec.GPUID, resp.StatusCode)
	}
	return nil
}

// fetchTelemetry calls the telemetry service and returns the current health snapshot
// for the GPU identified by gpuCR.Spec.GPUID. The request is bound to ctx so it
// respects cancellation and the reconcile deadline.
func (r *GPUHealthReconciler) fetchTelemetry(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (gpuModel.GPUHealth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/gpus/%s", r.TelemetryURL, gpuCR.Spec.GPUID), nil)
	if err != nil {
		return gpuModel.GPUHealth{}, fmt.Errorf("building telemetry request for %s: %w", gpuCR.Spec.GPUID, err)
	}

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return gpuModel.GPUHealth{}, fmt.Errorf("fetching telemetry for %s: %w", gpuCR.Spec.GPUID, err)
	}

	// defer after checking for an error so there's no panic
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return gpuModel.GPUHealth{}, fmt.Errorf("fetching telemetry for %s failed Status Code: %d, Status: %s", gpuCR.Spec.GPUID, resp.StatusCode, resp.Status)
	}

	var telemetry gpuModel.GPUHealth
	if err := json.NewDecoder(resp.Body).Decode(&telemetry); err != nil {
		return gpuModel.GPUHealth{}, fmt.Errorf("failed decoding telemetry for %s: %w", gpuCR.Spec.GPUID, err)
	}

	return telemetry, nil
}

// syncStatus writes gpuCR's status to the API server only if it differs from
// the live state in the cache. Sets ObservedGeneration and LastTransitionTime
// automatically so callers only need to mutate the fields they care about.
func (r *GPUHealthReconciler) syncStatus(ctx context.Context, gpuCR *v1alpha1.GPUHealth) error {
	gpuCR.Status.ObservedGeneration = gpuCR.Generation

	// looking up the cached gpuCR for comparison is very cheap and much better than trying
	// to capture at the top of the reconcile loop and passing around everywhere
	var live v1alpha1.GPUHealth
	if err := r.Get(ctx, client.ObjectKeyFromObject(gpuCR), &live); err != nil {
		return err
	}

	if reflect.DeepEqual(live.Status, gpuCR.Status) {
		return nil
	}

	// on phase change, capture the transition time
	if live.Status.Phase != gpuCR.Status.Phase {
		now := metav1.Now()
		gpuCR.Status.LastTransitionTime = &now
	}

	return r.Status().Update(ctx, gpuCR)
}

// mergeFindings derives findings from telemetry and merges them into the existing slice.
// Existing findings are deduplicated by Type and moved to the tail on each match.
// For ECC findings, Count is taken directly from the telemetry hardware counter and
// Severity and Message are preserved. For observation-based findings (temperature, power),
// Count is incremented and Severity and Message are updated to reflect the latest reading.
// New findings are appended with Count initialized to 1.
// The returned slice is capped at maxSize, dropping the oldest entries first.
func mergeFindings(existing []v1alpha1.Finding, telemetry gpuModel.GPUHealth, maxSize int) []v1alpha1.Finding {
	ts := metav1.NewTime(time.Now())

	var candidates []v1alpha1.Finding

	// ECC bit errors are counters that only increase while a GPU is running, once an error happens, it
	// will always be reported. Therefore the count will always be the number of errors that are
	// reported by telemetry.
	if count := telemetry.Memory.ECCSingleBitErrors; count >= 6 {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingECCSingleBitError,
			Severity: v1alpha1.SeverityWarning,
			Message:  "ECC single-bit errors detected",
			Count:    int32(count),
		})
	}

	if count := telemetry.Memory.ECCDoubleBitErrors; count > 0 {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingECCDoubleBitError,
			Severity: v1alpha1.SeverityCritical,
			Message:  "ECC double-bit errors detected, immediate action required",
			Count:    int32(count),
		})
	}

	// Temperature and power capped are not counters, therefore the count will increment on every report
	if telemetry.Temperature.GPUCoreCriticalThreshold > 0 &&
		telemetry.Temperature.GPUCoreCelsius >= telemetry.Temperature.GPUCoreCriticalThreshold {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingGPUThermalThrottle,
			Severity: v1alpha1.SeverityCritical,
			Message:  "GPU core temperature critical",
		})
	} else if telemetry.Temperature.GPUCoreWarningThreshold > 0 &&
		telemetry.Temperature.GPUCoreCelsius >= telemetry.Temperature.GPUCoreWarningThreshold {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingGPUThermalThrottle,
			Severity: v1alpha1.SeverityWarning,
			Message:  "GPU core temperature elevated",
		})
	}

	if telemetry.Temperature.MemoryCriticalThreshold > 0 &&
		telemetry.Temperature.MemoryCelsius >= telemetry.Temperature.MemoryCriticalThreshold {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingMemoryThermalThrottle,
			Severity: v1alpha1.SeverityCritical,
			Message:  "Memory temperature critical",
		})
	} else if telemetry.Temperature.MemoryWarningThreshold > 0 &&
		telemetry.Temperature.MemoryCelsius >= telemetry.Temperature.MemoryWarningThreshold {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingMemoryThermalThrottle,
			Severity: v1alpha1.SeverityWarning,
			Message:  "Memory temperature elevated",
		})
	}

	if telemetry.Power.PowerCapped {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingPowerCapped,
			Severity: v1alpha1.SeverityCritical,
			Message:  "GPU power capped",
		})
	}

	// merge any that already exist and move them to the tail so they don't drop off
	for _, c := range candidates {
		index := slices.IndexFunc(existing, func(f v1alpha1.Finding) bool {
			return f.Type == c.Type
		})

		if index >= 0 {
			f := existing[index]
			if c.Count > 0 {
				// if count was set, then its a counter managed by telemetry
				// like ECC memory errors
				f.Count = c.Count
			} else {
				// otherwise increment the existing count and update
				// the severity and message in case they change
				f.Count++
				f.Severity = c.Severity
				f.Message = c.Message
			}
			f.ObservedAt = ts
			existing = append(existing[:index], existing[index+1:]...)
			existing = append(existing, f)
		} else {
			existing = append(existing, v1alpha1.Finding{
				Type:       c.Type,
				Severity:   c.Severity,
				Message:    c.Message,
				Count:      max(1, c.Count),
				ObservedAt: ts,
			})
		}
	}

	// trim slice to max length
	if len(existing) > maxSize {
		return existing[len(existing)-maxSize:]
	}
	return existing
}

// SetupWithManager sets up the controller with the Manager.
func (r *GPUHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// use predicate.GenerationChangedPredicate to filter status updates so they don't fire another reconcile loop
		For(&v1alpha1.GPUHealth{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("gpuhealth").
		Complete(r)
}
