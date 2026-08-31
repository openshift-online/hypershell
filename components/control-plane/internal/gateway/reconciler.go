package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
)

// networkPoliciesDisabledLogOnce keeps the "network policies disabled" notice to
// a single line per process. When GATEWAY_SKIP_NETWORK_POLICIES=true (the
// default in the kind overlay) the skip branches run on every reconcile, so
// logging per-resource produced steady per-reconcile noise under a misleading
// DEBUG label. logNetworkPoliciesDisabled emits the notice once instead.
var networkPoliciesDisabledLogOnce sync.Once

func logNetworkPoliciesDisabled() {
	networkPoliciesDisabledLogOnce.Do(func() {
		log.Printf("network policies disabled (GATEWAY_SKIP_NETWORK_POLICIES=true); skipping gateway NetworkPolicy resources")
	})
}

func ReconcileGateway(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	nsConfig NamespaceConfig,
	manifests map[string][]*unstructured.Unstructured,
	opts ReconcileOpts,
) error {
	images := opts.Images
	if images == nil {
		images = StaticImageDefaults{}
	}
	ingressMode := gatewayIngressMode(opts)

	if !namespaceExists(ctx, clientset, nsConfig.Name) {
		if err := createNamespace(ctx, clientset, nsConfig.Name); err != nil {
			return fmt.Errorf("create namespace %s: %w", nsConfig.Name, err)
		}
	}

	if err := ValidateGatewayConfig(nsConfig.Gateway); err != nil {
		return fmt.Errorf("invalid gateway configuration: %w", err)
	}

	// When an ingress mode is active the gateway is reachable at an external
	// hostname (gw-<namespace>.<base-domain>, or an explicit Route.Host). The
	// gateway pod terminates TLS with its own per-tenant CA, and both ingress
	// modes carry that TLS through unmodified (Route passthrough / Gateway API
	// BackendTLSPolicy), so the server certificate must list the external
	// hostname as a SAN or clients fail verification. The controller derives
	// that hostname, so it -- not the operator -- injects it into the cert SANs
	// here, before cert-manager mints the certificate below.
	if ingressMode != IngressModeNone && nsConfig.Gateway.Route.Enabled {
		hostname, err := deriveGatewayHostname(nsConfig)
		if err != nil {
			log.Printf("WARN cannot add ingress hostname to gateway certificate SANs in %s: %v", nsConfig.Name, err)
		} else {
			nsConfig.Gateway.ServerDnsNames = appendDNSNameIfMissing(nsConfig.Gateway.ServerDnsNames, hostname)
		}
	}

	switch opts.DatabaseProvider {
	case "":
		// Legacy gateways may not have a database_id. Preserve any existing
		// database resources and continue reconciling the rest of the gateway.
	case "cnpg":
		if opts.CNPG.ClusterNamespace == "" {
			return fmt.Errorf("CNPG cluster namespace is required for gateway database reconciliation in namespace %s", nsConfig.Name)
		}
		if !opts.HasCNPG {
			return fmt.Errorf("CNPG operator is required but not available on the cluster: gateway deployment blocked for namespace %s", nsConfig.Name)
		}

		if err := reconcileCNPGDatabaseResources(ctx, dynamicClient, clientset, nsConfig.Name, opts.GatewayID, opts.CNPG); err != nil {
			return fmt.Errorf("reconcile CNPG database resources in %s: %w", nsConfig.Name, err)
		}

		if opts.RotateDBCredentials != "" {
			if err := rotateCNPGDatabaseCredentials(ctx, clientset, nsConfig.Name, opts.GatewayID, opts.CNPG, opts.RotateDBCredentials); err != nil {
				return fmt.Errorf("rotate database credentials in %s: %w", nsConfig.Name, err)
			}
		}
	case "deployment":
		if opts.DeploymentDBNamespace == "" {
			return fmt.Errorf("deployment database namespace is required for gateway database reconciliation in namespace %s", nsConfig.Name)
		}
		if err := reconcileDeploymentDatabaseCredentials(ctx, clientset, opts.DeploymentDBNamespace, nsConfig.Name); err != nil {
			return fmt.Errorf("copy deployment database credentials to %s: %w", nsConfig.Name, err)
		}
	default:
		return fmt.Errorf("unsupported database provider %q for gateway in namespace %s", opts.DatabaseProvider, nsConfig.Name)
	}

	if nsConfig.Gateway.CredentialDriver == nil {
		if err := reconcileCredentialKEK(ctx, clientset, nsConfig.Name); err != nil {
			return fmt.Errorf("reconcile credential KEK in %s: %w", nsConfig.Name, err)
		}
		deleteCredentialSecretsRBAC(ctx, dynamicClient, nsConfig.Name)
	} else {
		if err := reconcileCredentialDriverResources(ctx, dynamicClient, clientset, nsConfig); err != nil {
			return fmt.Errorf("reconcile credential driver resources in %s: %w", nsConfig.Name, err)
		}
	}

	if opts.HasCertManager {
		if err := reconcileCertManagerResources(ctx, dynamicClient, nsConfig); err != nil {
			return fmt.Errorf("reconcile cert-manager resources in %s: %w", nsConfig.Name, err)
		}
	} else {
		return fmt.Errorf("cert-manager is required but not available on the cluster: gateway deployment blocked for namespace %s", nsConfig.Name)
	}

	if opts.Keycloak != nil {
		if err := reconcileKeycloakClient(ctx, opts, &nsConfig); err != nil {
			return fmt.Errorf("reconcile keycloak client in %s: %w", nsConfig.Name, err)
		}
	}

	hasTrustedCA := reconcileTrustedCABundle(ctx, clientset, opts.ControlPlaneNamespace, nsConfig.Name)

	if err := deployGateway(ctx, dynamicClient, clientset, nsConfig, manifests, images, opts, hasTrustedCA); err != nil {
		return fmt.Errorf("deploy gateway in %s: %w", nsConfig.Name, err)
	}

	if opts.IsOpenShift {
		if err := reconcileOpenShiftSCC(ctx, dynamicClient, nsConfig.Name); err != nil {
			log.Printf("WARN failed to reconcile OpenShift SCC binding in %s: %v", nsConfig.Name, err)
		}
	}

	// Tenant ingress is environment-adaptive: Gateway API where available,
	// OpenShift Routes where it is not. See gatewayIngressMode.
	switch ingressMode {
	case IngressModeGatewayAPI:
		if nsConfig.Gateway.Route.Enabled {
			// Propagate this error rather than logging and swallowing it: the only
			// hard failures reconcileGatewayAPIResources returns are a TLS-secret
			// wait timeout and a fail-closed route-intent re-check (its best-effort
			// console/NetworkPolicy/CA steps log internally and never return). Both
			// leave a routed gateway without a usable route, so Handle must see the
			// error and mark the gateway Failed -- a Failed gateway is not phase-
			// gated, so the next watch event re-provisions and rebuilds the route.
			// Swallowing it here would strand a partial route the phase gate then
			// blocks any later event from repairing.
			if err := reconcileGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig, opts); err != nil {
				return fmt.Errorf("reconcile Gateway API resources in %s: %w", nsConfig.Name, err)
			}
		} else {
			if err := DeleteGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig.Name, opts); err != nil {
				log.Printf("WARN failed to remove Gateway API resources in %s: %v", nsConfig.Name, err)
			}
		}
	case IngressModeRoute:
		if nsConfig.Gateway.Route.Enabled {
			if err := reconcileRouteResources(ctx, dynamicClient, clientset, nsConfig, opts); err != nil {
				log.Printf("WARN failed to reconcile Route resources in %s: %v", nsConfig.Name, err)
			}
			// The console uses the same selected ingress mode as the gateway. A
			// console error must not fail gateway provisioning. The health loop
			// retries the console until it can serve.
			if err := ReconcileConsole(ctx, dynamicClient, clientset, nsConfig, opts); err != nil {
				log.Printf("WARN failed to reconcile console in %s: %v", nsConfig.Name, err)
			}
		} else {
			if err := DeleteRouteResources(ctx, dynamicClient, clientset, nsConfig.Name, opts); err != nil {
				log.Printf("WARN failed to remove Route resources in %s: %v", nsConfig.Name, err)
			}
		}
	default:
		log.Printf("INFO no ingress mode selected for %s (not OpenShift and no Gateway API); skipping tenant ingress", nsConfig.Name)
	}

	log.Printf("INFO gateway reconciled in namespace %s", nsConfig.Name)
	return nil
}

// DeleteGatewayResources cleans up the resources a gateway owns that live
// OUTSIDE its namespace and are therefore not reclaimed when the namespace is
// deleted. Everything inside the gateway's namespace (Deployments, Services,
// Secrets, ConfigMaps, PVCs, Jobs, Roles, RoleBindings, and cert-manager /
// Gateway API objects) is garbage-collected by Kubernetes as a side effect of
// deleting the namespace itself, so those are not enumerated here. The
// out-of-namespace resources handled below are:
//   - the cluster-scoped ClusterRoleBinding created for the gateway,
//   - the gateway's external Keycloak client, and
//   - any credential RBAC the gateway created in a separate credential namespace.
func DeleteGatewayResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	namespace string,
	opts ReconcileOpts,
	credentialNamespaces ...string,
) error {
	crbGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "clusterrolebindings",
	}
	crbName := fmt.Sprintf("openshell-gateway-node-reader-%s", namespace)
	if err := dynamicClient.Resource(crbGVR).Delete(ctx, crbName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete ClusterRoleBinding %s: %v", crbName, err)
		}
	} else {
		log.Printf("INFO deleted ClusterRoleBinding %s", crbName)
	}

	if opts.KeycloakClient != nil && opts.GatewayName != "" && opts.GatewayID != "" {
		kcClientID := fmt.Sprintf("%s-%s", opts.GatewayName, opts.GatewayID)
		if err := opts.KeycloakClient.DeleteGatewayServiceAccountClients(ctx, opts.GatewayID); err != nil {
			// Do not delete the parent clients while an OpenShell gateway service
			// account may still be enabled. Returning an error makes teardown retry.
			return fmt.Errorf("delete gateway service-account clients: %w", err)
		}
		log.Printf("INFO deleted keycloak service-account clients for gateway %s", opts.GatewayID)

		// The console namespaced resources are swept by label above, but the
		// console Keycloak client must be deleted explicitly (it lives in the
		// realm, not the namespace). Best-effort: log the orphan on failure.
		consoleClientID := kcClientID + "-console"
		if err := opts.KeycloakClient.DeleteConsoleClient(ctx, consoleClientID); err != nil {
			log.Printf("WARN failed to delete console client %s (orphaned): %v", consoleClientID, err)
		} else {
			log.Printf("INFO deleted console client %s", consoleClientID)
		}

		if err := opts.KeycloakClient.DeleteGatewayClient(ctx, kcClientID); err != nil {
			log.Printf("WARN failed to delete keycloak client %s (orphaned): %v", kcClientID, err)
		} else {
			log.Printf("INFO deleted keycloak client %s", kcClientID)
		}
	}

	if opts.HasCNPG && opts.GatewayID != "" {
		if opts.CNPG.ClusterNamespace == "" {
			log.Printf("WARN gateway %s: CNPG cluster namespace unknown; Database, DatabaseRole, and password Secret were not deleted and may require manual cleanup", opts.GatewayID)
		} else {
			deleteCNPGResources(ctx, dynamicClient, clientset, opts.GatewayID, opts.CNPG)
		}
	}

	for _, credNS := range credentialNamespaces {
		if credNS != "" && credNS != namespace {
			deleteCredentialSecretsRBAC(ctx, dynamicClient, credNS)
			log.Printf("INFO cleaned up credential RBAC from namespace %s", credNS)
		}
	}

	log.Printf("INFO gateway out-of-namespace resources cleaned up for namespace %s", namespace)
	return nil
}

