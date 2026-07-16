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

	newC := metav1.Condition{
		Type:               v1alpha1.ConditionRemediationInProgress,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gpuCR.Generation,
		Reason:             "GPURecovering",
		Message:            "GPU is recovering",
	}

	if err := r.handleStatusUpdate(ctx, gpuCR, v1alpha1.PhaseRecovering, newC); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleRecovering(ctx context.Context, gpuCR *v1alpha1.GPUHealth, telemetry gpuModel.GPUHealth) (ctrl.Result, error) {
	// recovering is driven by telemetry
	var newPhase v1alpha1.GPUPhase
	var newC metav1.Condition
	switch telemetry.HealthStatus {
	case gpuModel.StatusCritical:
		// Not good, go back to remediation
		return r.handleRemediation(ctx, gpuCR, telemetry)
	case gpuModel.StatusWarning:
		newPhase = v1alpha1.PhaseRecovering
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             "GPURecovering",
			Message:            "GPU is recovering",
		}
	default:
		newPhase = v1alpha1.PhaseHealthy
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             "GPUHealthy",
			Message:            "GPU is Healthy",
		}

		if _, err := r.uncordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.handleStatusUpdate(ctx, gpuCR, newPhase, newC); err != nil {
		return ctrl.Result{}, err
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

	if !gpuCR.Status.NodeNotReady && !ready {
		// node has transitioned from being ready to now being not ready
		// this is the signal that the node has gone down for replacement
		gpuCR.Status.NodeNotReady = true
		gpuCR.Status.ObservedGeneration = gpuCR.Generation
		if err := r.Status().Update(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 120 * time.Second}, nil
	}

	if gpuCR.Status.NodeNotReady && ready {
		// node has transitioned from not being ready to now being ready
		// this is the signal that the node has been replaced
		gpuCR.Status.NodeNotReady = false
		newPhase := v1alpha1.PhaseRejoining
		newC := metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gpuCR.Generation,
			Reason:             "GPURejoining",
			Message:            "GPU is rejoining",
		}

		if err := r.handleStatusUpdate(ctx, gpuCR, newPhase, newC); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// awaiting node replacement
	return ctrl.Result{RequeueAfter: 120 * time.Second}, nil
}

