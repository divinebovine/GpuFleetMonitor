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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/divinebovine/GpuFleetMonitor/api/v1alpha1"

	gpuModel "github.com/divinebovine/GpuFleetMonitor/internal/gpu"
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
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;update;patch
func (r *GPUHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var gpuCR v1alpha1.GPUHealth
	if err := r.Get(ctx, req.NamespacedName, &gpuCR); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	resp, err := http.Get(fmt.Sprintf("%s/v1/gpus/%s", r.TelemetryURL, gpuCR.Spec.GPUID))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("fetching telemetry for %s: %w", gpuCR.Spec.GPUID, err)
	} else if resp.StatusCode != http.StatusOK {
		return ctrl.Result{}, fmt.Errorf("fetching telemetry for %s failed Status Code: %d, Status: %s", gpuCR.Spec.GPUID, resp.StatusCode, resp.Status)
	}

	defer resp.Body.Close()
	var telemetry gpuModel.GPUHealth
	if err := json.NewDecoder(resp.Body).Decode(&telemetry); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed decoding telemetry for %s: %w", gpuCR.Spec.GPUID, err)
	}

	var result ctrl.Result
	switch gpuCR.Status.Phase {
	case v1alpha1.PhaseFailed:
		result, err = r.handleFailed()
	case v1alpha1.PhaseDraining:
		result, err = r.handleDraining(ctx, &gpuCR)
	case v1alpha1.PhaseRecovering:
		result, err = r.handleRecovering(ctx, &gpuCR, telemetry)
	case v1alpha1.PhaseReplacing:
		result, err = r.handleReplacing(ctx, &gpuCR)
	case v1alpha1.PhaseRejoining:
		result, err = r.handleRejoining(ctx, &gpuCR, telemetry)
	default:
		result, err = r.handleObserving(ctx, &gpuCR, telemetry)
	}

	log.Info("reconciled", "gpu", gpuCR.Spec.GPUID, "phase", gpuCR.Status.Phase)
	return result, err
}