// DeleteLabeledNamespaceResources reclaims this gateway's own in-namespace
// resources when the namespace itself survives gateway deletion. Kubernetes only
// garbage-collects namespaced objects as a side effect of deleting the namespace,
// so on the delete path we normally rely on DeleteManagedNamespace to cascade
// them. This is the fallback for when the namespace is NOT reaped: most
// importantly a pre-existing namespace the control plane does not manage (it is
// missing the management labels, so DeleteManagedNamespace leaves it and the
// NamespaceGCReconciler ignores it too). Without this sweep the workloads the
// gateway created inside such a namespace would be orphaned.
//
// Only resources carrying hypershell.redhat.io/managed=true (the label the
// gateway stamps on everything it creates) are deleted, so co-tenant workloads
// sharing the namespace are never touched: the same no-collateral guarantee that
// keeps GC from reaping a shared namespace. The namespace itself is never
// deleted here. It is best-effort - per-resource failures are logged and do not
// abort the sweep, matching DeleteGatewayResources - and the caller invokes it
// only when the namespace was left in place.
func DeleteLabeledNamespaceResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	namespace string,
	opts ReconcileOpts,
) {
	labelSelector := fmt.Sprintf("%s=%s", ManagedLabel, ManagedLabelValue)

	namespacedResources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Version: "v1", Resource: "services"},
		{Version: "v1", Resource: "configmaps"},
		{Version: "v1", Resource: "serviceaccounts"},
		{Version: "v1", Resource: "secrets"},
		{Version: "v1", Resource: "persistentvolumeclaims"},
		{Group: "batch", Version: "v1", Resource: "jobs"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
	}
	if opts.IsOpenShift {
		namespacedResources = append(namespacedResources,
			schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"},
		)
	}
	if opts.HasCertManager {
		namespacedResources = append(namespacedResources,
			schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"},
			schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
		)
	}
	if opts.HasGatewayAPI {
		namespacedResources = append(namespacedResources,
			schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"},
			schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"},
			schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "backendtlspolicies"},
		)
	}

	for _, gvr := range namespacedResources {
		list, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			// A missing CRD/resource type is expected on clusters without the
			// optional APIs; skip it rather than treating it as a failure.
			if k8serrors.IsNotFound(err) {
				continue
			}
			log.Printf("WARN failed to list %s in namespace %s for cleanup: %v", gvr.Resource, namespace, err)
			continue
		}
		for i := range list.Items {
			name := list.Items[i].GetName()
			if err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
				if !k8serrors.IsNotFound(err) {
					log.Printf("WARN failed to delete %s %s in namespace %s: %v", gvr.Resource, name, namespace, err)
				}
				continue
			}
			log.Printf("INFO deleted %s %s from namespace %s", gvr.Resource, name, namespace)
		}
	}
}

// DeleteGatewayAPIResources reconciles the desired *absence* of a gateway's
// route: it removes the GRPCRoute, BackendTLSPolicy, backend-CA ConfigMap and
// router NetworkPolicy, tears down the console (which follows the route), and
// clears the stored route_address. It attempts every deletion regardless of
// individual failures and returns their joined errors (nil once everything is
// absent), so a caller -- the provisioning path's route-disabled branch and the
// health loop, which owns a Running gateway the provisioning path never revisits
// -- can retry until the route and its console are fully gone rather than
// stopping on partial cleanup. Idempotent: absent resources are ignored.
func DeleteGatewayAPIResources(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, namespace string, opts ReconcileOpts) error {
	var errs []error

	grpcRouteGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "grpcroutes",
	}
	if err := dynamicClient.Resource(grpcRouteGVR).Namespace(namespace).Delete(ctx, "openshell-gateway", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete GRPCRoute in %s: %w", namespace, err))
	}

	btlsGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "backendtlspolicies",
	}
	if err := dynamicClient.Resource(btlsGVR).Namespace(namespace).Delete(ctx, "openshell-gateway", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete BackendTLSPolicy in %s: %w", namespace, err))
	}

	if err := clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, "openshell-backend-ca", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete backend CA ConfigMap in %s: %w", namespace, err))
	}

	netpolGVR := schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}
	if err := dynamicClient.Resource(netpolGVR).Namespace(namespace).Delete(ctx, "openshell-gateway-allow-router", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete router NetworkPolicy in %s: %w", namespace, err))
	}

	// The console follows the route, so removing the route removes the console.
	if err := deleteConsole(ctx, dynamicClient, clientset, namespace, opts); err != nil {
		errs = append(errs, err)
	}

	if opts.UpdateRouteAddress != nil {
		if err := opts.UpdateRouteAddress(ctx, ""); err != nil {
			errs = append(errs, fmt.Errorf("clear routeAddress in %s: %w", namespace, err))
		} else {
			log.Printf("INFO cleared routeAddress for gateway in %s", namespace)
		}
	}

	if len(errs) == 0 {
		log.Printf("INFO Gateway API resources removed from namespace %s", namespace)
	}
	return errors.Join(errs...)
}

// ConsoleClientChecker reports whether the gateway's external Keycloak console
// client still exists. RouteResourcesAbsent uses it so teardown's settled-state
// check converges across BOTH Kubernetes and the external realm: the console
// client is a realm object, not a namespaced one, so a stale provisioning pass
// that recreated only the client -- e.g. failing before its namespaced Secret
// and Deployment writes -- would otherwise be invisible to a Kubernetes-only
// probe and let teardown settle while the client leaks.
type ConsoleClientChecker interface {
	ConsoleClientExists(ctx context.Context, consoleClientID string) (bool, error)
}

// RouteResourcesAbsent reports whether the resources for the selected gateway
// ingress mode and all console resources are absent. It checks both console
// exposure kinds because cleanup removes an inactive exposure after a mode
// change. It also checks the external Keycloak console client. Unknown state
// must never be read as absence.
//
// It returns (true, nil) only when every probed resource is confirmed absent.
// The first resource found present short-circuits to (false, nil). Any probe
// that fails for a reason other than Kubernetes NotFound is returned as an error
// so the caller treats absence as unconfirmed (and re-runs teardown) rather than
// trusting an unknown state. The Keycloak client probe is skipped when
// consoleClient is nil or consoleClientID is empty (no Keycloak configured).
func RouteResourcesAbsent(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, namespace, ingressMode string, consoleClient ConsoleClientChecker, consoleClientID string) (bool, error) {
	grpcRouteGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}
	btlsGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "backendtlspolicies"}
	httpRouteGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
	openShiftRouteGVR := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	netpolGVR := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}

	type dynamicProbe struct {
		gvr  schema.GroupVersionResource
		name string
	}
	var dynamicProbes []dynamicProbe
	switch ingressMode {
	case IngressModeGatewayAPI:
		dynamicProbes = append(dynamicProbes,
			dynamicProbe{grpcRouteGVR, "openshell-gateway"},
			dynamicProbe{btlsGVR, "openshell-gateway"},
			dynamicProbe{netpolGVR, "openshell-gateway-allow-router"},
		)
	case IngressModeRoute:
		dynamicProbes = append(dynamicProbes,
			dynamicProbe{openShiftRouteGVR, "openshell-gateway"},
			dynamicProbe{netpolGVR, "openshell-gateway-allow-router"},
		)
	case IngressModeNone:
		// There is no selected gateway exposure to probe.
	default:
		return false, fmt.Errorf("unsupported ingress mode %q for resource absence probe", ingressMode)
	}
	dynamicProbes = append(dynamicProbes,
		dynamicProbe{httpRouteGVR, consoleName},
		dynamicProbe{openShiftRouteGVR, consoleName},
		dynamicProbe{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, consoleName},
		dynamicProbe{netpolGVR, "openshell-console-allow-router"},
		dynamicProbe{netpolGVR, "openshell-gateway-allow-console"},
	)
	for _, p := range dynamicProbes {
		if _, err := dynamicClient.Resource(p.gvr).Namespace(namespace).Get(ctx, p.name, metav1.GetOptions{}); err == nil {
			return false, nil
		} else if !k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("probe %s/%s in %s: %w", p.gvr.Resource, p.name, namespace, err)
		}
	}

	if ingressMode == IngressModeGatewayAPI {
		if _, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "openshell-backend-ca", metav1.GetOptions{}); err == nil {
			return false, nil
		} else if !k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("probe configmap openshell-backend-ca in %s: %w", namespace, err)
		}
	}
	if _, err := clientset.CoreV1().Services(namespace).Get(ctx, consoleName, metav1.GetOptions{}); err == nil {
		return false, nil
	} else if !k8serrors.IsNotFound(err) {
		return false, fmt.Errorf("probe service %s in %s: %w", consoleName, namespace, err)
	}
	if _, err := clientset.CoreV1().Secrets(namespace).Get(ctx, consoleSecretName, metav1.GetOptions{}); err == nil {
		return false, nil
	} else if !k8serrors.IsNotFound(err) {
		return false, fmt.Errorf("probe secret %s in %s: %w", consoleSecretName, namespace, err)
	}

	// Probe the external Keycloak console client last: it is a realm object, not a
	// namespaced one, and a stale provisioning pass that recreated only the client
	// -- e.g. failing before its namespaced Secret and Deployment writes -- would
	// otherwise be invisible above and let teardown settle while the client leaks.
	// Skipped when unconfigured (nil checker / empty ID). A probe error is returned
	// so absence stays unconfirmed rather than being read as gone.
	if consoleClient != nil && consoleClientID != "" {
		exists, err := consoleClient.ConsoleClientExists(ctx, consoleClientID)
		if err != nil {
			return false, fmt.Errorf("probe keycloak console client %s: %w", consoleClientID, err)
		}
		if exists {
			return false, nil
		}
	}

	return true, nil
}

