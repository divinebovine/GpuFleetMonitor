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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
	switch gpuCR.Status.Phase {
	case v1alpha1.PhaseFailed:
		result, err = r.handleFailed()
	case v1alpha1.PhaseDraining:
		result, err = r.handleDraining(ctx, &gpuCR)
	case v1alpha1.PhaseRecovering:
		result, err = r.handleRecovering(ctx, &gpuCR)
	case v1alpha1.PhaseReplacing:
		result, err = r.handleReplacing(ctx, &gpuCR)
	case v1alpha1.PhaseRejoining:
		result, err = r.handleRejoining(ctx, &gpuCR)
	default:
		result, err = r.handleObserving(ctx, &gpuCR)
	}

	logger.Info("reconciled", "gpu", gpuCR.Spec.GPUID, "phase", gpuCR.Status.Phase)
	return result, err
}

func (r *GPUHealthReconciler) handleFailed() (ctrl.Result, error) {
	// noop requeue slowly
	return ctrl.Result{RequeueAfter: 120 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleDraining(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	isStillDraining, err := r.drainNode(ctx, gpuCR)

	if err != nil {
		return ctrl.Result{}, err
	}

	if isStillDraining {
		// check again a little quicker than usual
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	old := gpuCR.Status.DeepCopy()
	gpuCR.Status.Phase = v1alpha1.PhaseRecovering
	newC := &metav1.Condition{
		Type:               v1alpha1.ConditionRemediationInProgress,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gpuCR.Generation,
		Reason:             reasonGPURecovering,
		Message:            "GPU node drain complete, entering recovery",
	}

	if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
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
		// Not good, go back to remediation
		// decrement remediation attempts so it is not double counted
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.RemediationAttempts--
		if err := r.syncStatus(ctx, gpuCR, old, nil); err != nil {
			return ctrl.Result{}, err
		}
		return r.handleRemediation(ctx, gpuCR, telemetry)
	case gpuModel.StatusWarning:
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.Phase = v1alpha1.PhaseRecovering
		newC := &metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             reasonGPURecovering,
			Message:            "GPU is recovering",
		}
		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
			return ctrl.Result{}, err
		}
	case gpuModel.StatusHealthy:
		if err := r.uncordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.Phase = v1alpha1.PhaseHealthy
		gpuCR.Status.RemediationAttempts = 0
		newC := &metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             reasonGPUHealthy,
			Message:            "GPU is Healthy",
		}
		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
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
			old := gpuCR.Status.DeepCopy()
			ts := metav1.NewTime(time.Now())
			gpuCR.Status.ReplacementStartedAt = &ts
			if err := r.syncStatus(ctx, gpuCR, old, nil); err != nil {
				return ctrl.Result{}, err
			}
		}

		deadline := gpuCR.Status.ReplacementStartedAt.Add(
			time.Duration(gpuCR.Spec.ReplacementTimeoutSeconds) * time.Second)

		if time.Now().After(deadline) {
			old := gpuCR.Status.DeepCopy()
			gpuCR.Status.Phase = v1alpha1.PhaseFailed
			newC := &metav1.Condition{
				Type:               v1alpha1.ConditionEscalationRequired,
				ObservedGeneration: gpuCR.Generation,
				Status:             metav1.ConditionTrue,
				Reason:             reasonGPUFailed,
				Message:            "GPU has failed and requires replacement",
			}

			if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
			}
			return r.handleFailed()
		}

		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if gpuCR.Status.ReplacementStartedAt != nil {
		// node has transitioned from not being not ready to ready
		// this is the signal that the node has been replaced
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.ReplacementStartedAt = nil
		gpuCR.Status.Phase = v1alpha1.PhaseRejoining
		newC := &metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             reasonGPURejoining,
			Message:            "GPU is rejoining",
		}

		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// awaiting node replacement
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleRejoining(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	telemetry, err := r.fetchTelemetry(ctx, gpuCR)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		return r.handleRemediation(ctx, gpuCR, telemetry)
	case gpuModel.StatusWarning:
		// give the node time to settle
		return ctrl.Result{RequeueAfter: 120 * time.Second}, nil
	case gpuModel.StatusHealthy:
		if err := r.uncordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.Phase = v1alpha1.PhaseHealthy
		gpuCR.Status.RemediationAttempts = 0
		newC := &metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUHealthy,
			Message:            "GPU is operating within normal parameters",
		}
		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
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

	old := gpuCR.Status.DeepCopy()
	var newPhase v1alpha1.GPUPhase
	var newC *metav1.Condition
	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		return r.handleRemediation(ctx, gpuCR, telemetry)
	case gpuModel.StatusWarning:
		newPhase = v1alpha1.PhaseWarning
		newC = &metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionFalse,
			Reason:             reasonGPUDegraded,
			Message:            "GPU metrics are outside normal parameters",
		}
	case gpuModel.StatusHealthy:
		gpuCR.Status.RemediationAttempts = 0
		newPhase = v1alpha1.PhaseHealthy
		newC = &metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUOperational,
			Message:            "GPU is operating within normal parameters",
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unexpected health status returned by telemetry: %s", telemetry.HealthStatus)
	}

	gpuCR.Status.Phase = newPhase
	if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleRemediation(ctx context.Context, gpuCR *v1alpha1.GPUHealth, telemetry gpuModel.GPUHealth) (ctrl.Result, error) {
	old := gpuCR.Status.DeepCopy()

	if gpuCR.Status.ObservedGeneration < gpuCR.Generation {
		gpuCR.Status.RemediationAttempts = 0
	}

	gpuCR.Status.RemediationAttempts++

	var newC *metav1.Condition
	if gpuCR.Status.RemediationAttempts >= gpuCR.Spec.MaxRemediationAttempts {
		// transition to failed
		gpuCR.Status.Phase = v1alpha1.PhaseFailed
		newC = &metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUFailed,
			Message:            "GPU has failed and requires replacement",
		}

		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
		}

		return r.handleFailed()
	}

	gpuCR.Status.Findings = mergeFindings(gpuCR.Status.Findings, telemetry, 100)

	switch gpuCR.Spec.RemediationPolicy {
	case v1alpha1.RemediationPolicyDrain:
		if err := r.cordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		gpuCR.Status.Phase = v1alpha1.PhaseDraining
		newC = &metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUCritical,
			Message:            "GPU is experiencing critical errors and requires draining",
		}
	case v1alpha1.RemediationPolicyReplace:
		if err := r.cordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		gpuCR.Status.Phase = v1alpha1.PhaseReplacing
		newC = &metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUCritical,
			Message:            "GPU is experiencing critical errors and requires replacing",
		}
	case v1alpha1.RemediationPolicyEscalate:
		gpuCR.Status.Phase = v1alpha1.PhaseCritical
		newC = &metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             reasonGPUCritical,
			Message:            "GPU is experiencing critical errors and requires escalating the issue",
		}
	case v1alpha1.RemediationPolicyNone:
		gpuCR.Status.Phase = v1alpha1.PhaseCritical
		newC = &metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionFalse,
			Reason:             reasonGPUCritical,
			Message:            "GPU is experiencing critical errors",
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unexpected remediation policy: %s", gpuCR.Spec.RemediationPolicy)
	}

	if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

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
			return err
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
			return err
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

// syncStatus applies newCondition to the CR's status conditions, then writes the
// full status to the API server only if it has changed since old was snapshotted.
// Callers must capture old = gpuCR.Status.DeepCopy() before mutating any status
// fields so the diff is accurate.
func (r *GPUHealthReconciler) syncStatus(ctx context.Context, gpuCR *v1alpha1.GPUHealth, old *v1alpha1.GPUHealthStatus, newCondition *metav1.Condition) error {
	if newCondition != nil {
		meta.SetStatusCondition(&gpuCR.Status.Conditions, *newCondition)
	}

	if old.Phase != gpuCR.Status.Phase {
		now := metav1.Now()
		gpuCR.Status.LastTransitionTime = &now
	}

	if reflect.DeepEqual(old, &gpuCR.Status) {
		return nil
	}

	gpuCR.Status.ObservedGeneration = gpuCR.Generation
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
	// TODO: Simulator does not produce XIDErrors or MemoryLeak and may be outside the scope of this simulator.
	// TODO: Research DGM Exporter when time permits

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
		For(&v1alpha1.GPUHealth{}).
		Named("gpuhealth").
		Complete(r)
}