func (r *GPUHealthReconciler) handleFailed() (ctrl.Result, error) {
	// noop requeue slowly
	return ctrl.Result{RequeueAfter: 120 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleDraining(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	// enforce draining
	if _, err := r.cordonNode(ctx, gpuCR); err != nil {
		return ctrl.Result{}, err
	}

	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.MatchingFields{"spec.nodeName": gpuCR.Spec.NodeName}); err != nil {
		return ctrl.Result{}, err
	}

	// filter pods removing DaemonSets and pods that have aleady succeeded or failed
	podList.Items = slices.DeleteFunc(podList.Items, func(pod corev1.Pod) bool {
		daemonSet := slices.ContainsFunc(pod.OwnerReferences, func(ownerRef metav1.OwnerReference) bool {
			return ownerRef.Kind == "DaemonSet"
		})
		return daemonSet ||
			pod.Status.Phase == corev1.PodSucceeded ||
			pod.Status.Phase == corev1.PodFailed
	})

	if len(podList.Items) > 0 {
		// still draining check back again quicker than usual
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	old := gpuCR.Status.DeepCopy()
	gpuCR.Status.Phase = v1alpha1.PhaseRecovering
	newC := metav1.Condition{
		Type:               v1alpha1.ConditionRemediationInProgress,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gpuCR.Generation,
		Reason:             "GPUDraining",
		Message:            "GPU node drain complete, entering recovery",
	}

	if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleRecovering(ctx context.Context, gpuCR *v1alpha1.GPUHealth, telemetry gpuModel.GPUHealth) (ctrl.Result, error) {
	// recovering is driven by telemetry
	var newC metav1.Condition
	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		// Not good, go back to remediation
		return r.handleRemediation(ctx, gpuCR, telemetry)
	case gpuModel.StatusWarning:
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.Phase = v1alpha1.PhaseRecovering
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             "GPURecovering",
			Message:            "GPU is recovering",
		}
		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
			return ctrl.Result{}, err
		}
	default:
		if _, err := r.uncordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.Phase = v1alpha1.PhaseHealthy
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             "GPUHealthy",
			Message:            "GPU is Healthy",
		}
		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleReplacing(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (ctrl.Result, error) {
	// ensure the node is still cordoned
	node, err := r.cordonNode(ctx, gpuCR)
	if err != nil {
		return ctrl.Result{}, err
	}

	ready := slices.ContainsFunc(node.Status.Conditions, func(c corev1.NodeCondition) bool {
		return c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue
	})

	if !ready {
		// node has transitioned from being ready to now being not ready
		// this is the signal that the node has gone down for replacement
		if gpuCR.Status.ReplacementStartedAt == nil {
			ts := metav1.NewTime(time.Now())
			gpuCR.Status.ReplacementStartedAt = &ts
			gpuCR.Status.ObservedGeneration = gpuCR.Generation
			if err := r.Status().Update(ctx, gpuCR); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if ready && gpuCR.Status.ReplacementStartedAt != nil {
		// node has transitioned from not being ready to now being ready
		// this is the signal that the node has been replaced
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.ReplacementStartedAt = nil
		gpuCR.Status.Phase = v1alpha1.PhaseRejoining
		newC := metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             "GPURejoining",
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

func (r *GPUHealthReconciler) handleRejoining(ctx context.Context, gpuCR *v1alpha1.GPUHealth, telemetry gpuModel.GPUHealth) (ctrl.Result, error) {
	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		return r.handleRemediation(ctx, gpuCR, telemetry)
	case gpuModel.StatusWarning:
		// give the node time to settle
		return ctrl.Result{RequeueAfter: 120 * time.Second}, nil
	default:
		if _, err := r.uncordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		old := gpuCR.Status.DeepCopy()
		gpuCR.Status.Phase = v1alpha1.PhaseHealthy
		newC := metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUHealthy",
			Message:            "GPU is operating within normal parameters",
		}
		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleObserving(ctx context.Context, gpuCR *v1alpha1.GPUHealth, telemetry gpuModel.GPUHealth) (ctrl.Result, error) {
	old := gpuCR.Status.DeepCopy()
	var newPhase v1alpha1.GPUPhase
	var newC metav1.Condition
	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		return r.handleRemediation(ctx, gpuCR, telemetry)
	case gpuModel.StatusWarning:
		newPhase = v1alpha1.PhaseWarning
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionFalse,
			Reason:             "GPUDegraded",
			Message:            "GPU metrics are outside normal parameters",
		}
	default:
		gpuCR.Status.RemediationAttempts = 0
		newPhase = v1alpha1.PhaseHealthy
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUOperational",
			Message:            "GPU is operating within normal parameters",
		}
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

	var newC metav1.Condition
	if gpuCR.Status.RemediationAttempts >= gpuCR.Spec.MaxRemediationAttempts {
		// transition to failed
		gpuCR.Status.Phase = v1alpha1.PhaseFailed
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUFailed",
			Message:            "GPU has failed and requires replacement",
		}

		if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
		}

		return r.handleFailed()
	}

	switch gpuCR.Spec.RemediationPolicy {
	case v1alpha1.RemediationPolicyDrain:
		if _, err := r.cordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		gpuCR.Status.Phase = v1alpha1.PhaseDraining
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUCritical",
			Message:            "GPU is experiencing critical errors and requires draining",
		}
	case v1alpha1.RemediationPolicyReplace:
		if _, err := r.cordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		gpuCR.Status.Phase = v1alpha1.PhaseReplacing
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUCritical",
			Message:            "GPU is experiencing critical errors and requires replacing",
		}
		gpuCR.Status.Findings = mergeFindings(gpuCR.Status.Findings, telemetry, 100)
	case v1alpha1.RemediationPolicyEscalate:
		gpuCR.Status.Phase = v1alpha1.PhaseCritical
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUCritical",
			Message:            "GPU is experiencing critical errors and requires escalating the issue",
		}
	default:
		// v1alpha1.RemediationPolicyNone
		gpuCR.Status.Phase = v1alpha1.PhaseCritical
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionFalse,
			Reason:             "GPUCritical",
			Message:            "GPU is experiencing critical errors",
		}
	}

	if err := r.syncStatus(ctx, gpuCR, old, newC); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) cordonNode(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (corev1.Node, error) {
	// cordon the node by making it unschedulable allowing it to drain
	var node corev1.Node
	if err := r.Get(ctx, client.ObjectKey{Name: gpuCR.Spec.NodeName}, &node); err != nil {
		return corev1.Node{}, err
	}

	if !node.Spec.Unschedulable {
		node.Spec.Unschedulable = true
		if err := r.Update(ctx, &node); err != nil {
			return corev1.Node{}, err
		}
	}

	return node, nil
}

func (r *GPUHealthReconciler) uncordonNode(ctx context.Context, gpuCR *v1alpha1.GPUHealth) (corev1.Node, error) {
	// uncordon the node by making it schedulable allowing it to recover
	var node corev1.Node
	if err := r.Get(ctx, client.ObjectKey{Name: gpuCR.Spec.NodeName}, &node); err != nil {
		return corev1.Node{}, err
	}

	if node.Spec.Unschedulable {
		node.Spec.Unschedulable = false
		if err := r.Update(ctx, &node); err != nil {
			return corev1.Node{}, err
		}
	}

	return node, nil
}

// syncStatus applies newCondition to the CR's status conditions, then writes the
// full status to the API server only if it has changed since old was snapshotted.
// Callers must capture old = gpuCR.Status.DeepCopy() before mutating any status
// fields so the diff is accurate.
func (r *GPUHealthReconciler) syncStatus(ctx context.Context, gpuCR *v1alpha1.GPUHealth, old *v1alpha1.GPUHealthStatus, newCondition metav1.Condition) error {
	meta.SetStatusCondition(&gpuCR.Status.Conditions, newCondition)

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
	if telemetry.Temperature.GPUCoreCelsius >= telemetry.Temperature.GPUCoreCriticalThreshold {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingGPUThermalThrottle,
			Severity: v1alpha1.SeverityCritical,
			Message:  "GPU core temperature critical",
		})
	} else if telemetry.Temperature.GPUCoreCelsius >= telemetry.Temperature.GPUCoreWarningThreshold {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingGPUThermalThrottle,
			Severity: v1alpha1.SeverityWarning,
			Message:  "GPU core temperature elevated",
		})
	}

	if telemetry.Temperature.MemoryCelsius >= telemetry.Temperature.MemoryCriticalThreshold {
		candidates = append(candidates, v1alpha1.Finding{
			Type:     v1alpha1.FindingMemoryThermalThrottle,
			Severity: v1alpha1.SeverityCritical,
			Message:  "Memory temperature critical",
		})
	} else if telemetry.Temperature.MemoryCelsius >= telemetry.Temperature.MemoryWarningThreshold {
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
				// otherwise increment the existing count
				f.Count++
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