// readServerTLSCA returns the PEM-encoded ca.crt from the per-namespace
// openshell-server-tls secret (issued by the openshell-ca-issuer alongside the
// gateway server certificate). It returns an empty string when the secret or the
// ca.crt key is absent; callers that require the CA (Gateway API BackendTLSPolicy,
// reencrypt Route) treat empty as "not yet available" and retry.
func readServerTLSCA(ctx context.Context, clientset kubernetes.Interface, namespace string) string {
	tlsSecret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, "openshell-server-tls", metav1.GetOptions{})
	if err != nil {
		return ""
	}
	if ca, ok := tlsSecret.Data["ca.crt"]; ok {
		return string(ca)
	}
	return ""
}

// reconcileRouteResources exposes a tenant gateway through an OpenShift Route
// instead of the Gateway API. The TLS termination is selected by
// GATEWAY_ROUTE_TERMINATION (see routeTermination):
//
//   - passthrough (default): the least invasive mode. The gateway pod already
//     terminates TLS with its per-tenant self-signed CA and performs client
//     mTLS, so HAProxy forwards the encrypted connection end-to-end (SNI-routed)
//     with no wildcard cert, cert-manager ClusterIssuer, or external DNS
//     integration required. This is the ingress mode used where the Gateway
//     API/Istio cannot run (e.g. IBM Cloud ROKS).
//   - reencrypt: the router terminates external TLS with its own publicly-trusted
//     wildcard and re-encrypts to the pod, verifying the backend against the
//     openshell-server-tls ca.crt. Clients see a trusted certificate. Used on
//     ROSA/OpenShift where a *.apps wildcard is already provisioned on the
//     router. This is safe because the gateway server requires no client mTLS
//     (no client_ca_path in the gateway config), so the router presenting no
//     client certificate is accepted; OIDC remains the sole client auth.
func reconcileRouteResources(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, nsConfig NamespaceConfig, opts ReconcileOpts) error {
	namespace := nsConfig.Name

	hostname, err := deriveGatewayHostname(nsConfig)
	if err != nil {
		log.Printf("WARN %v", err)
		return nil
	}

	termination := routeTermination()

	// reencrypt requires the backend CA so the router can verify the gateway
	// pod's self-signed server certificate. Without it the router falls back to
	// its default trust bundle, which does not include openshell-ca, and every
	// backend connection fails TLS verification. Fail closed: skip creating the
	// Route until the CA is available (the reconcile is retried on the next watch
	// event), rather than publish a broken reencrypt Route.
	tlsConfig := map[string]interface{}{
		// Passthrough preserves the gateway pod's own TLS + client mTLS
		// end-to-end. No router-side certificate is involved.
		"termination":                   "passthrough",
		"insecureEdgeTerminationPolicy": "None",
	}
	if termination == RouteTerminationReencrypt {
		caData := readServerTLSCA(ctx, clientset, namespace)
		if caData == "" {
			return fmt.Errorf("reencrypt Route in %s requires openshell-server-tls ca.crt, which is not yet available", namespace)
		}
		// No certificate/key fields: the router serves its default
		// publicly-trusted wildcard automatically. destinationCACertificate lets
		// the router verify the re-encrypted backend connection to the gateway.
		tlsConfig = map[string]interface{}{
			"termination":                   "reencrypt",
			"insecureEdgeTerminationPolicy": "Redirect",
			"destinationCACertificate":      caData,
		}
	}

	// gRPC streams are long-lived; extend the router timeout well beyond the 30s
	// default so streams are not torn down.
	routeAnnotations := map[string]interface{}{
		"haproxy.router.openshift.io/timeout": "3600s",
	}

	// Per-gateway public certificate for a reencrypt Route. On OpenShift the
	// router advertises ALPN h2 on an edge/reencrypt Route only when the Route
	// carries its own certificate; a Route riding the shared default *.apps
	// wildcard is denied h2 (cross-route connection-coalescing protection), which
	// breaks gRPC (grpcs://) even though reencrypt already fixes UnknownIssuer.
	// When GATEWAY_ROUTE_TLS_ISSUER names a cert-manager ClusterIssuer, annotate
	// the Route so the cert-manager openshift-routes controller mints a
	// certificate from it and injects it into spec.tls.{certificate,key}.
	if issuer := routeTLSIssuer(); termination == RouteTerminationReencrypt && issuer != "" {
		routeAnnotations["cert-manager.io/issuer-name"] = issuer
		routeAnnotations["cert-manager.io/issuer-kind"] = "ClusterIssuer"

		// reconcileResource replaces the whole Route on every reconcile, and the
		// spec built here intentionally omits certificate/key. openshift-routes
		// co-owns this Route (we own termination + destinationCACertificate, it
		// owns the edge cert), so carry forward any certificate/key it has already
		// injected -- otherwise each reconcile strips the cert and flaps h2.
		if cert, key := readInjectedRouteCert(ctx, dynamicClient, namespace); cert != "" {
			tlsConfig["certificate"] = cert
			if key != "" {
				tlsConfig["key"] = key
			}
		}
	}

	publishRouteAddress(ctx, opts, namespace, hostname)

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
				"annotations": routeAnnotations,
			},
			"spec": map[string]interface{}{
				"host": hostname,
				"to": map[string]interface{}{
					"kind":   "Service",
					"name":   "openshell-gateway",
					"weight": int64(100),
				},
				"port": map[string]interface{}{
					"targetPort": "grpc",
				},
				"tls":            tlsConfig,
				"wildcardPolicy": "None",
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, route); err != nil {
		return fmt.Errorf("reconcile Route: %w", err)
	}

	// Allow ingress from the OpenShift router namespace to the gateway ports.
	routerNS := gatewayIngressNamespace()
	ingressRule := map[string]interface{}{
		"ports": []interface{}{
			map[string]interface{}{
				"port":     int64(8080),
				"protocol": "TCP",
			},
		},
		"from": []interface{}{
			map[string]interface{}{
				"namespaceSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"kubernetes.io/metadata.name": routerNS,
					},
				},
			},
		},
	}

	routerNetpol := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-allow-router",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"podSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/instance": "openshell-gateway",
						"app.kubernetes.io/name":     "openshell",
					},
				},
				"policyTypes": []interface{}{"Ingress"},
				"ingress":     []interface{}{ingressRule},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, routerNetpol); err != nil {
		log.Printf("WARN failed to reconcile router NetworkPolicy: %v", err)
	}

	log.Printf("INFO Route resources reconciled in namespace %s (hostname=%s)", namespace, hostname)
	return nil
}

// DeleteRouteResources removes the OpenShift gateway Route, the router
// NetworkPolicy, and all console resources. It also clears the stored route
// address. It attempts all operations and returns all errors so the health loop
// can retry incomplete cleanup.
func DeleteRouteResources(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, namespace string, opts ReconcileOpts) error {
	var errs []error

	routeGVR := schema.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	}
	if err := dynamicClient.Resource(routeGVR).Namespace(namespace).Delete(ctx, "openshell-gateway", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete gateway Route in %s: %w", namespace, err))
	}

	netpolGVR := schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}
	if err := dynamicClient.Resource(netpolGVR).Namespace(namespace).Delete(ctx, "openshell-gateway-allow-router", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete router NetworkPolicy in %s: %w", namespace, err))
	}

	if err := DeleteConsole(ctx, dynamicClient, clientset, namespace, opts); err != nil {
		errs = append(errs, err)
	}

	if opts.UpdateRouteAddress != nil {
		if err := opts.UpdateRouteAddress(ctx, ""); err != nil {
			errs = append(errs, fmt.Errorf("clear routeAddress in %s: %w", namespace, err))
		} else {
			log.Printf("INFO cleared routeAddress for gateway in %s", namespace)
		}
	}

	if len(errs) == 0 {
		log.Printf("INFO Route resources removed from namespace %s", namespace)
	}
	return errors.Join(errs...)
}

func NamespaceExists(ctx context.Context, clientset kubernetes.Interface, namespace string) bool {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	return err == nil
}

func namespaceExists(ctx context.Context, clientset *kubernetes.Clientset, namespace string) bool {
	return NamespaceExists(ctx, clientset, namespace)
}

func CreateManagedNamespace(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	return createNamespace(ctx, clientset, namespace)
}

func createNamespace(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
				ManagedLabel:   ManagedLabelValue,
			},
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace: %w", err)
	}
	log.Printf("INFO created namespace %s", namespace)
	return nil
}

func deployGateway(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	nsConfig NamespaceConfig,
	manifests map[string][]*unstructured.Unstructured,
	images ImageDefaults,
	opts ReconcileOpts,
	hasTrustedCA bool,
) error {
	order := []string{
		"rbac.yaml",
		"serviceaccount.yaml",
		"configmap.yaml",
		"certgen-job.yaml",
		"service.yaml",
		"deployment.yaml",
		"networkpolicy.yaml",
	}

	for _, filename := range order {
		resources, ok := manifests[filename]
		if !ok {
			log.Printf("WARN manifest file %s not found, skipping", filename)
			continue
		}

		for _, manifest := range resources {
			if opts.SkipNetworkPolicies && manifest.GetKind() == "NetworkPolicy" {
				logNetworkPoliciesDisabled()
				continue
			}

			obj, err := ApplyManifestToNamespace(manifest.DeepCopy(), nsConfig.Name, opts.ControlPlaneNamespace, nsConfig.Gateway, images)
			if err != nil {
				return fmt.Errorf("apply substitutions for %s: %w", filename, err)
			}

			if err := ApplyConfigOverrides(obj, nsConfig.Gateway, nsConfig.Name); err != nil {
				return fmt.Errorf("apply config overrides for %s: %w", filename, err)
			}

			if obj.GetKind() == "Deployment" {
				applyConfigHashAnnotation(ctx, clientset, obj, nsConfig.Name)
			}

			if hasTrustedCA && obj.GetKind() == "Deployment" {
				applyTrustedCAOverrides(obj)
			}

			if opts.IsOpenShift && obj.GetKind() == "Deployment" {
				applyOpenShiftOverrides(obj)
			}

			if err := reconcileResource(ctx, dynamicClient, obj); err != nil {
				return fmt.Errorf("reconcile resource from %s: %w", filename, err)
			}

			log.Printf("DEBUG reconciled %s %s in %s", obj.GetKind(), obj.GetName(), nsConfig.Name)
		}
	}

	return nil
}

