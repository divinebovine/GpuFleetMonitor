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
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlreconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/divinebovine/GpuFleetMonitor/api/v1alpha1"
	gpuModel "github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

const (
	testGPUID    = "GPU-00001"
	testNodeName = "node-00001"
)

// newTelemetryServer returns a test HTTP server that responds with the given health status.
func newTelemetryServer(status gpuModel.HealthStatus) *httptest.Server {
	return newTelemetryServerWithHealth(gpuModel.GPUHealth{
		GPUID:        testGPUID,
		HealthStatus: status,
	})
}

func newTelemetryServerWithHealth(health gpuModel.GPUHealth) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(health)
	}))
}

func newReconciler(serverURL string) *GPUHealthReconciler {
	return &GPUHealthReconciler{
		Client:       k8sClient,
		Scheme:       k8sClient.Scheme(),
		TelemetryURL: serverURL,
	}
}

func doReconcile(nn types.NamespacedName, serverURL string) error {
	_, err := newReconciler(serverURL).Reconcile(ctx, ctrlreconcile.Request{NamespacedName: nn})
	return err
}

func createGPUHealth(name string, spec v1alpha1.GPUHealthSpec) types.NamespacedName {
	nn := types.NamespacedName{Name: name}
	Expect(k8sClient.Create(ctx, &v1alpha1.GPUHealth{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       spec,
	})).To(Succeed())
	DeferCleanup(func() {
		gh := &v1alpha1.GPUHealth{}
		if err := k8sClient.Get(ctx, nn, gh); err == nil {
			Expect(k8sClient.Delete(ctx, gh)).To(Succeed())
		}
	})
	return nn
}

func createNode(name string) {
	Expect(k8sClient.Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	})).To(Succeed())
	DeferCleanup(func() {
		node := &corev1.Node{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, node); err == nil {
			Expect(k8sClient.Delete(ctx, node)).To(Succeed())
		}
	})
}

func getGPUHealth(nn types.NamespacedName) *v1alpha1.GPUHealth {
	gh := &v1alpha1.GPUHealth{}
	Expect(k8sClient.Get(ctx, nn, gh)).To(Succeed())
	return gh
}

func baseSpec(policy v1alpha1.RemediationPolicy) v1alpha1.GPUHealthSpec {
	return v1alpha1.GPUHealthSpec{
		NodeName:               testNodeName,
		GPUID:                  testGPUID,
		RemediationPolicy:      policy,
		MaxRemediationAttempts: 3,
	}
}

var _ = Describe("GPUHealth Controller", func() {

	Context("when telemetry reports healthy", func() {
		It("sets phase to Healthy with GPUHealthy condition True", func() {
			nn := createGPUHealth("gpu-healthy", baseSpec(v1alpha1.RemediationPolicyNone))
			server := newTelemetryServer(gpuModel.StatusHealthy)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseHealthy))

			cond := apimeta.FindStatusCondition(gh.Status.Conditions, v1alpha1.ConditionGPUHealthy)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(reasonGPUOperational))
		})
	})

	Context("when telemetry reports warning", func() {
		It("sets phase to Warning with GPUHealthy condition False", func() {
			nn := createGPUHealth("gpu-warning", baseSpec(v1alpha1.RemediationPolicyNone))
			server := newTelemetryServer(gpuModel.StatusWarning)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseWarning))

			cond := apimeta.FindStatusCondition(gh.Status.Conditions, v1alpha1.ConditionGPUHealthy)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(reasonGPUDegraded))
		})
	})

	Context("when telemetry reports critical with RemediationPolicyNone", func() {
		It("sets phase to Critical and increments remediation attempts", func() {
			nn := createGPUHealth("gpu-critical-none", baseSpec(v1alpha1.RemediationPolicyNone))
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseCritical))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(1)))
		})
	})

	Context("when telemetry reports critical with RemediationPolicyEscalate", func() {
		It("sets phase to Critical with EscalationRequired condition", func() {
			nn := createGPUHealth("gpu-critical-escalate", baseSpec(v1alpha1.RemediationPolicyEscalate))
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseCritical))

			cond := apimeta.FindStatusCondition(gh.Status.Conditions, v1alpha1.ConditionEscalationRequired)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("when telemetry reports critical with RemediationPolicyDrain", func() {
		It("sets phase to Draining and cordons the node", func() {
			createNode(testNodeName)
			nn := createGPUHealth("gpu-critical-drain", baseSpec(v1alpha1.RemediationPolicyDrain))
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseDraining))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(1)))

			node := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testNodeName}, node)).To(Succeed())
			Expect(node.Spec.Unschedulable).To(BeTrue())
		})
	})

	Context("when telemetry reports critical with RemediationPolicyReplace", func() {
		It("sets phase to Replacing and records findings", func() {
			createNode(testNodeName)
			nn := createGPUHealth("gpu-critical-replace", baseSpec(v1alpha1.RemediationPolicyReplace))
			server := newTelemetryServerWithHealth(gpuModel.GPUHealth{
				GPUID:        testGPUID,
				HealthStatus: gpuModel.StatusCritical,
				Memory:       gpuModel.Memory{ECCDoubleBitErrors: 2},
			})
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseReplacing))
			Expect(gh.Status.Findings).NotTo(BeEmpty())
			Expect(gh.Status.Findings[0].Type).To(Equal(v1alpha1.FindingECCDoubleBitError))
		})
	})

	Context("when max remediation attempts is reached", func() {
		It("transitions to Failed with EscalationRequired condition", func() {
			nn := createGPUHealth("gpu-failed", v1alpha1.GPUHealthSpec{
				NodeName:               testNodeName,
				GPUID:                  testGPUID,
				RemediationPolicy:      v1alpha1.RemediationPolicyNone,
				MaxRemediationAttempts: 1,
			})
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseFailed))

			cond := apimeta.FindStatusCondition(gh.Status.Conditions, v1alpha1.ConditionEscalationRequired)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(reasonGPUFailed))
		})
	})
})