func (r *GPUHealthReconciler) getFindings(telemetry gpuModel.GPUHealth) []v1alpha1.Finding {
	// TODO: Simulator does not produce XIDErrors or MemoryLeak and may be outside the scope of this simulator.
	// TODO: Research DGM Exporter when time permits
	var findings []v1alpha1.Finding

	if telemetry.Memory.ECCSingleBitErrors > 0 {
		findings = append(findings, v1alpha1.Finding{
			Type:     v1alpha1.FindingECCSingleBitError,
			Severity: v1alpha1.SeverityWarning,
			Message: fmt.Sprintf("ECC single-bit errors detected - %d errors observed (threshold: 1)",
				telemetry.Memory.ECCSingleBitErrors),
			Count:      1,
			ObservedAt: metav1.NewTime(time.Now()),
		})
	}

	if telemetry.Memory.ECCDoubleBitErrors > 0 {
		findings = append(findings, v1alpha1.Finding{
			Type:     v1alpha1.FindingECCDoubleBitError,
			Severity: v1alpha1.SeverityCritical,
			Message: fmt.Sprintf("ECC double-bit errors detected - %d errors observed, immediate action required",
				telemetry.Memory.ECCDoubleBitErrors),
			Count:      1,
			ObservedAt: metav1.NewTime(time.Now()),
		})
	}

	if telemetry.Temperature.GPUCoreCelsius >= telemetry.Temperature.CriticalThreshold {
		findings = append(findings, v1alpha1.Finding{
			Type:     v1alpha1.FindingThermalThrottle,
			Severity: v1alpha1.SeverityCritical,
			Message: fmt.Sprintf("GPU core temperature critical - %.1f°C exceeds critical threshold of %.1f°C",
				telemetry.Temperature.GPUCoreCelsius,
				telemetry.Temperature.CriticalThreshold),
			Count:      1,
			ObservedAt: metav1.NewTime(time.Now()),
		})
	} else if telemetry.Temperature.GPUCoreCelsius >= telemetry.Temperature.WarningThreshold {
		findings = append(findings, v1alpha1.Finding{
			Type:     v1alpha1.FindingThermalThrottle,
			Severity: v1alpha1.SeverityWarning,
			Message: fmt.Sprintf("GPU core temperature elevated - %.1f°C exceeds warning threshold of %.1f°C",
				telemetry.Temperature.GPUCoreCelsius,
				telemetry.Temperature.WarningThreshold),
			Count:      1,
			ObservedAt: metav1.NewTime(time.Now()),
		})
	}

	if telemetry.Power.PowerCapped {
		findings = append(findings, v1alpha1.Finding{
			Type:     v1alpha1.FindingPowerCapped,
			Severity: v1alpha1.SeverityCritical,
			Message: fmt.Sprintf("GPU power capped - drawing %.1fW against limit of %.1fW",
				telemetry.Power.DrawWatts,
				telemetry.Power.LimitWatts),
			Count:      1,
			ObservedAt: metav1.NewTime(time.Now()),
		})
	}

	return findings
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
		newPhase := v1alpha1.PhaseHealthy
		newC := metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUHealthy",
			Message:            "GPU is operating within normal parameters",
		}
		if err := r.handleStatusUpdate(ctx, gpuCR, newPhase, newC); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleObserving(ctx context.Context, gpuCR *v1alpha1.GPUHealth, telemetry gpuModel.GPUHealth) (ctrl.Result, error) {
	// observing is driven by telemetry
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
		newPhase = v1alpha1.PhaseHealthy
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUOperational",
			Message:            "GPU is operating within normal parameters",
		}
	}

	if err := r.handleStatusUpdate(ctx, gpuCR, newPhase, newC); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GPUHealthReconciler) handleRemediation(ctx context.Context, gpuCR *v1alpha1.GPUHealth, telemetry gpuModel.GPUHealth) (ctrl.Result, error) {
	var newPhase v1alpha1.GPUPhase
	var newC metav1.Condition

	if gpuCR.Status.ObservedGeneration < gpuCR.Generation {
		gpuCR.Status.RemediationAttempts = 0
	}

	gpuCR.Status.RemediationAttempts++
	if err := r.Status().Update(ctx, gpuCR); err != nil {
		return ctrl.Result{}, err
	}

	if gpuCR.Status.RemediationAttempts >= gpuCR.Spec.MaxRemediationAttempts {
		// transition to failed
		newPhase = v1alpha1.PhaseFailed
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUFailed",
			Message:            "GPU has failed and requires replacement",
		}

		if err := r.handleStatusUpdate(ctx, gpuCR, newPhase, newC); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpuCR.Spec.GPUID, err)
		}

		return r.handleFailed()
	}

	switch gpuCR.Spec.RemediationPolicy {
	case v1alpha1.RemediationPolicyDrain:
		newPhase = v1alpha1.PhaseDraining
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUCritical",
			Message:            "GPU is experiencing critical errors and requires draining",
		}

		if _, err := r.cordonNode(ctx, gpuCR); err != nil {
			return ctrl.Result{}, err
		}
	case v1alpha1.RemediationPolicyReplace:
		newPhase = v1alpha1.PhaseReplacing
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionRemediationInProgress,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUCritical",
			Message:            "GPU is experiencing critical errors and requires replacing",
		}

		// collect findings
		gpuCR.Status.Findings = append(gpuCR.Status.Findings, r.getFindings(telemetry)...)
	case v1alpha1.RemediationPolicyEscalate:
		newPhase = v1alpha1.PhaseCritical
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionEscalationRequired,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "GPUCritical",
			Message:            "GPU is experiencing critical errors and requires escalating the issue",
		}
	default:
		// v1alpha1.RemediationPolicyNone
		newPhase = v1alpha1.PhaseCritical
		newC = metav1.Condition{
			Type:               v1alpha1.ConditionGPUHealthy,
			ObservedGeneration: gpuCR.Generation,
			Status:             metav1.ConditionFalse,
			Reason:             "GPUCritical",
			Message:            "GPU is experiencing critical errors",
		}
	}

	if err := r.handleStatusUpdate(ctx, gpuCR, newPhase, newC); err != nil {
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

func (r *GPUHealthReconciler) handleStatusUpdate(ctx context.Context, gpuCR *v1alpha1.GPUHealth, newPhase v1alpha1.GPUPhase, newCondition metav1.Condition) error {
	if changed := meta.SetStatusCondition(&gpuCR.Status.Conditions, newCondition); changed {
		// set status only if it has changed
		gpuCR.Status.ObservedGeneration = gpuCR.Generation
		gpuCR.Status.Phase = newPhase
		return r.Status().Update(ctx, gpuCR)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GPUHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.GPUHealth{}).
		Named("gpuhealth").
		Complete(r)
}