func waitForSecret(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	watchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fieldSelector := fields.OneTermEqualSelector("metadata.name", name).String()

	for {
		list, err := clientset.CoreV1().Secrets(namespace).List(watchCtx, metav1.ListOptions{
			FieldSelector: fieldSelector,
		})
		if err != nil {
			if watchCtx.Err() != nil {
				return fmt.Errorf("timed out waiting for secret %s/%s: %w", namespace, name, watchCtx.Err())
			}
			return fmt.Errorf("list secret %s/%s: %w", namespace, name, err)
		}
		if len(list.Items) > 0 {
			return nil
		}

		watcher, err := clientset.CoreV1().Secrets(namespace).Watch(watchCtx, metav1.ListOptions{
			FieldSelector:   fieldSelector,
			ResourceVersion: list.ResourceVersion,
		})
		if err != nil {
			if watchCtx.Err() != nil {
				return fmt.Errorf("timed out waiting for secret %s/%s: %w", namespace, name, watchCtx.Err())
			}
			return fmt.Errorf("watch secret %s/%s: %w", namespace, name, err)
		}

		appeared := false
		for event := range watcher.ResultChan() {
			if event.Type == watch.Added || event.Type == watch.Modified {
				appeared = true
				break
			}
		}
		watcher.Stop()

		if appeared {
			log.Printf("INFO secret %s/%s is available", namespace, name)
			return nil
		}

		if watchCtx.Err() != nil {
			return fmt.Errorf("timed out waiting for secret %s/%s: %w", namespace, name, watchCtx.Err())
		}
		log.Printf("INFO watch for secret %s/%s closed early; re-establishing", namespace, name)
	}
}

// GatewayDeploymentName is the name of the primary gateway workload Deployment
// whose readiness gates the Gateway `Running` phase.
const GatewayDeploymentName = "openshell-gateway"

// DeploymentReadiness performs a single, non-blocking check of a Deployment's
// readiness. It returns ready=true when ready replicas meet or exceed desired
// replicas. When the Deployment is not ready, reason carries a short
// human-readable descriptor (e.g. "1/2 replicas ready" or "deployment not
// found") suitable for the Gateway `status` field.
func DeploymentReadiness(ctx context.Context, clientset kubernetes.Interface, namespace, name string) (ready bool, reason string, err error) {
	deploy, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, "deployment not found", nil
		}
		return false, "", fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}

	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	if desired < 1 {
		return false, "deployment has zero desired replicas", nil
	}
	if deploy.Status.ReadyReplicas >= desired {
		return true, "", nil
	}
	return false, fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, desired), nil
}

// WaitForGatewayReady blocks until the openshell-gateway Deployment reaches
// readiness or the timeout elapses. It returns ready=true on readiness, or
// ready=false with the last observed reason when the provisioning readiness
// window expires without the workload becoming ready.
func WaitForGatewayReady(ctx context.Context, clientset *kubernetes.Clientset, namespace string, timeout time.Duration) (bool, string) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastReason := "not ready"
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err().Error()
		case <-deadline:
			return false, lastReason
		case <-ticker.C:
			ready, reason, err := DeploymentReadiness(ctx, clientset, namespace, GatewayDeploymentName)
			if err != nil {
				lastReason = err.Error()
				continue
			}
			if ready {
				return true, ""
			}
			if reason != "" {
				lastReason = reason
			}
		}
	}
}

func reconcileResource(ctx context.Context, dynamicClient dynamic.Interface, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()
	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: kindToResource(gvk.Kind),
	}

	namespace := obj.GetNamespace()
	name := obj.GetName()

	var resourceClient dynamic.ResourceInterface
	if namespace != "" {
		resourceClient = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceClient = dynamicClient.Resource(gvr)
	}

	existing, err := resourceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			_, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("create %s %s: %w", gvk.Kind, name, err)
			}
			log.Printf("INFO created %s %s in %s", gvk.Kind, name, namespace)
			return nil
		}
		return fmt.Errorf("get %s %s: %w", gvk.Kind, name, err)
	}

	if gvk.Kind == "Job" {
		log.Printf("DEBUG job %s already exists, skipping update", name)
		return nil
	}

	if gvk.Kind == "PersistentVolumeClaim" {
		log.Printf("DEBUG PVC %s already exists, skipping update", name)
		return nil
	}

	if gvk.Kind == "ClusterRoleBinding" {
		mergeClusterRoleBindingSubjects(existing, obj)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())

	_, err = resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update %s %s: %w", gvk.Kind, name, err)
	}

	return nil
}

func kindToResource(kind string) string {
	mapping := map[string]string{
		"ServiceAccount":        "serviceaccounts",
		"ConfigMap":             "configmaps",
		"Service":               "services",
		"StatefulSet":           "statefulsets",
		"Deployment":            "deployments",
		"Job":                   "jobs",
		"Role":                  "roles",
		"RoleBinding":           "rolebindings",
		"ClusterRole":           "clusterroles",
		"ClusterRoleBinding":    "clusterrolebindings",
		"NetworkPolicy":         "networkpolicies",
		"Secret":                "secrets",
		"PersistentVolumeClaim": "persistentvolumeclaims",
		"Issuer":                "issuers",
		"Certificate":           "certificates",
		"Gateway":               "gateways",
		"GRPCRoute":             "grpcroutes",
		"HTTPRoute":             "httproutes",
		"BackendTLSPolicy":      "backendtlspolicies",
		"Route":                 "routes",
		"Cluster":               "clusters",
		"Database":              "databases",
		"DatabaseRole":          "databaseroles",
	}

	if resource, ok := mapping[kind]; ok {
		return resource
	}

	log.Printf("DEBUG unknown kind %s, using naive plural", kind)
	return strings.ToLower(kind) + "s"
}

func mergeClusterRoleBindingSubjects(existing, desired *unstructured.Unstructured) {
	existingSubjects, _, _ := unstructured.NestedSlice(existing.Object, "subjects")
	desiredSubjects, _, _ := unstructured.NestedSlice(desired.Object, "subjects")

	seen := make(map[string]bool)
	for _, s := range desiredSubjects {
		sub, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := sub["name"].(string)
		ns, _ := sub["namespace"].(string)
		seen[name+"/"+ns] = true
	}

	for _, s := range existingSubjects {
		sub, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := sub["name"].(string)
		ns, _ := sub["namespace"].(string)
		if !seen[name+"/"+ns] {
			desiredSubjects = append(desiredSubjects, s)
			seen[name+"/"+ns] = true
		}
	}

	_ = unstructured.SetNestedSlice(desired.Object, desiredSubjects, "subjects")
}

func applyConfigHashAnnotation(ctx context.Context, clientset *kubernetes.Clientset, obj *unstructured.Unstructured, namespace string) {
	h := sha256.New()

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "openshell-gateway-config", metav1.GetOptions{})
	if err == nil {
		keys := make([]string, 0, len(cm.Data))
		for k := range cm.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte(cm.Data[k]))
		}
	} else if !k8serrors.IsNotFound(err) {
		log.Printf("WARN skipping config-hash annotation in %s: failed to get ConfigMap: %v", namespace, err)
		return
	}

	for _, secretName := range []string{"openshell-server-tls", "openshell-gateway-db-credentials"} {
		secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err == nil {
			keys := make([]string, 0, len(secret.Data))
			for k := range secret.Data {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				h.Write([]byte(k))
				h.Write(secret.Data[k])
			}
		} else if !k8serrors.IsNotFound(err) {
			log.Printf("WARN skipping config-hash annotation in %s: failed to get Secret %s: %v", namespace, secretName, err)
			return
		}
	}

	hashStr := hex.EncodeToString(h.Sum(nil))

	annotations, _, _ := unstructured.NestedMap(obj.Object, "spec", "template", "metadata", "annotations")
	if annotations == nil {
		annotations = make(map[string]interface{})
	}
	annotations["hypershell.redhat.io/config-hash"] = hashStr
	_ = unstructured.SetNestedMap(obj.Object, annotations, "spec", "template", "metadata", "annotations")
}

func applyOpenShiftOverrides(obj *unstructured.Unstructured) {
	unstructured.RemoveNestedField(obj.Object, "spec", "template", "spec", "securityContext", "fsGroup")

	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return
	}
	for i, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		unstructured.RemoveNestedField(container, "securityContext", "runAsUser")
		containers[i] = container
	}
	_ = unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers")
}

func reconcileOpenShiftSCC(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	roleBindingGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "rolebindings",
	}
	bindingName := "openshell-sandbox-privileged-scc"
	binding := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      bindingName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "system:openshift:scc:privileged",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "openshell-gateway-sandbox",
					"namespace": namespace,
				},
			},
		},
	}

	existing, err := dynamicClient.Resource(roleBindingGVR).Namespace(namespace).Get(ctx, bindingName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get SCC RoleBinding: %w", err)
		}
		if _, createErr := dynamicClient.Resource(roleBindingGVR).Namespace(namespace).Create(ctx, binding, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("create SCC RoleBinding: %w", createErr)
		}
		log.Printf("INFO created privileged SCC binding for openshell-gateway-sandbox in %s", namespace)
		return nil
	}

	binding.SetResourceVersion(existing.GetResourceVersion())
	if _, err := dynamicClient.Resource(roleBindingGVR).Namespace(namespace).Update(ctx, binding, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update SCC RoleBinding: %w", err)
	}
	return nil
}

func reconcileTrustedCABundle(ctx context.Context, clientset *kubernetes.Clientset, cpNamespace, targetNamespace string) bool {
	if cpNamespace == "" {
		return false
	}

	caConfigMapName := "gateway-trusted-ca"
	sourceCM, err := clientset.CoreV1().ConfigMaps(cpNamespace).Get(ctx, caConfigMapName, metav1.GetOptions{})
	if err != nil {
		return false
	}

	targetCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caConfigMapName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		Data: sourceCM.Data,
	}

	existing, err := clientset.CoreV1().ConfigMaps(targetNamespace).Get(ctx, caConfigMapName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := clientset.CoreV1().ConfigMaps(targetNamespace).Create(ctx, targetCM, metav1.CreateOptions{}); err != nil {
				log.Printf("WARN failed to create trusted CA ConfigMap in %s: %v", targetNamespace, err)
				return false
			}
			log.Printf("INFO copied trusted CA ConfigMap to %s", targetNamespace)
			return true
		}
		log.Printf("WARN failed to get trusted CA ConfigMap in %s: %v", targetNamespace, err)
		return false
	}

	targetCM.ResourceVersion = existing.ResourceVersion
	if _, err := clientset.CoreV1().ConfigMaps(targetNamespace).Update(ctx, targetCM, metav1.UpdateOptions{}); err != nil {
		log.Printf("WARN failed to update trusted CA ConfigMap in %s: %v", targetNamespace, err)
		return false
	}
	return true
}

func applyTrustedCAOverrides(obj *unstructured.Unstructured) {
	volumes, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
	if !found {
		return
	}

	caVolume := map[string]interface{}{
		"name": "trusted-ca",
		"configMap": map[string]interface{}{
			"name": "gateway-trusted-ca",
		},
	}
	volumes = append(volumes, caVolume)
	_ = unstructured.SetNestedSlice(obj.Object, volumes, "spec", "template", "spec", "volumes")

	containers, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if !found {
		return
	}
	for i, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(container, "name")
		if name != "openshell-gateway" {
			continue
		}

		volumeMounts, _, _ := unstructured.NestedSlice(container, "volumeMounts")
		volumeMounts = append(volumeMounts, map[string]interface{}{
			"name":      "trusted-ca",
			"mountPath": "/etc/pki/tls/certs/hypershell-ca-bundle.crt",
			"subPath":   "ca-bundle.crt",
			"readOnly":  true,
		})
		_ = unstructured.SetNestedSlice(container, volumeMounts, "volumeMounts")

		env, _, _ := unstructured.NestedSlice(container, "env")
		env = append(env, map[string]interface{}{
			"name":  "SSL_CERT_FILE",
			"value": "/etc/pki/tls/certs/hypershell-ca-bundle.crt",
		})
		_ = unstructured.SetNestedSlice(container, env, "env")

		containers[i] = container
	}
	_ = unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers")
}

