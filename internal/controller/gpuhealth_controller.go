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
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GPUHealth object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *GPUHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var gpu v1alpha1.GPUHealth
	err := r.Get(ctx, req.NamespacedName, &gpu)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	resp, err := http.Get(fmt.Sprintf("%s/v1/gpus/%s", r.TelemetryURL, gpu.Spec.GPUID))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("fetching telemetry for %s: %w", gpu.Spec.GPUID, err)
	}

	defer resp.Body.Close()
	var gpuHealthModel gpuModel.GPUHealth
	err = json.NewDecoder(resp.Body).Decode(&gpuHealthModel)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed decoding telemetry for %s: %w", gpu.Spec.GPUID, err)
	}

	existing := meta.FindStatusCondition(gpu.Status.Conditions, v1alpha1.ConditionGPUHealthy)
	newC := metav1.Condition{
		Type:               v1alpha1.ConditionGPUHealthy,
		ObservedGeneration: gpu.Generation,
	}
	switch gpuHealthModel.HealthStatus {
	case gpuModel.StatusCritical:
		gpu.Status.Phase = v1alpha1.PhaseCritical
		newC.Status = metav1.ConditionFalse
		newC.Reason = "GPUCritical"
		newC.Message = "GPU is experiencing critical errors and requires remediation"
	case gpuModel.StatusWarning:
		gpu.Status.Phase = v1alpha1.PhaseWarning
		newC.Status = metav1.ConditionFalse
		newC.Reason = "GPUDegraded"
		newC.Message = "GPU metrics are outside normal parameters"
	default:
		gpu.Status.Phase = v1alpha1.PhaseHealthy
		newC.Status = metav1.ConditionTrue
		newC.Reason = "GPUOperational"
		newC.Message = "GPU is operating within normal parameters"
	}

	// break loop when conditions haven't changed
	if existing == nil || existing.Reason != newC.Reason {
		gpu.Status.ObservedGeneration = gpu.Generation
		meta.SetStatusCondition(&gpu.Status.Conditions, newC)
		err = r.Status().Update(ctx, &gpu)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating status for %s: %w", gpu.Spec.GPUID, err)
		}
	}

	log.Info("reconciled", "gpu", gpu.Spec.GPUID, "phase", gpu.Status.Phase)

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GPUHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.GPUHealth{}).
		Named("gpuhealth").
		Complete(r)
}
