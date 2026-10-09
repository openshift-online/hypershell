// Package harness holds the infrastructure-agnostic helpers the e2e suite shares
// across all drivers: the Kubernetes clients built once from the current
// KUBECONFIG context, the CommandRunner that logs every operation as a demo
// command before running it and poll/retry primitives.
package harness

import (
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// Clients bundles the Kubernetes access the suite and drivers share. It is built
// once from the current KUBECONFIG context (SetupSuite) and threaded through the
// driver and every phase step; nothing in the suite shells out to kubectl or oc.
type Clients struct {
	// Config is the REST config resolved from the current KUBECONFIG context.
	Config *rest.Config
	// Kube serves core/apps/batch/networking resources (Deployments, Services,
	// Secrets, Jobs, NetworkPolicies, Namespaces, Events, pod exec).
	Kube kubernetes.Interface
	// Gateway serves the Gateway API resources (Gateway, GRPCRoute, HTTPRoute)
	// with the typed clientset, matching the control-plane's idiom.
	Gateway gatewayclient.Interface
	// contextNamespace is the namespace of the resolved KUBECONFIG context (the
	// OpenShift driver uses it as the default platform namespace / oc project).
	contextNamespace string
}

// ContextNamespace returns the namespace from the resolved KUBECONFIG context, or
// "" when the context sets none.
func (c *Clients) ContextNamespace() string { return c.contextNamespace }

// NewClients builds the shared clients from the current KUBECONFIG context (the
// same context kubectl would use), honoring KUBECONFIG and the current-context
// selection. It returns an error rather than panicking so SetupSuite reports a
// missing or unreachable cluster as a failed assertion with context.
func NewClients() (*Clients, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	// E2E_KUBECONTEXT pins the suite to a specific context regardless of the
	// kubeconfig's current-context, so a run targets the intended cluster even when
	// another session has switched the active context.
	if ctxName := os.Getenv("E2E_KUBECONTEXT"); ctxName != "" {
		overrides.CurrentContext = ctxName
	}
	deferred := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	cfg, err := deferred.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("resolve KUBECONFIG context: %w", err)
	}
	ctxNamespace, _, _ := deferred.Namespace()

	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build core Kubernetes client: %w", err)
	}

	gw, err := gatewayclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build Gateway API client: %w", err)
	}

	return &Clients{Config: cfg, Kube: kube, Gateway: gw, contextNamespace: ctxNamespace}, nil
}

// ServesAPIGroup reports whether the cluster the clients point at serves the
// named API group (for example "route.openshift.io"). The driver auto-detection
// uses it to distinguish an OpenShift cluster from a plain Kind cluster.
func (c *Clients) ServesAPIGroup(group string) (bool, error) {
	groups, err := c.Kube.Discovery().ServerGroups()
	if err != nil {
		return false, fmt.Errorf("list server API groups: %w", err)
	}
	for _, g := range groups.Groups {
		if g.Name == group {
			return true, nil
		}
	}
	return false, nil
}