func cnpgResourceName(gatewayID string) string {
	return "gw-" + strings.ToLower(gatewayID)
}

func cnpgPGName(gatewayID string) string {
	return "gw_" + strings.ToLower(gatewayID)
}

func reconcileCNPGDatabaseResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	tenantNamespace string,
	gatewayID string,
	cnpg CNPGConfig,
) error {
	crName := cnpgResourceName(gatewayID)
	pgName := cnpgPGName(gatewayID)
	passwordSecretName := crName + "-credentials"

	log.Printf("INFO CNPG provisioning: gateway=%s cr=%s db=%s cluster=%s/%s tenant=%s",
		gatewayID, crName, pgName, cnpg.ClusterNamespace, cnpg.ClusterName, tenantNamespace)

	_, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Get(ctx, passwordSecretName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get CNPG password secret: %w", err)
		}

		passwordBytes := make([]byte, 32)
		if _, err := rand.Read(passwordBytes); err != nil {
			return fmt.Errorf("generate database password: %w", err)
		}
		password := hex.EncodeToString(passwordBytes)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      passwordSecretName,
				Namespace: cnpg.ClusterNamespace,
				Labels: map[string]string{
					"cnpg.io/reload":                         "true",
					"hypershell.redhat.io/managed":           "true",
					"hypershell.redhat.io/gateway-namespace": tenantNamespace,
				},
			},
			Type: corev1.SecretTypeBasicAuth,
			StringData: map[string]string{
				"username": pgName,
				"password": password,
			},
		}
		if _, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create CNPG password secret: %w", err)
		}
		log.Printf("INFO created CNPG password secret %s in %s", passwordSecretName, cnpg.ClusterNamespace)
	} else {
		log.Printf("DEBUG CNPG password secret %s already exists in %s, skipping creation", passwordSecretName, cnpg.ClusterNamespace)
	}

	log.Printf("INFO reconciling CNPG DatabaseRole %s in %s (cluster=%s)", crName, cnpg.ClusterNamespace, cnpg.ClusterName)
	role := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "DatabaseRole",
			"metadata": map[string]interface{}{
				"name":      crName,
				"namespace": cnpg.ClusterNamespace,
				"labels": map[string]interface{}{
					"hypershell.redhat.io/managed":           "true",
					"hypershell.redhat.io/gateway-namespace": tenantNamespace,
				},
			},
			"spec": map[string]interface{}{
				"cluster": map[string]interface{}{
					"name": cnpg.ClusterName,
				},
				"name":  pgName,
				"login": true,
				"passwordSecret": map[string]interface{}{
					"name": passwordSecretName,
				},
				"databaseRoleReclaimPolicy": "delete",
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, role); err != nil {
		return fmt.Errorf("reconcile CNPG DatabaseRole: %w", err)
	}

	log.Printf("INFO reconciling CNPG Database %s in %s (owner=%s)", crName, cnpg.ClusterNamespace, pgName)
	db := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      crName,
				"namespace": cnpg.ClusterNamespace,
				"labels": map[string]interface{}{
					"hypershell.redhat.io/managed":           "true",
					"hypershell.redhat.io/gateway-namespace": tenantNamespace,
				},
			},
			"spec": map[string]interface{}{
				"cluster": map[string]interface{}{
					"name": cnpg.ClusterName,
				},
				"name":                  pgName,
				"owner":                 pgName,
				"databaseReclaimPolicy": "delete",
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, db); err != nil {
		return fmt.Errorf("reconcile CNPG Database: %w", err)
	}

	log.Printf("INFO waiting for CNPG Database %s/%s to become ready (timeout=2m)", cnpg.ClusterNamespace, crName)
	if err := waitForCNPGDatabase(ctx, dynamicClient, cnpg.ClusterNamespace, crName, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for CNPG database: %w", err)
	}

	gwSecretName := "openshell-gateway-db-credentials"
	_, err = clientset.CoreV1().Secrets(tenantNamespace).Get(ctx, gwSecretName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get gateway credentials secret: %w", err)
		}

		cnpgSecret, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Get(ctx, passwordSecretName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("read CNPG password secret: %w", err)
		}
		passwordBytes, ok := cnpgSecret.Data["password"]
		if !ok || len(passwordBytes) == 0 {
			return fmt.Errorf("CNPG password secret %s/%s has no password key", cnpg.ClusterNamespace, passwordSecretName)
		}
		password := string(passwordBytes)

		host := fmt.Sprintf("%s-rw.%s.svc.cluster.local", cnpg.ClusterName, cnpg.ClusterNamespace)
		dbURI := fmt.Sprintf("postgresql://%s:%s@%s:5432/%s?sslmode=require",
			pgName, url.QueryEscape(password), host, pgName)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gwSecretName,
				Namespace: tenantNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "database",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"host":     host,
				"port":     "5432",
				"dbname":   pgName,
				"user":     pgName,
				"password": password,
				"uri":      dbURI,
			},
		}
		if _, err := clientset.CoreV1().Secrets(tenantNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create gateway credentials secret: %w", err)
		}
		log.Printf("INFO created gateway credentials secret %s in %s (host=%s db=%s)", gwSecretName, tenantNamespace, host, pgName)
	} else {
		log.Printf("DEBUG gateway credentials secret %s already exists in %s, skipping creation", gwSecretName, tenantNamespace)
	}

	log.Printf("INFO CNPG database provisioning complete for gateway %s in %s", gatewayID, tenantNamespace)
	return nil
}

func copyDeploymentDatabaseCredentials(
	ctx context.Context,
	clientset kubernetes.Interface,
	sourceNamespace string,
	tenantNamespace string,
) error {
	const (
		sourceSecretName = "openshell-db-credentials"
		gwSecretName     = "openshell-gateway-db-credentials"
	)

	sourceSecret, err := clientset.CoreV1().Secrets(sourceNamespace).Get(ctx, sourceSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read source database credentials from %s/%s: %w", sourceNamespace, sourceSecretName, err)
	}

	required := map[string]string{}
	for _, key := range []string{"dbname", "user", "password"} {
		value := string(sourceSecret.Data[key])
		if value == "" {
			return fmt.Errorf("source database credentials %s/%s is missing required key %q", sourceNamespace, sourceSecretName, key)
		}
		required[key] = value
	}

	host := fmt.Sprintf("openshell-gateway-db.%s.svc.cluster.local", sourceNamespace)
	port := "5432"
	dbURI := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		required["user"], url.QueryEscape(required["password"]), host, port, required["dbname"])
	desiredData := map[string][]byte{
		"host":     []byte(host),
		"port":     []byte(port),
		"dbname":   []byte(required["dbname"]),
		"user":     []byte(required["user"]),
		"password": []byte(required["password"]),
		"uri":      []byte(dbURI),
	}
	desiredLabels := map[string]string{
		"app.kubernetes.io/name":       "openshell",
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/managed-by": "hypershell-control-plane",
		"hypershell.redhat.io/managed": "true",
	}

	secrets := clientset.CoreV1().Secrets(tenantNamespace)
	existing, err := secrets.Get(ctx, gwSecretName, metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, err)
	}
	if k8serrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: gwSecretName, Namespace: tenantNamespace, Labels: desiredLabels},
			Type:       corev1.SecretTypeOpaque,
			Data:       desiredData,
		}
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, err)
		}
		log.Printf("INFO copied deployment database credentials to %s (host=%s db=%s)", tenantNamespace, host, required["dbname"])
		return nil
	}

	updated := existing.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	for key, value := range desiredLabels {
		updated.Labels[key] = value
	}
	updated.Type = corev1.SecretTypeOpaque
	updated.Data = desiredData
	if reflect.DeepEqual(existing.Labels, updated.Labels) && existing.Type == updated.Type && reflect.DeepEqual(existing.Data, updated.Data) {
		return nil
	}
	if _, err := secrets.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, err)
	}
	log.Printf("INFO updated deployment database credentials in %s (host=%s db=%s)", tenantNamespace, host, required["dbname"])
	return nil
}

func waitForCNPGDatabase(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string, timeout time.Duration) error {
	databaseGVR := schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "databases",
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for CNPG Database %s/%s to become ready", namespace, name)
		case <-ticker.C:
			obj, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					log.Printf("DEBUG CNPG Database %s/%s not found yet, waiting...", namespace, name)
				} else {
					log.Printf("WARN error checking CNPG Database %s/%s: %v", namespace, name, err)
				}
				continue
			}
			applied, _, _ := unstructured.NestedBool(obj.Object, "status", "applied")
			if applied {
				log.Printf("INFO CNPG Database %s/%s is ready (status.applied=true)", namespace, name)
				return nil
			}
			log.Printf("DEBUG CNPG Database %s/%s exists but not ready (status.applied=%v)", namespace, name, applied)
		}
	}
}

func deleteCNPGResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	gatewayID string,
	cnpg CNPGConfig,
) {
	crName := cnpgResourceName(gatewayID)
	ns := cnpg.ClusterNamespace
	log.Printf("INFO deleting CNPG resources for gateway %s: cr=%s namespace=%s", gatewayID, crName, ns)

	databaseGVR := schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "databases",
	}
	if err := dynamicClient.Resource(databaseGVR).Namespace(ns).Delete(ctx, crName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete CNPG Database %s: %v", crName, err)
		}
	} else {
		log.Printf("INFO deleted CNPG Database %s from %s", crName, ns)
	}

	roleGVR := schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "databaseroles",
	}
	if err := dynamicClient.Resource(roleGVR).Namespace(ns).Delete(ctx, crName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete CNPG DatabaseRole %s: %v", crName, err)
		}
	} else {
		log.Printf("INFO deleted CNPG DatabaseRole %s from %s", crName, ns)
	}

	passwordSecretName := crName + "-credentials"
	if err := clientset.CoreV1().Secrets(ns).Delete(ctx, passwordSecretName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete CNPG password secret %s: %v", passwordSecretName, err)
		}
	} else {
		log.Printf("INFO deleted CNPG password secret %s from %s", passwordSecretName, ns)
	}
}

