package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	k8srest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	crClient "sigs.k8s.io/controller-runtime/pkg/client"
	schemeyaml "sigs.k8s.io/yaml"

	"github.com/divinebovine/GpuFleetMonitor/api/v1alpha1"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/workflows"
	temporalclient "go.temporal.io/sdk/client"
)

type gpuDef struct {
	name   string
	node   string
	gpuID  string
	policy v1alpha1.RemediationPolicy
}

// demoGPUs defines the 12 GPUHealth CRs created for the demo:
//   - gpu-node-1: Drain policy — operator cordons + evicts on Critical
//   - gpu-node-2: Escalate policy — operator sets EscalationRequired condition
//   - gpu-node-3: None policy — observes and records findings only
var demoGPUs = []gpuDef{
	{"gpu-drain-1", "gpu-node-1", "GPU-00001", v1alpha1.RemediationPolicyDrain},
	{"gpu-drain-2", "gpu-node-1", "GPU-00002", v1alpha1.RemediationPolicyDrain},
	{"gpu-drain-3", "gpu-node-1", "GPU-00003", v1alpha1.RemediationPolicyDrain},
	{"gpu-drain-4", "gpu-node-1", "GPU-00004", v1alpha1.RemediationPolicyDrain},
	{"gpu-escalate-1", "gpu-node-2", "GPU-00005", v1alpha1.RemediationPolicyEscalate},
	{"gpu-escalate-2", "gpu-node-2", "GPU-00006", v1alpha1.RemediationPolicyEscalate},
	{"gpu-escalate-3", "gpu-node-2", "GPU-00007", v1alpha1.RemediationPolicyEscalate},
	{"gpu-escalate-4", "gpu-node-2", "GPU-00008", v1alpha1.RemediationPolicyEscalate},
	{"gpu-observe-1", "gpu-node-3", "GPU-00009", v1alpha1.RemediationPolicyNone},
	{"gpu-observe-2", "gpu-node-3", "GPU-00010", v1alpha1.RemediationPolicyNone},
	{"gpu-observe-3", "gpu-node-3", "GPU-00011", v1alpha1.RemediationPolicyNone},
	{"gpu-observe-4", "gpu-node-3", "GPU-00012", v1alpha1.RemediationPolicyNone},
}

func main() {
	ctx := context.Background()

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = "/kubeconfig/config"
	}
	k3sServer := os.Getenv("K3S_SERVER")
	if k3sServer == "" {
		k3sServer = "k3s-server:6443"
	}
	temporalAddr := os.Getenv("TEMPORAL_ADDRESS")
	if temporalAddr == "" {
		temporalAddr = "temporal:7233"
	}

	log.Println("Waiting for k3s API server...")
	restCfg := waitForK3s(ctx, kubeconfigPath, k3sServer)

	log.Println("Installing CRD...")
	if err := installCRD(ctx, restCfg); err != nil {
		log.Fatalf("installing CRD: %v", err)
	}

	log.Println("Waiting for worker nodes...")
	k8sClient := waitForNodes(ctx, restCfg, 3)

	log.Println("Creating GPUHealth CRs...")
	if err := createCRs(ctx, k8sClient); err != nil {
		log.Fatalf("creating CRs: %v", err)
	}

	log.Println("Starting Temporal workflows...")
	if err := startWorkflows(ctx, temporalAddr); err != nil {
		// Non-fatal: operator demo works without Temporal
		log.Printf("warning: Temporal workflows not started: %v", err)
	}

	log.Println("Demo setup complete.")
}

// patchKubeconfig replaces the 127.0.0.1 server address that k3s writes
// with the Docker-internal hostname so other containers can reach the API server.
func patchKubeconfig(path, k3sServer string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.Replace(data, []byte("127.0.0.1:6443"), []byte(k3sServer), 1), nil
}

func waitForK3s(ctx context.Context, kubeconfigPath, k3sServer string) *k8srest.Config {
	for {
		data, err := patchKubeconfig(kubeconfigPath, k3sServer)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		cfg, err := clientcmd.RESTConfigFromKubeConfig(data)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		dc, err := discovery.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if _, err := dc.ServerVersion(); err != nil {
			log.Printf("API server not ready yet: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		log.Println("k3s API server ready.")
		return cfg
	}
}

func installCRD(ctx context.Context, cfg *k8srest.Config) error {
	crdYAML, err := os.ReadFile("/etc/demo/crd.yaml")
	if err != nil {
		return fmt.Errorf("reading CRD YAML: %w", err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := schemeyaml.Unmarshal(crdYAML, &crd); err != nil {
		return fmt.Errorf("parsing CRD YAML: %w", err)
	}
	extClient, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		return err
	}
	_, err = extClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, &crd, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func buildK8sClient(cfg *k8srest.Config) (crClient.Client, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return crClient.New(cfg, crClient.Options{Scheme: scheme})
}

func waitForNodes(ctx context.Context, cfg *k8srest.Config, required int) crClient.Client {
	k8sClient, err := buildK8sClient(cfg)
	if err != nil {
		log.Fatalf("building k8s client: %v", err)
	}
	for {
		var nodeList corev1.NodeList
		if err := k8sClient.List(ctx, &nodeList); err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		ready := 0
		for _, n := range nodeList.Items {
			if _, isCP := n.Labels["node-role.kubernetes.io/control-plane"]; isCP {
				continue
			}
			for _, c := range n.Status.Conditions {
				if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
					ready++
				}
			}
		}
		if ready >= required {
			log.Printf("%d worker nodes ready.", ready)
			return k8sClient
		}
		log.Printf("waiting for %d worker nodes (%d ready)...", required, ready)
		time.Sleep(3 * time.Second)
	}
}

func createCRs(ctx context.Context, k8sClient crClient.Client) error {
	for _, def := range demoGPUs {
		cr := &v1alpha1.GPUHealth{
			ObjectMeta: metav1.ObjectMeta{Name: def.name},
			Spec: v1alpha1.GPUHealthSpec{
				NodeName:               def.node,
				GPUID:                  def.gpuID,
				RemediationPolicy:      def.policy,
				MaxRemediationAttempts: 3,
			},
		}
		if err := k8sClient.Create(ctx, cr); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return fmt.Errorf("creating %s: %w", def.name, err)
		}
		log.Printf("created %s (%s → %s)", def.name, def.gpuID, def.policy)
	}
	return nil
}

func startWorkflows(ctx context.Context, temporalAddr string) error {
	tc, err := temporalclient.Dial(temporalclient.Options{HostPort: temporalAddr})
	if err != nil {
		return fmt.Errorf("dialing Temporal: %w", err)
	}
	defer tc.Close()
	for _, def := range demoGPUs {
		_, err := tc.ExecuteWorkflow(ctx, temporalclient.StartWorkflowOptions{
			ID:        fmt.Sprintf("monitor-%s", def.gpuID),
			TaskQueue: "gpu-monitor",
		}, workflows.MonitorGPU, def.gpuID)
		if err != nil {
			log.Printf("warning: workflow for %s: %v", def.gpuID, err)
		} else {
			log.Printf("started MonitorGPU for %s", def.gpuID)
		}
	}
	return nil
}
