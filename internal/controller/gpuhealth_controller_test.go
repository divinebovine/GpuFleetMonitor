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
	"time"

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

func createNode() {
	Expect(k8sClient.Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: testNodeName},
	})).To(Succeed())
	DeferCleanup(func() {
		node := &corev1.Node{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: testNodeName}, node); err == nil {
			Expect(k8sClient.Delete(ctx, node)).To(Succeed())
		}
	})
}

func getGPUHealth(nn types.NamespacedName) *v1alpha1.GPUHealth {
	gh := &v1alpha1.GPUHealth{}
	Expect(k8sClient.Get(ctx, nn, gh)).To(Succeed())
	return gh
}

func getNode(name string) *corev1.Node {
	node := &corev1.Node{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, node)).To(Succeed())
	return node
}

// setNodeReady updates the node's Ready condition. A freshly created node has no
// conditions, which the controller treats as not ready.
func setNodeReady(name string, ready bool) {
	node := getNode(name)
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	node.Status.Conditions = []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: status},
	}
	Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
}

func baseSpec(policy v1alpha1.RemediationPolicy) v1alpha1.GPUHealthSpec {
	return v1alpha1.GPUHealthSpec{
		NodeName:               testNodeName,
		GPUID:                  testGPUID,
		RemediationPolicy:      policy,
		MaxRemediationAttempts: 3,
	}
}