func reconcileKeycloakClient(ctx context.Context, opts ReconcileOpts, nsConfig *NamespaceConfig) error {
	kc := keycloak.NewClient(
		opts.Keycloak.ServerURL,
		opts.Keycloak.Realm,
		opts.Keycloak.ClientID,
		opts.Keycloak.ClientSecret,
	)

	if opts.GatewayName == "" {
		return fmt.Errorf("gateway name is required for keycloak provisioning")
	}
	if opts.GatewayID == "" {
		return fmt.Errorf("gateway ID is required for keycloak provisioning")
	}
	kcClientID := fmt.Sprintf("%s-%s", opts.GatewayName, opts.GatewayID)

	existingUUID, err := kc.GetClientUUID(ctx, kcClientID)
	if err != nil {
		return fmt.Errorf("check existing keycloak client: %w", err)
	}

	if existingUUID != "" {
		if err := kc.EnsureDeviceAuthorizationGrant(ctx, existingUUID); err != nil {
			return fmt.Errorf("reconcile device authorization grant on keycloak client %s: %w", kcClientID, err)
		}
		log.Printf("INFO reconciled keycloak client %s (uuid=%s)", kcClientID, existingUUID)
	} else {
		clientUUID, err := kc.ProvisionGatewayClient(ctx, kcClientID)
		if err != nil {
			return fmt.Errorf("provision keycloak client %s: %w", kcClientID, err)
		}
		log.Printf("INFO provisioned keycloak client %s (uuid=%s)", kcClientID, clientUUID)
	}

	oidcConfig := OIDCConfig{
		Issuer:     kc.Issuer(),
		ClientID:   kcClientID,
		Audience:   kcClientID,
		JwksTTL:    3600,
		RolesClaim: "hypershell.roles",
		AdminRole:  "openshell-admin",
		UserRole:   "openshell-user",
	}
	// The Keycloak Admin API server URL must be reachable in-cluster, but the
	// gateway's client-facing issuer (consumed by the gateway pod, console, and
	// CLI) may need to be a separately reachable URL. When GATEWAY_OIDC_ISSUER_URL
	// is set it overrides the admin-derived issuer; it MUST equal Keycloak's
	// KC_HOSTNAME so the token `iss` claim validates. Unset preserves 98's default.
	if issuerURL := os.Getenv("GATEWAY_OIDC_ISSUER_URL"); issuerURL != "" {
		oidcConfig.Issuer = issuerURL
	}
	nsConfig.Gateway.OIDC = oidcConfig

	if opts.UpdateOIDC != nil {
		oidcJSON, err := json.Marshal(oidcConfig)
		if err != nil {
			return fmt.Errorf("marshal oidc config: %w", err)
		}
		if err := opts.UpdateOIDC(ctx, string(oidcJSON)); err != nil {
			log.Printf("WARN failed to persist oidc config for %s: %v", kcClientID, err)
		}
	}

	return nil
}

func rotateCNPGDatabaseCredentials(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	tenantNamespace string,
	gatewayID string,
	cnpg CNPGConfig,
	rotateTimestamp string,
) error {
	gwSecretName := "openshell-gateway-db-credentials"
	existing, err := clientset.CoreV1().Secrets(tenantNamespace).Get(ctx, gwSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get gateway credentials secret for rotation: %w", err)
	}

	lastRotation := existing.Annotations["hypershell.redhat.io/last-db-rotation"]
	if lastRotation == rotateTimestamp {
		log.Printf("DEBUG database credentials in %s already rotated at %s, skipping", tenantNamespace, rotateTimestamp)
		return nil
	}

	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return fmt.Errorf("generate new database password: %w", err)
	}
	newPassword := hex.EncodeToString(passwordBytes)

	crName := cnpgResourceName(gatewayID)
	pgName := cnpgPGName(gatewayID)
	passwordSecretName := crName + "-credentials"

	cnpgSecret, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Get(ctx, passwordSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get CNPG password secret for rotation: %w", err)
	}
	cnpgSecret.Data["password"] = []byte(newPassword)
	if _, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Update(ctx, cnpgSecret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update CNPG password secret: %w", err)
	}
	log.Printf("INFO updated CNPG password secret %s in %s", passwordSecretName, cnpg.ClusterNamespace)

	host := fmt.Sprintf("%s-rw.%s.svc.cluster.local", cnpg.ClusterName, cnpg.ClusterNamespace)
	newURI := fmt.Sprintf("postgresql://%s:%s@%s:5432/%s?sslmode=require",
		pgName, url.QueryEscape(newPassword), host, pgName)

	existing.Data["password"] = []byte(newPassword)
	existing.Data["uri"] = []byte(newURI)
	if existing.Annotations == nil {
		existing.Annotations = make(map[string]string)
	}
	existing.Annotations["hypershell.redhat.io/last-db-rotation"] = rotateTimestamp

	if _, err := clientset.CoreV1().Secrets(tenantNamespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update gateway credentials secret after rotation: %w", err)
	}

	log.Printf("INFO rotated database credentials in %s (timestamp=%s)", tenantNamespace, rotateTimestamp)
	return nil
}

// reconcileCredentialKEK uses create-or-skip (not update-or-create) because
// replacing an existing key would render all previously encrypted credentials
// unrecoverable.
func reconcileCredentialKEK(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	secretName := "openshell-gateway-credential-kek"
	_, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		log.Printf("DEBUG credential KEK secret %s already exists in %s, skipping", secretName, namespace)
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get credential KEK secret: %w", err)
	}

	kekBytes := make([]byte, 32)
	if _, err := rand.Read(kekBytes); err != nil {
		return fmt.Errorf("generate credential KEK: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"key-encryption-key": []byte(base64.StdEncoding.EncodeToString(kekBytes)),
		},
	}

	if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create credential KEK secret: %w", err)
	}

	log.Printf("INFO created credential KEK secret %s in %s", secretName, namespace)
	return nil
}

func reconcileCredentialDriverResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	nsConfig NamespaceConfig,
) error {
	driver := nsConfig.Gateway.CredentialDriver
	if driver.Type == "kubernetes-secrets" {
		credNS := nsConfig.Name
		if driver.KubernetesSecrets != nil && driver.KubernetesSecrets.Namespace != "" {
			credNS = driver.KubernetesSecrets.Namespace
		}
		if err := reconcileCredentialSecretsRBAC(ctx, dynamicClient, nsConfig.Name, credNS); err != nil {
			return fmt.Errorf("reconcile credential secrets RBAC: %w", err)
		}
	} else {
		deleteCredentialSecretsRBAC(ctx, dynamicClient, nsConfig.Name)
	}
	return nil
}

func deleteCredentialSecretsRBAC(ctx context.Context, dynamicClient dynamic.Interface, namespace string) {
	roleGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "roles",
	}
	roleBindingGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "rolebindings",
	}

	name := "openshell-gateway-credential-secrets"
	if err := dynamicClient.Resource(roleBindingGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		log.Printf("WARN failed to delete credential secrets RoleBinding in %s: %v", namespace, err)
	}
	if err := dynamicClient.Resource(roleGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		log.Printf("WARN failed to delete credential secrets Role in %s: %v", namespace, err)
	}
}

func reconcileCredentialSecretsRBAC(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	gatewayNamespace, credentialNamespace string,
) error {
	managedLabels := map[string]interface{}{
		"app.kubernetes.io/name":       "openshell",
		"app.kubernetes.io/component":  "gateway",
		"app.kubernetes.io/managed-by": "hypershell-control-plane",
		"hypershell.redhat.io/managed": "true",
	}

	role := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-credential-secrets",
				"namespace": credentialNamespace,
				"labels":    managedLabels,
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{""},
					"resources": []interface{}{"secrets"},
					"verbs":     []interface{}{"get", "create", "patch", "delete"},
				},
			},
		},
	}

	roleBinding := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-credential-secrets",
				"namespace": credentialNamespace,
				"labels":    managedLabels,
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "openshell-gateway-credential-secrets",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "openshell-gateway",
					"namespace": gatewayNamespace,
				},
			},
		},
	}

	if err := reconcileResource(ctx, dynamicClient, role); err != nil {
		return fmt.Errorf("reconcile credential secrets Role: %w", err)
	}
	if err := reconcileResource(ctx, dynamicClient, roleBinding); err != nil {
		return fmt.Errorf("reconcile credential secrets RoleBinding: %w", err)
	}

	log.Printf("INFO reconciled credential secrets RBAC in %s for gateway in %s", credentialNamespace, gatewayNamespace)
	return nil
}

func DetectOpenShift(clientset *kubernetes.Clientset) bool {
	_, resources, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Printf("WARN failed to discover API groups, assuming non-OpenShift: %v", err)
		return false
	}
	for _, list := range resources {
		if strings.HasPrefix(list.GroupVersion, "route.openshift.io/") {
			return true
		}
	}
	return false
}

func DetectCertManager(clientset *kubernetes.Clientset) bool {
	_, resources, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Printf("WARN failed to discover API groups for cert-manager detection: %v", err)
		return false
	}
	for _, list := range resources {
		if strings.HasPrefix(list.GroupVersion, "cert-manager.io/") {
			return true
		}
	}
	return false
}

// cnpgAPIGroupVersion is the exact CNPG API group and version this codebase
// integration depends on. Detection is pinned to this version, rather than
// any postgresql.cnpg.io/* prefix, because the resource shapes this code
// builds -- Cluster, Database, and DatabaseRole specs -- are v1-specific.
const cnpgAPIGroupVersion = "postgresql.cnpg.io/v1"

// requiredCNPGResourceNames are the plural resource names this codebase reads
// or writes directly: clusters (ManagedDatabaseReconciler) and databases and
// databaseroles (per-gateway provisioning below). Finding that some
// postgresql.cnpg.io API group merely exists is not sufficient evidence CNPG
// is usable: a partial or version-mismatched install could serve one
// resource but not the others, and that would otherwise surface much later
// as an unstructured-apply 404 deep inside gateway provisioning instead of
// at startup or capability-detection time.
var requiredCNPGResourceNames = []string{"clusters", "databases", "databaseroles"}

