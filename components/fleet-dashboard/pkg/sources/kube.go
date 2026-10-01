package sources

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// KubeClients bundles the typed and dynamic clients the BFF needs.
type KubeClients struct {
	Clientset kubernetes.Interface
	Dynamic   dynamic.Interface
}

// NewKubeClients builds clients from in-cluster config, or from a kubeconfig
// path when configured (local dev).
func NewKubeClients(c *config.Config) (*KubeClients, error) {
	cfg, err := restConfig(c.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	return &KubeClients{Clientset: cs, Dynamic: dyn}, nil
}

func restConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}
	return rest.InClusterConfig()
}