// replaceSpec returns a spec for Replace-policy tests with explicit timeout values
// so tests don't depend on kubebuilder defaults (which aren't applied in envtest).
func replaceSpec() v1alpha1.GPUHealthSpec {
	return v1alpha1.GPUHealthSpec{
		NodeName:                  testNodeName,
		GPUID:                     testGPUID,
		RemediationPolicy:         v1alpha1.RemediationPolicyReplace,
		MaxRemediationAttempts:    3,
		ReplacementTimeoutSeconds: 1800,
		RejoiningTimeoutSeconds:   300,
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
		It("sets phase to Critical without incrementing remediation attempts", func() {
			nn := createGPUHealth("gpu-critical-none", baseSpec(v1alpha1.RemediationPolicyNone))
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseCritical))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(0)))
		})
	})

	Context("when telemetry reports critical with RemediationPolicyEscalate", func() {
		It("sets phase to Critical with EscalationRequired condition without cordoning the node", func() {
			createNode()
			nn := createGPUHealth("gpu-critical-escalate", baseSpec(v1alpha1.RemediationPolicyEscalate))
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseCritical))

			cond := apimeta.FindStatusCondition(gh.Status.Conditions, v1alpha1.ConditionEscalationRequired)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			Expect(getNode(testNodeName).Spec.Unschedulable).To(BeFalse())
		})
	})

	Context("when telemetry reports critical with RemediationPolicyDrain", func() {
		It("sets phase to Draining and cordons the node", func() {
			createNode()
			nn := createGPUHealth("gpu-critical-drain", baseSpec(v1alpha1.RemediationPolicyDrain))
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseDraining))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(1)))

			Expect(getNode(testNodeName).Spec.Unschedulable).To(BeTrue())
		})
	})

	Context("when telemetry reports critical with RemediationPolicyReplace", func() {
		It("sets phase to Replacing and records findings", func() {
			createNode()
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

	Context("when max remediation attempts is reached with RemediationPolicyDrain", func() {
		It("transitions to Failed with EscalationRequired condition", func() {
			createNode()
			nn := createGPUHealth("gpu-max-attempts", v1alpha1.GPUHealthSpec{
				NodeName:               testNodeName,
				GPUID:                  testGPUID,
				RemediationPolicy:      v1alpha1.RemediationPolicyDrain,
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

	Context("drain flow", func() {
		It("transitions from Draining to Recovering when no pods remain on the node", func() {
			createNode()
			nn := createGPUHealth("gpu-draining-to-recovering", baseSpec(v1alpha1.RemediationPolicyDrain))
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // Critical → Draining
			Expect(getGPUHealth(nn).Status.Phase).To(Equal(v1alpha1.PhaseDraining))

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // Draining → Recovering
			Expect(getGPUHealth(nn).Status.Phase).To(Equal(v1alpha1.PhaseRecovering))
		})

		It("transitions from Recovering to Healthy and uncordons the node when telemetry recovers", func() {
			createNode()
			nn := createGPUHealth("gpu-recovering-to-healthy", baseSpec(v1alpha1.RemediationPolicyDrain))
			critical := newTelemetryServer(gpuModel.StatusCritical)
			defer critical.Close()
			healthy := newTelemetryServer(gpuModel.StatusHealthy)
			defer healthy.Close()

			Expect(doReconcile(nn, critical.URL)).To(Succeed()) // → Draining
			Expect(doReconcile(nn, critical.URL)).To(Succeed()) // → Recovering
			Expect(doReconcile(nn, healthy.URL)).To(Succeed())  // → Healthy

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseHealthy))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(0)))
			Expect(getNode(testNodeName).Spec.Unschedulable).To(BeFalse())
		})

		It("re-drains with an incremented attempt count when telemetry is still Critical during recovery", func() {
			createNode()
			nn := createGPUHealth("gpu-recovering-critical", baseSpec(v1alpha1.RemediationPolicyDrain))
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → Draining (attempts=1)
			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → Recovering
			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → Draining (attempts=2)

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseDraining))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(2)))
		})
	})

	Context("failed phase", func() {
		It("returns to Healthy and resets attempts when the spec changes", func() {
			createNode()
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			nn := createGPUHealth("gpu-failed-spec-change", v1alpha1.GPUHealthSpec{
				NodeName:               testNodeName,
				GPUID:                  testGPUID,
				RemediationPolicy:      v1alpha1.RemediationPolicyDrain,
				MaxRemediationAttempts: 1,
			})

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → Failed
			Expect(getGPUHealth(nn).Status.Phase).To(Equal(v1alpha1.PhaseFailed))

			// Bump Generation by updating spec
			gh := getGPUHealth(nn)
			gh.Spec.MaxRemediationAttempts = 5
			Expect(k8sClient.Update(ctx, gh)).To(Succeed())

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // spec change → Healthy

			gh = getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseHealthy))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(0)))
		})

		It("returns to Healthy and resets attempts when telemetry recovers organically", func() {
			createNode()
			critical := newTelemetryServer(gpuModel.StatusCritical)
			defer critical.Close()
			healthy := newTelemetryServer(gpuModel.StatusHealthy)
			defer healthy.Close()

			nn := createGPUHealth("gpu-failed-recovers", v1alpha1.GPUHealthSpec{
				NodeName:               testNodeName,
				GPUID:                  testGPUID,
				RemediationPolicy:      v1alpha1.RemediationPolicyDrain,
				MaxRemediationAttempts: 1,
			})

			Expect(doReconcile(nn, critical.URL)).To(Succeed()) // → Failed
			Expect(doReconcile(nn, healthy.URL)).To(Succeed())  // telemetry healthy → Healthy

			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseHealthy))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(0)))
		})

		It("stays in Failed and requeues slowly when still critical", func() {
			createNode()
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			nn := createGPUHealth("gpu-failed-stays", v1alpha1.GPUHealthSpec{
				NodeName:               testNodeName,
				GPUID:                  testGPUID,
				RemediationPolicy:      v1alpha1.RemediationPolicyDrain,
				MaxRemediationAttempts: 1,
			})

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → Failed

			result, err := newReconciler(server.URL).Reconcile(ctx, ctrlreconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(120 * time.Second))
			Expect(getGPUHealth(nn).Status.Phase).To(Equal(v1alpha1.PhaseFailed))
		})
	})

	Context("replace flow", func() {
		It("sets ReplacementStartedAt when the node goes NotReady", func() {
			createNode()
			// A freshly created node has no Ready condition — the controller treats it as not ready.
			nn := createGPUHealth("gpu-replace-notready", replaceSpec())
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // Critical → Replacing (attempts=1)
			Expect(getGPUHealth(nn).Status.Phase).To(Equal(v1alpha1.PhaseReplacing))

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // node not ready → ReplacementStartedAt set
			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseReplacing))
			Expect(gh.Status.ReplacementStartedAt).NotTo(BeNil())
		})

		It("clears ReplacementStartedAt and starts a new attempt when replacement times out", func() {
			createNode()
			nn := createGPUHealth("gpu-replace-timeout", replaceSpec())
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			// Drive to Replacing with ReplacementStartedAt set via natural reconcile chain.
			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → Replacing
			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → sets ReplacementStartedAt
			Expect(getGPUHealth(nn).Status.ReplacementStartedAt).NotTo(BeNil())

			// Simulate timeout by moving ReplacementStartedAt into the past.
			gh := getGPUHealth(nn)
			past := metav1.NewTime(time.Now().Add(-time.Hour))
			gh.Status.ReplacementStartedAt = &past
			Expect(k8sClient.Status().Update(ctx, gh)).To(Succeed())

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // timeout → retry
			gh = getGPUHealth(nn)
			Expect(gh.Status.ReplacementStartedAt).To(BeNil())
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(2)))
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseReplacing))
		})

		It("transitions to Rejoining when the node returns Ready after replacement", func() {
			createNode()
			nn := createGPUHealth("gpu-replace-rejoining", replaceSpec())
			server := newTelemetryServer(gpuModel.StatusCritical)
			defer server.Close()

			// Drive to Replacing with ReplacementStartedAt set.
			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → Replacing
			Expect(doReconcile(nn, server.URL)).To(Succeed()) // → sets ReplacementStartedAt
			Expect(getGPUHealth(nn).Status.ReplacementStartedAt).NotTo(BeNil())

			// Node comes back Ready (hardware replacement complete).
			setNodeReady(testNodeName, true)

			Expect(doReconcile(nn, server.URL)).To(Succeed()) // node ready → Rejoining
			gh := getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseRejoining))
			Expect(gh.Status.ReplacementStartedAt).To(BeNil())
		})
	})

	Context("rejoining phase", func() {
		It("transitions to Healthy and uncordons the node when telemetry recovers", func() {
			createNode()
			nn := createGPUHealth("gpu-rejoining-healthy", replaceSpec())
			healthy := newTelemetryServer(gpuModel.StatusHealthy)
			defer healthy.Close()

			// Set up Rejoining state directly.
			now := metav1.Now()
			gh := getGPUHealth(nn)
			gh.Status.Phase = v1alpha1.PhaseRejoining
			gh.Status.RemediationAttempts = 1
			gh.Status.LastTransitionTime = &now
			gh.Status.ObservedGeneration = gh.Generation
			Expect(k8sClient.Status().Update(ctx, gh)).To(Succeed())

			// Cordon the node to simulate the state left by the Replace flow.
			node := getNode(testNodeName)
			node.Spec.Unschedulable = true
			Expect(k8sClient.Update(ctx, node)).To(Succeed())

			Expect(doReconcile(nn, healthy.URL)).To(Succeed())

			gh = getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseHealthy))
			Expect(gh.Status.RemediationAttempts).To(Equal(int32(0)))
			Expect(getNode(testNodeName).Spec.Unschedulable).To(BeFalse())
		})

		It("stays in Rejoining and requeues quickly when Warning telemetry is within the timeout", func() {
			nn := createGPUHealth("gpu-rejoining-warning-ok", replaceSpec())
			server := newTelemetryServer(gpuModel.StatusWarning)
			defer server.Close()

			now := metav1.Now()
			gh := getGPUHealth(nn)
			gh.Status.Phase = v1alpha1.PhaseRejoining
			gh.Status.RemediationAttempts = 1
			gh.Status.LastTransitionTime = &now
			gh.Status.ObservedGeneration = gh.Generation
			Expect(k8sClient.Status().Update(ctx, gh)).To(Succeed())

			result, err := newReconciler(server.URL).Reconcile(ctx, ctrlreconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
			Expect(getGPUHealth(nn).Status.Phase).To(Equal(v1alpha1.PhaseRejoining))
		})

		It("transitions to Failed with EscalationRequired when the rejoining timeout elapses", func() {
			nn := createGPUHealth("gpu-rejoining-timeout", replaceSpec())
			server := newTelemetryServer(gpuModel.StatusWarning)
			defer server.Close()

			// Set LastTransitionTime far in the past so the deadline is already exceeded.
			past := metav1.NewTime(time.Now().Add(-time.Hour))
			gh := getGPUHealth(nn)
			gh.Status.Phase = v1alpha1.PhaseRejoining
			gh.Status.RemediationAttempts = 1
			gh.Status.LastTransitionTime = &past
			gh.Status.ObservedGeneration = gh.Generation
			Expect(k8sClient.Status().Update(ctx, gh)).To(Succeed())

			Expect(doReconcile(nn, server.URL)).To(Succeed())

			gh = getGPUHealth(nn)
			Expect(gh.Status.Phase).To(Equal(v1alpha1.PhaseFailed))

			cond := apimeta.FindStatusCondition(gh.Status.Conditions, v1alpha1.ConditionEscalationRequired)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(reasonGPUFailed))
		})
	})
})