// missingCNPGResources reports which of requiredCNPGResourceNames are absent
// from a postgresql.cnpg.io/v1 APIResourceList. A nil list (group/version not
// found at all) is reported as everything missing.
func missingCNPGResources(list *metav1.APIResourceList) []string {
	served := map[string]bool{}
	if list != nil {
		for _, r := range list.APIResources {
			served[r.Name] = true
		}
	}
	var missing []string
	for _, name := range requiredCNPGResourceNames {
		if !served[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// DetectCNPG reports whether the exact CNPG API resources this codebase
// depends on (Cluster, Database, DatabaseRole in postgresql.cnpg.io/v1) are
// served by the cluster. It is a best-effort, non-fatal capability check used
// to gate reconciliation of individual ManagedDatabase and Gateway resources
// whose provider is "cnpg", independent of the control plane configured
// DATABASE_PROVIDER default; see RequireCNPGAPI for the fail-fast startup
// check.
func DetectCNPG(clientset kubernetes.Interface) bool {
	list, err := clientset.Discovery().ServerResourcesForGroupVersion(cnpgAPIGroupVersion)
	if err != nil {
		log.Printf("WARN CNPG operator not detected: %s not found in cluster discovery: %v", cnpgAPIGroupVersion, err)
		return false
	}
	if missing := missingCNPGResources(list); len(missing) > 0 {
		log.Printf("WARN CNPG operator not detected: %s is present but missing required resources %v", cnpgAPIGroupVersion, missing)
		return false
	}
	log.Printf("INFO CNPG operator detected: %s (clusters, databases, databaseroles)", cnpgAPIGroupVersion)
	return true
}

// RequireCNPGAPI verifies that the exact CNPG API resources this codebase
// depends on (Cluster, Database, DatabaseRole in postgresql.cnpg.io/v1) are
// served by the cluster, returning a descriptive, non-fatal error if they are
// not. Callers configured with DATABASE_PROVIDER=cnpg use this at startup to
// fail cleanly before launching any reconcilers, rather than deferring the
// failure to the first CNPG-backed reconciliation. The caller must pass a
// non-nil client; unlike DetectCNPG, this is a startup precondition check,
// not a best-effort capability probe.
func RequireCNPGAPI(clientset kubernetes.Interface) error {
	list, err := clientset.Discovery().ServerResourcesForGroupVersion(cnpgAPIGroupVersion)
	if err != nil {
		return fmt.Errorf("DATABASE_PROVIDER=cnpg requires the CNPG operator %s API group, which was not found: %w", cnpgAPIGroupVersion, err)
	}
	if missing := missingCNPGResources(list); len(missing) > 0 {
		return fmt.Errorf("DATABASE_PROVIDER=cnpg requires CNPG resources %v in %s, but the cluster is missing %v; install or upgrade the CloudNativePG operator, or set DATABASE_PROVIDER=deployment", requiredCNPGResourceNames, cnpgAPIGroupVersion, missing)
	}
	return nil
}

func DetectGatewayAPI(clientset *kubernetes.Clientset) bool {
	_, resources, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Printf("WARN failed to discover API groups for Gateway API detection: %v", err)
		return false
	}
	for _, list := range resources {
		if list.GroupVersion == "gateway.networking.k8s.io/v1" {
			for _, r := range list.APIResources {
				if r.Kind == "GRPCRoute" {
					return true
				}
			}
		}
	}
	return false
}

func gatewayIngressNamespace() string {
	if ns := os.Getenv("GATEWAY_API_GATEWAY_NAMESPACE"); ns != "" {
		return ns
	}
	return "openshift-ingress"
}

func gatewayIngressName() string {
	if name := os.Getenv("GATEWAY_API_GATEWAY_NAME"); name != "" {
		return name
	}
	return ""
}

// Ingress modes select how a tenant gateway is exposed for external traffic.
// Exported so the controller entrypoint can select the matching Gateway Exposure
// adapter (Gateway API vs Route) by the same rule the reconciler uses to emit
// ingress resources.
const (
	IngressModeGatewayAPI = "gateway-api"
	IngressModeRoute      = "route"
	IngressModeNone       = ""
)

// Route TLS termination modes for the "route" ingress mode, selected via
// GATEWAY_ROUTE_TERMINATION.
//
//   - passthrough (default): HAProxy forwards the gateway pod's own TLS
//     end-to-end. No router certificate is involved, so no wildcard cert or DNS
//     is needed, but external clients must trust the per-tenant self-signed
//     openshell-ca. This preserves the ROKS behavior.
//   - reencrypt: the router terminates external TLS with its own publicly-trusted
//     wildcard (e.g. a ROSA/OpenShift *.apps Let's Encrypt cert, served
//     automatically with no certificate on the Route) and re-encrypts to the
//     gateway pod, verifying the backend against the openshell-server-tls ca.crt
//     set as destinationCACertificate. Clients see a trusted certificate, so the
//     UnknownIssuer error is gone with no new LB, DNS, or cert-manager issuer.
const (
	RouteTerminationPassthrough = "passthrough"
	RouteTerminationReencrypt   = "reencrypt"
)

// routeTermination resolves the Route TLS termination for the "route" ingress
// mode from GATEWAY_ROUTE_TERMINATION, defaulting to passthrough. Any
// unrecognized value falls back to passthrough so a typo cannot silently expose
// a gateway with the wrong termination.
func routeTermination() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GATEWAY_ROUTE_TERMINATION")), RouteTerminationReencrypt) {
		return RouteTerminationReencrypt
	}
	return RouteTerminationPassthrough
}

// routeTLSIssuer resolves the cert-manager ClusterIssuer that mints a per-gateway
// public certificate for a reencrypt Route, from GATEWAY_ROUTE_TLS_ISSUER. Empty
// (the default) leaves the Route on the router's shared default *.apps wildcard,
// which does not advertise ALPN h2 and therefore cannot serve gRPC. Only
// meaningful with reencrypt termination; reconcileRouteResources ignores it
// otherwise.
func routeTLSIssuer() string {
	return strings.TrimSpace(os.Getenv("GATEWAY_ROUTE_TLS_ISSUER"))
}

// readInjectedRouteCert returns the certificate and key that the cert-manager
// openshift-routes controller has injected into the existing openshell-gateway
// Route's spec.tls, or empty strings when the Route or those fields are absent.
// The route reconcile does a full replace, so reconcileRouteResources carries
// these forward to avoid clobbering the injected edge certificate (which would
// flap ALPN h2, and hence gRPC, on every reconcile).
func readInjectedRouteCert(ctx context.Context, dynamicClient dynamic.Interface, namespace string) (string, string) {
	gvr := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	existing, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, "openshell-gateway", metav1.GetOptions{})
	if err != nil {
		return "", ""
	}
	cert, _, _ := unstructured.NestedString(existing.Object, "spec", "tls", "certificate")
	key, _, _ := unstructured.NestedString(existing.Object, "spec", "tls", "key")
	return cert, key
}

// IngressMode resolves how tenant-gateway ingress is provisioned from
// GATEWAY_INGRESS_MODE, falling back to a capability-based default.
//
// HyperShell is environment-adaptive: it emits Kubernetes Gateway API resources
// (a GRPCRoute onto a shared Gateway) where the Gateway API is available and
// functional, and OpenShift Routes (HAProxy passthrough) where it is not (e.g.
// IBM Cloud ROKS, which ships the Gateway API CRDs but cannot run the
// CIO-managed Istio). The mode is chosen per environment via the kustomize-set
// env var GATEWAY_INGRESS_MODE, with a sensible capability-based default when it
// is unset.
//
// GATEWAY_INGRESS_MODE values: "gateway-api", "route", or "none"/"off" to
// disable managed ingress. When unset, auto-detect: prefer the Gateway API when
// present, otherwise fall back to Routes on OpenShift. Note that on some
// platforms the Gateway API CRDs exist but do not function (ROKS); those
// operators must set GATEWAY_INGRESS_MODE=route explicitly.
//
// This is the single source of truth for ingress-mode selection: both the
// reconciler (which ingress resources to emit) and the controller entrypoint
// (which exposure adapter to observe readiness through) resolve the mode here,
// so the emitted resource and the observed resource cannot diverge -- the ROKS
// bug where a Route-exposed gateway was observed through the Gateway API adapter
// (because the Gateway API CRDs happened to be present) is thereby impossible.
func IngressMode(hasGatewayAPI, isOpenShift bool) string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_INGRESS_MODE"))) {
	case IngressModeGatewayAPI, "gatewayapi":
		return IngressModeGatewayAPI
	case IngressModeRoute, "routes":
		return IngressModeRoute
	case "none", "off", "disabled":
		return IngressModeNone
	}

	// Auto-detect from cluster capabilities when no explicit override is set.
	if hasGatewayAPI {
		return IngressModeGatewayAPI
	}
	if isOpenShift {
		return IngressModeRoute
	}
	return IngressModeNone
}

// gatewayIngressMode resolves the ingress mode for a reconcile pass from the
// detected cluster capabilities carried on opts.
func gatewayIngressMode(opts ReconcileOpts) string {
	return IngressMode(opts.HasGatewayAPI, opts.IsOpenShift)
}

// deriveGatewayHostname resolves the external hostname for a tenant gateway,
// shared by both ingress modes. An explicit Route.Host wins; otherwise it is
// derived as gw-<namespace>.<GATEWAY_API_BASE_DOMAIN>. The gateway's server
// certificate SANs (ServerDnsNames/ExternalDns) must cover this hostname.
func deriveGatewayHostname(nsConfig NamespaceConfig) (string, error) {
	baseDomain := os.Getenv("GATEWAY_API_BASE_DOMAIN")
	if h := nsConfig.Gateway.Route.Host; h != "" {
		// An explicit host that falls under the operator's shared base domain
		// MUST be this tenant's own slot (gw-<namespace>.<base-domain>).
		// Otherwise a tenant could set Route.Host to another tenant's derived
		// host and hijack its route under the shared wildcard, since OpenShift
		// Route host claiming is first-come. Hosts outside the base domain
		// (genuine external/vanity names) are the operator's responsibility and
		// pass through; empty base domain means no shared wildcard to protect.
		if baseDomain != "" && strings.HasSuffix(h, "."+baseDomain) {
			expected := fmt.Sprintf("gw-%s.%s", nsConfig.Name, baseDomain)
			if h != expected {
				return "", fmt.Errorf("route host %q under base domain %q must equal %q for namespace %q", h, baseDomain, expected, nsConfig.Name)
			}
		}
		return h, nil
	}
	if baseDomain == "" {
		return "", fmt.Errorf("cannot derive gateway hostname: set Route.Host or GATEWAY_API_BASE_DOMAIN")
	}
	return fmt.Sprintf("gw-%s.%s", nsConfig.Name, baseDomain), nil
}

// appendDNSNameIfMissing returns names with hostname appended, unless it is
// empty or already present. Used to add the derived ingress hostname to the
// gateway server certificate SANs without duplicating it.
func appendDNSNameIfMissing(names []string, hostname string) []string {
	if hostname == "" {
		return names
	}
	for _, n := range names {
		if n == hostname {
			return names
		}
	}
	return append(names, hostname)
}

// publishRouteAddress writes the externally reachable gRPC address back to the
// API-server Gateway resource. Shared by both ingress modes.
func publishRouteAddress(ctx context.Context, opts ReconcileOpts, namespace, hostname string) {
	if opts.UpdateRouteAddress == nil {
		return
	}
	routeAddress := fmt.Sprintf("grpcs://%s:443", hostname)
	if err := opts.UpdateRouteAddress(ctx, routeAddress); err != nil {
		log.Printf("WARN failed to publish routeAddress %s for gateway in %s: %v", routeAddress, namespace, err)
	} else {
		log.Printf("INFO published routeAddress %s for gateway in %s", routeAddress, namespace)
	}
}

func reconcileGatewayAPIResources(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, nsConfig NamespaceConfig, opts ReconcileOpts) error {
	namespace := nsConfig.Name
	routeConfig := nsConfig.Gateway.Route

	gwName := gatewayIngressName()
	if gwName == "" {
		log.Printf("WARN GATEWAY_API_GATEWAY_NAME is required -- set it to the name of a pre-existing Gateway resource")
		return fmt.Errorf("GATEWAY_API_GATEWAY_NAME is required")
	}
	gwNS := gatewayIngressNamespace()

	// Derive the external hostname through the Gateway Exposure adapter's shared
	// helper so the hostname baked into the GRPCRoute cannot drift from the
	// address published through the port.
	hostname, ok := exposure.DeriveGatewayAPIHost(namespace, routeConfig.Host)
	if !ok {
		log.Printf("WARN cannot derive GRPCRoute hostname: GATEWAY_API_BASE_DOMAIN not set")
		return nil
	}

	// Publish the deterministic route address through the Gateway Exposure port.
	// The hostname is known before the shared Gateway reports Accepted/Programmed,
	// so the connection command is available to the CLI and console while the
	// gateway finishes provisioning. Readiness is reflected separately by the
	// Gateway phase.
	if opts.Exposure != nil && opts.UpdateRouteAddress != nil {
		routeAddress, err := opts.Exposure.ResolveAddress(ctx, exposure.Request{Namespace: namespace, Host: routeConfig.Host})
		if err != nil {
			log.Printf("WARN failed to resolve routeAddress for gateway in %s: %v", namespace, err)
		} else if routeAddress != "" {
			if err := opts.UpdateRouteAddress(ctx, routeAddress); err != nil {
				log.Printf("WARN failed to publish routeAddress %s for gateway in %s: %v", routeAddress, namespace, err)
			} else {
				log.Printf("INFO published routeAddress %s for gateway in %s", routeAddress, namespace)
			}
		}
	}

	log.Printf("INFO using Gateway %s/%s for tenant %s", gwNS, gwName, namespace)

	parentRef := map[string]interface{}{
		"name":        gwName,
		"namespace":   gwNS,
		"sectionName": "grpc",
	}

	grpcRoute := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GRPCRoute",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{parentRef},
				"hostnames":  []interface{}{hostname},
				"rules": []interface{}{
					map[string]interface{}{
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": "openshell-gateway",
								"port": int64(8080),
							},
						},
					},
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, grpcRoute); err != nil {
		return fmt.Errorf("reconcile GRPCRoute: %w", err)
	}

	if err := waitForSecret(ctx, clientset, namespace, "openshell-server-tls", 60*time.Second); err != nil {
		return fmt.Errorf("wait for server TLS secret in %s: %w", namespace, err)
	}

	// The wait above can run for up to a minute. A route removal (or gateway
	// deletion) during it is observed only by the independent health loop -- the
	// watcher phase gate blocks a re-provision -- which tears down this gateway's
	// route and console. Re-check live route intent before creating the remaining
	// route- and console-owned resources so this in-flight pass does not race that
	// teardown. Fail closed: unknown intent must not authorize new resources, so a
	// check error aborts the pass (the gateway parks at Failed and is retried on
	// the next watch resync) rather than risk creating resources behind a
	// concurrent teardown. The health loop's route teardown verifies actual
	// resource absence (RouteResourcesAbsent), so it still removes anything a
	// narrow check-then-act window lets slip through.
	if opts.RouteStillDesired != nil {
		desired, err := opts.RouteStillDesired(ctx)
		if err != nil {
			return fmt.Errorf("re-check route intent in %s: %w", namespace, err)
		}
		if !desired {
			log.Printf("INFO gateway in %s no longer routed after TLS wait; skipping route/console resource creation (health loop owns teardown)", namespace)
			return nil
		}
	}

	caData := readServerTLSCA(ctx, clientset, namespace)

	if caData != "" {
		backendCA := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshell-backend-ca",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			Data: map[string]string{
				"ca.crt": caData,
			},
		}

		existing, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "openshell-backend-ca", metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				if _, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, backendCA, metav1.CreateOptions{}); err != nil {
					log.Printf("WARN failed to create backend CA ConfigMap: %v", err)
				}
			}
		} else {
			backendCA.ResourceVersion = existing.ResourceVersion
			if _, err := clientset.CoreV1().ConfigMaps(namespace).Update(ctx, backendCA, metav1.UpdateOptions{}); err != nil {
				log.Printf("WARN failed to update backend CA ConfigMap: %v", err)
			}
		}

		btlsPolicy := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.networking.k8s.io/v1",
				"kind":       "BackendTLSPolicy",
				"metadata": map[string]interface{}{
					"name":      "openshell-gateway",
					"namespace": namespace,
					"labels": map[string]interface{}{
						"app.kubernetes.io/name":       "openshell",
						"app.kubernetes.io/component":  "gateway",
						"app.kubernetes.io/managed-by": "hypershell-control-plane",
						"hypershell.redhat.io/managed": "true",
					},
				},
				"spec": map[string]interface{}{
					"targetRefs": []interface{}{
						map[string]interface{}{
							"group": "",
							"kind":  "Service",
							"name":  "openshell-gateway",
						},
					},
					"validation": map[string]interface{}{
						"caCertificateRefs": []interface{}{
							map[string]interface{}{
								"group": "",
								"kind":  "ConfigMap",
								"name":  "openshell-backend-ca",
							},
						},
						"hostname": fmt.Sprintf("openshell-gateway.%s.svc.cluster.local", namespace),
					},
				},
			},
		}
		if err := reconcileResource(ctx, dynamicClient, btlsPolicy); err != nil {
			log.Printf("WARN failed to reconcile BackendTLSPolicy (may require OpenShift 4.22+): %v", err)
		}
	}

	// Build the router → gateway NetworkPolicy unless dev has opted out (Kind's
	// out-of-cluster proxy has a source IP no selector can match, so the policy
	// would blackhole gateway ingress). Restrict source to the namespace hosting
	// the shared Gateway so only the admin-provisioned proxy can reach the ports.
	if opts.SkipNetworkPolicies {
		logNetworkPoliciesDisabled()
	} else {
		ingressRule := map[string]interface{}{
			"ports": []interface{}{
				map[string]interface{}{
					"port":     int64(8080),
					"protocol": "TCP",
				},
			},
			"from": []interface{}{
				map[string]interface{}{
					"namespaceSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"kubernetes.io/metadata.name": gwNS,
						},
					},
				},
			},
		}

		routerNetpol := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "networking.k8s.io/v1",
				"kind":       "NetworkPolicy",
				"metadata": map[string]interface{}{
					"name":      "openshell-gateway-allow-router",
					"namespace": namespace,
					"labels": map[string]interface{}{
						"app.kubernetes.io/name":       "openshell",
						"app.kubernetes.io/component":  "gateway",
						"app.kubernetes.io/managed-by": "hypershell-control-plane",
						"hypershell.redhat.io/managed": "true",
					},
				},
				"spec": map[string]interface{}{
					"podSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app.kubernetes.io/instance": "openshell-gateway",
							"app.kubernetes.io/name":     "openshell",
						},
					},
					"policyTypes": []interface{}{"Ingress"},
					"ingress":     []interface{}{ingressRule},
				},
			},
		}
		if err := reconcileResource(ctx, dynamicClient, routerNetpol); err != nil {
			log.Printf("WARN failed to reconcile router NetworkPolicy: %v", err)
		}
	}

	// The route address is published deterministically at the top of this
	// function, so no readiness-gated discovery is required here.

	// The console follows the route: it is deployed in the same pass that creates
	// the route resources. A console failure must not fail the gateway route
	// reconciliation, so it is logged and the reconcile continues.
	images := opts.Images
	if images == nil {
		images = StaticImageDefaults{}
	}
	if err := reconcileConsole(ctx, dynamicClient, clientset, nsConfig, opts, images); err != nil {
		log.Printf("WARN failed to reconcile console in %s: %v", namespace, err)
	}

	log.Printf("INFO Gateway API resources reconciled in namespace %s (hostname=%s)", namespace, hostname)
	return nil
}

func reconcileCertManagerResources(ctx context.Context, dynamicClient dynamic.Interface, nsConfig NamespaceConfig) error {
	namespace := nsConfig.Name
	dnsNames := nsConfig.Gateway.ServerDnsNames

	selfSignedIssuer := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Issuer",
			"metadata": map[string]interface{}{
				"name":      "openshell-selfsigned",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"selfSigned": map[string]interface{}{},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, selfSignedIssuer); err != nil {
		return fmt.Errorf("reconcile self-signed issuer: %w", err)
	}

	caCert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      "openshell-ca",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"isCA":           true,
				"commonName":     "openshell-ca",
				"secretName":     "openshell-ca-tls",
				"rotationPolicy": "Always",
				"privateKey": map[string]interface{}{
					"algorithm": "ECDSA",
					"size":      int64(256),
				},
				"issuerRef": map[string]interface{}{
					"name":  "openshell-selfsigned",
					"kind":  "Issuer",
					"group": "cert-manager.io",
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, caCert); err != nil {
		return fmt.Errorf("reconcile CA certificate: %w", err)
	}

	caIssuer := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Issuer",
			"metadata": map[string]interface{}{
				"name":      "openshell-ca-issuer",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"ca": map[string]interface{}{
					"secretName": "openshell-ca-tls",
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, caIssuer); err != nil {
		return fmt.Errorf("reconcile CA issuer: %w", err)
	}

	dnsNamesInterface := make([]interface{}, len(dnsNames))
	for i, d := range dnsNames {
		dnsNamesInterface[i] = d
	}

	serverCert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      "openshell-server",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"secretName": "openshell-server-tls",
				"dnsNames":   dnsNamesInterface,
				"issuerRef": map[string]interface{}{
					"name":  "openshell-ca-issuer",
					"kind":  "Issuer",
					"group": "cert-manager.io",
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, serverCert); err != nil {
		return fmt.Errorf("reconcile server certificate: %w", err)
	}

	// The client certificate is NOT for external-client mTLS (external clients
	// authenticate via OIDC over the Route). It exists so sandbox runners can
	// verify the gateway's TLS server cert: openshell 0.0.109's Kubernetes driver
	// mounts this secret into every sandbox and sets OPENSHELL_TLS_CA from its
	// ca.crt whenever gateway.toml sets client_tls_secret_name. Because it is
	// issued by the same openshell-ca-issuer as the server cert, its ca.crt
	// chains to the gateway's server certificate. Without it the sandbox agent
	// crashloops ("OPENSHELL_TLS_CA is required") and never reaches Ready.
	clientCert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      "openshell-client",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"secretName": "openshell-client-tls",
				"commonName": "openshell-client",
				"issuerRef": map[string]interface{}{
					"name":  "openshell-ca-issuer",
					"kind":  "Issuer",
					"group": "cert-manager.io",
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, clientCert); err != nil {
		return fmt.Errorf("reconcile client certificate: %w", err)
	}

	log.Printf("INFO cert-manager resources reconciled in namespace %s", namespace)
	return nil
}
