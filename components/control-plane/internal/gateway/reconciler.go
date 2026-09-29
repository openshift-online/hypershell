package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func ReconcileGateway(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	helmClient *helm.ShellClient,
	nsConfig NamespaceConfig,
	opts ReconcileOpts,
) error {
	report := opts.ReportProgress
	if report == nil {
		report = func(string, string, string) {}
	}

	ingressMode := gatewayIngressMode(opts)

	// Step 1: EnvironmentReady
	report(ConditionEnvironmentReady, StatusInProgress, "")

	if err := EnsureManagedNamespace(ctx, clientset, nsConfig.Name, opts.ControlPlaneNamespace); err != nil {
		report(ConditionEnvironmentReady, StatusFailed, "Environment preparation failed - unable to set up the gateway namespace")
		return fmt.Errorf("ensure namespace %s: %w", nsConfig.Name, err)
	}

	if err := ValidateGatewayConfig(nsConfig.Gateway); err != nil {
		report(ConditionEnvironmentReady, StatusFailed, "Environment preparation failed - the gateway configuration is invalid")
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
			if nsConfig.Gateway.Route.Host == "" {
				nsConfig.Gateway.Route.Host = hostname
			}
		}
	}

	report(ConditionEnvironmentReady, StatusComplete, "")

	// Step 2: DatabaseReady
	report(ConditionDatabaseReady, StatusInProgress, "")

	dbReconciler, err := newDatabaseReconciler(opts)
	if err != nil {
		report(ConditionDatabaseReady, StatusFailed, "Database provisioning failed - the database service is unavailable")
		return fmt.Errorf("database reconciler for gateway in namespace %s: %w", nsConfig.Name, err)
	}
	if err := dbReconciler.Reconcile(ctx, dynamicClient, clientset, nsConfig.Name, opts.GatewayID); err != nil {
		log.Printf("ERROR gateway database provisioning in %s: %v", nsConfig.Name, err)
		report(ConditionDatabaseReady, StatusFailed, "Database provisioning failed - unable to provision the gateway database")
		return err
	}

	// Credential driver resources (Vault RBAC, etc.) are not managed by the
	// Helm chart; reconcile them here. The chart handles the default credential
	// KEK secret.
	if nsConfig.Gateway.CredentialDriver != nil {
		if err := reconcileCredentialDriverResources(ctx, dynamicClient, clientset, nsConfig); err != nil {
			report(ConditionDatabaseReady, StatusFailed, "Database provisioning failed - unable to configure credential storage")
			return fmt.Errorf("reconcile credential driver resources in %s: %w", nsConfig.Name, err)
		}
	}

	report(ConditionDatabaseReady, StatusComplete, "")

	// Step 3: IdentityProviderReady (only when Keycloak is configured)
	if opts.Keycloak != nil {
		report(ConditionIdentityProviderReady, StatusInProgress, "")
		if err := reconcileKeycloakClient(ctx, opts, &nsConfig); err != nil {
			report(ConditionIdentityProviderReady, StatusFailed, "Identity provider configuration failed - the authentication service is currently unavailable")
			return fmt.Errorf("reconcile keycloak client in %s: %w", nsConfig.Name, err)
		}
		report(ConditionIdentityProviderReady, StatusComplete, "")
	}

	// Step 4: GatewayDeployed
	report(ConditionGatewayDeployed, StatusInProgress, "")

	// Copy trusted CA bundle (for OIDC issuer verification)
	hasTrustedCA := reconcileTrustedCABundle(ctx, clientset, opts.ControlPlaneNamespace, nsConfig.Name)

	// Reconcile OpenShift SCC binding BEFORE Helm install
	// (sandbox pods need privileged SCC to schedule)
	if opts.IsOpenShift {
		if err := reconcileOpenShiftSCC(ctx, dynamicClient, nsConfig.Name); err != nil {
			log.Printf("WARN failed to reconcile OpenShift SCC binding in %s: %v", nsConfig.Name, err)
		}
	}

	// Deploy gateway via Helm
	// The chart handles: Deployment, Services, RBAC, cert-manager, GRPCRoute,
	// BackendTLSPolicy, Route, credential KEK
	if err := deployGatewayViaHelm(ctx, helmClient, nsConfig, opts, hasTrustedCA); err != nil {
		report(ConditionGatewayDeployed, StatusFailed, "Gateway deployment failed - unable to deploy the gateway workload")
		return fmt.Errorf("deploy gateway via helm in %s: %w", nsConfig.Name, err)
	}

	// Console reconciliation and route-address publishing are not managed by
	// the Helm chart. Reconcile them after the Helm release so the gateway
	// workload is already deployed.
	switch ingressMode {
	case IngressModeGatewayAPI:
		if nsConfig.Gateway.Route.Enabled {
			if err := reconcileGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig, opts); err != nil {
				report(ConditionGatewayDeployed, StatusFailed, "Gateway deployment failed - unable to configure network routing")
				return fmt.Errorf("reconcile Gateway API resources in %s: %w", nsConfig.Name, err)
			}
		} else {
			if err := DeleteGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig.Name, opts); err != nil {
				log.Printf("WARN failed to remove Gateway API resources in %s: %v", nsConfig.Name, err)
			}
		}
	case IngressModeRoute:
		if nsConfig.Gateway.Route.Enabled {
			if err := reconcileRouteResources(ctx, dynamicClient, nsConfig, opts); err != nil {
				log.Printf("WARN failed to reconcile Route resources in %s: %v", nsConfig.Name, err)
			}
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

	report(ConditionGatewayDeployed, StatusComplete, "")

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
	helmClient *helm.ShellClient,
	namespace string,
	opts ReconcileOpts,
	credentialNamespaces ...string,
) error {
	// Uninstall Helm release (removes all chart-managed resources in the namespace)
	if helmClient != nil {
		if err := helmClient.Uninstall(ctx, namespace); err != nil {
			log.Printf("WARN failed to uninstall helm release in namespace %s: %v", namespace, err)
		}
	}

	crbGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "clusterrolebindings",
	}
	crbName := fmt.Sprintf("openshell-gateway-node-reader-%s", namespace)
	if err := dynamicClient.Resource(crbGVR).Delete(ctx, crbName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete ClusterRoleBinding %s: %v", crbName, err)
			// This cluster-scoped binding has no owning namespace to cascade-reap
			// it and no reconciler that reclaims leaked bindings, so a failure here
			// is a silent orphan unless it is recorded durably.
			recordOrphan(ctx, opts, "ClusterRoleBinding", crbName,
				fmt.Sprintf("delete failed during gateway deletion: %v", err))
		}
	} else {
		log.Printf("INFO deleted ClusterRoleBinding %s", crbName)
	}

	if opts.KeycloakClient != nil && opts.GatewayID != "" {
		kcClientID := opts.GatewayClientID
		// Defensive for other callers; GatewayReconciler already passes the validated stored identity, including name-id fallback.
		if kcClientID == "" && opts.GatewayName != "" {
			kcClientID = fmt.Sprintf("%s-%s", opts.GatewayName, opts.GatewayID)
		}
		if kcClientID == "" {
			return fmt.Errorf("gateway identity is required for cleanup")
		}
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
			recordOrphan(ctx, opts, "KeycloakClient", consoleClientID,
				fmt.Sprintf("delete failed during gateway deletion: %v", err))
		} else {
			log.Printf("INFO deleted console client %s", consoleClientID)
		}

		if err := opts.KeycloakClient.DeleteGatewayClient(ctx, kcClientID); err != nil {
			log.Printf("WARN failed to delete keycloak client %s (orphaned): %v", kcClientID, err)
			recordOrphan(ctx, opts, "KeycloakClient", kcClientID,
				fmt.Sprintf("delete failed during gateway deletion: %v", err))
		} else {
			log.Printf("INFO deleted keycloak client %s", kcClientID)
		}
	}

	dbReconciler, err := newDatabaseReconciler(opts)
	if err != nil {
		return fmt.Errorf("database reconciler for gateway %s delete: %w", opts.GatewayID, err)
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cleanupCancel()
	if delErr := dbReconciler.Delete(cleanupCtx, dynamicClient, clientset, opts.GatewayID); delErr != nil {
		// The database and role may still exist on the server. Record the
		// leftover so it is operator-visible even if the controller restarts and
		// loses the queued retry, then return the error so the delete-reconcile
		// retries (gateway-deletion-finalization.spec.md).
		recordOrphan(ctx, opts, "PostgreSQLDatabase", gatewayDBName(opts.GatewayID),
			fmt.Sprintf("database cleanup failed during gateway deletion; the gateway database and role may remain on the server and the delete will be retried: %v", delErr))
		return fmt.Errorf("database cleanup for gateway %s: %w", opts.GatewayID, delErr)
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

// recordOrphan invokes opts.RecordOrphan if the caller wired one, so a
// best-effort deletion failure that leaves a gateway-owned resource behind is
// surfaced durably instead of only logged. It is a no-op when no recorder is
// configured, keeping the best-effort branches backward compatible.
func recordOrphan(ctx context.Context, opts ReconcileOpts, resourceKind, resourceName, reason string) {
	if opts.RecordOrphan != nil {
		opts.RecordOrphan(ctx, resourceKind, resourceName, reason)
	}
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
// route: it removes the GRPCRoute, BackendTLSPolicy, and backend-CA ConfigMap,
// tears down the console (which follows the route), and clears the stored
// route_address. It attempts every deletion regardless of individual failures
// and returns their joined errors (nil once everything is absent), so a caller
// can retry until the route and its console are fully gone rather than stopping
// on partial cleanup. Idempotent: absent resources are ignored.
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

	if err := clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, "openshell-gateway-backend-ca", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete backend CA ConfigMap in %s: %w", namespace, err))
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
		)
	case IngressModeRoute:
		dynamicProbes = append(dynamicProbes,
			dynamicProbe{openShiftRouteGVR, "openshell-gateway"},
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
	)
	for _, p := range dynamicProbes {
		if _, err := dynamicClient.Resource(p.gvr).Namespace(namespace).Get(ctx, p.name, metav1.GetOptions{}); err == nil {
			return false, nil
		} else if !k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("probe %s/%s in %s: %w", p.gvr.Resource, p.name, namespace, err)
		}
	}

	if ingressMode == IngressModeGatewayAPI {
		if _, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "openshell-gateway-backend-ca", metav1.GetOptions{}); err == nil {
			return false, nil
		} else if !k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("probe configmap openshell-gateway-backend-ca in %s: %w", namespace, err)
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

// reconcileRouteResources handles the non-Route resources needed when the
// gateway is exposed through an OpenShift Route. The Route itself is owned by
// the Helm chart (openshiftRoute.enabled=true in the chart values); this
// function publishes the route address back to the API server.
func reconcileRouteResources(ctx context.Context, dynamicClient dynamic.Interface, nsConfig NamespaceConfig, opts ReconcileOpts) error {
	namespace := nsConfig.Name

	hostname, err := deriveGatewayHostname(nsConfig)
	if err != nil {
		log.Printf("WARN %v", err)
		return nil
	}

	publishRouteAddress(ctx, opts, namespace, hostname)

	log.Printf("INFO Route resources reconciled in namespace %s (hostname=%s)", namespace, hostname)
	return nil
}

// DeleteRouteResources removes the OpenShift gateway Route and all console
// resources. It also clears the stored route address. It attempts all
// operations and returns all errors so the health loop can retry incomplete
// cleanup.
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

// AppliedReleaseAnnotation records, on the gateway Deployment's metadata, the
// GatewayRelease id the Deployment's current pod template was rendered from. The
// control plane stamps it at apply time so the continuous health loop can advance
// observed_release_id only to the release actually applied to the workload -- not
// to a desired release the provisioning path has committed to the database but not
// yet rolled out. Empty for a direct-image gateway. See
// gateway-release-rollout.spec.md.
const AppliedReleaseAnnotation = "hypershell.redhat.io/applied-release-id"

// AppliedRelease returns the GatewayRelease id the given gateway Deployment was
// rendered from, read from AppliedReleaseAnnotation, or "" when unset (a
// direct-image gateway, or a Deployment applied before this annotation existed).
func AppliedRelease(deploy *appsv1.Deployment) string {
	if deploy == nil {
		return ""
	}
	return deploy.Annotations[AppliedReleaseAnnotation]
}

// deploymentRolloutComplete judges a Deployment's rollout on its *new* revision,
// not on any still-Ready old pod. It reports complete=true only when the
// Deployment's spec change has been observed by its controller, its updated
// replicas are available at the desired count, and no old replicas remain. It
// also reports rollingOut=true when a new revision is still being rolled out
// (spec not yet observed, updated replicas not yet at desired, or old replicas
// still terminating), so callers can distinguish an in-progress rollout from a
// steady-state degradation of the current revision. With maxUnavailable:0 and a
// positive maxSurge, a still-Ready old pod would satisfy a plain
// ReadyReplicas>=desired check while the new revision is still starting or
// crash-looping; judging on the updated replicas closes that gap. When the
// updated revision is fully rolled out but its pods are not all available, the
// current revision is unhealthy (rollingOut=false). See
// gateway-release-rollout.spec.md.
func deploymentRolloutComplete(deploy *appsv1.Deployment) (complete bool, rollingOut bool, reason string) {
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	if desired < 1 {
		return false, false, "deployment has zero desired replicas"
	}
	if deploy.Status.ObservedGeneration < deploy.Generation {
		return false, true, "waiting for deployment spec update to be observed"
	}
	if deploy.Status.UpdatedReplicas < desired {
		return false, true, fmt.Sprintf("%d/%d updated replicas rolled out", deploy.Status.UpdatedReplicas, desired)
	}
	if deploy.Status.Replicas > deploy.Status.UpdatedReplicas {
		old := deploy.Status.Replicas - deploy.Status.UpdatedReplicas
		if deploy.Status.AvailableReplicas < deploy.Status.Replicas {
			// With maxUnavailable:0 the old replica(s) are kept running until the
			// updated revision becomes available, so an unavailable pod during the
			// surge window means the new revision is not up yet (e.g.
			// ImagePullBackOff or a slow start), not that an old replica is winding
			// down. Report that truthfully so an operator debugging a stuck roll is
			// not misdirected to a healthy-looking termination message.
			return false, true, fmt.Sprintf("updated revision not yet available; %d old replica(s) retained", old)
		}
		return false, true, fmt.Sprintf("waiting for %d old replica(s) to terminate", old)
	}
	if deploy.Status.AvailableReplicas < desired {
		return false, false, fmt.Sprintf("%d/%d updated replicas available", deploy.Status.AvailableReplicas, desired)
	}
	return true, false, ""
}

// DeploymentReadiness performs a single, non-blocking check of a Deployment's
// readiness. It returns ready=true when ready replicas meet or exceed desired
// replicas. When the Deployment is not ready, reason carries a short
// human-readable descriptor (e.g. "1/2 replicas ready" or "deployment not
// found") suitable for the Gateway `status` field.
//
// This is the general readiness primitive used for auxiliary workloads (the
// per-gateway console and the embedded database). The gateway workload's own
// rollout is judged on the *new* revision instead, via ObserveGatewayRollout /
// deploymentRolloutComplete, so a still-Ready old pod cannot mask an unready new
// revision during a release roll. See gateway-release-rollout.spec.md.
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

// ObserveGatewayRollout reports the gateway Deployment's revision-aware readiness,
// whether a new revision is still rolling out, and the GatewayRelease the ready
// revision was actually rendered from (AppliedReleaseAnnotation). The health
// reconciler uses rollingOut to leave an in-progress rollout to the provisioning
// path (which owns the Provisioning -> Running/Degraded transition and preserves
// the last-good workload) rather than flapping the phase, and uses appliedRelease
// to advance observed_release_id only to the release actually on the workload --
// never to a desired release the provisioning path has not yet applied. It returns
// rollingOut=false with reason "deployment not found" when the Deployment does not
// yet exist. See gateway-release-rollout.spec.md.
func ObserveGatewayRollout(ctx context.Context, clientset kubernetes.Interface, namespace, name string) (ready bool, rollingOut bool, appliedRelease string, reason string, err error) {
	deploy, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, false, "", "deployment not found", nil
		}
		return false, false, "", "", fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}
	complete, rollingOut, reason := deploymentRolloutComplete(deploy)
	return complete, rollingOut, AppliedRelease(deploy), reason, nil
}

// gatewayReadyPollInterval is how often WaitForGatewayReady re-observes the
// gateway workload while waiting for readiness. It is a package variable so tests
// can shorten it; production keeps the 2s cadence.
var gatewayReadyPollInterval = 2 * time.Second

// WaitForGatewayReady blocks until the openshell-gateway Deployment reaches
// readiness or the timeout elapses. Readiness is judged on the new revision
// (see ObserveGatewayRollout), so a still-Ready old pod cannot let a defective
// release be reported ready during a roll. It returns ready=true on readiness,
// or ready=false with the last observed reason when the provisioning readiness
// window expires without the workload becoming ready.
func WaitForGatewayReady(ctx context.Context, clientset kubernetes.Interface, namespace string, timeout time.Duration) (bool, string) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(gatewayReadyPollInterval)
	defer ticker.Stop()

	lastReason := "not ready"
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err().Error()
		case <-deadline:
			return false, lastReason
		case <-ticker.C:
			ready, _, _, reason, err := ObserveGatewayRollout(ctx, clientset, namespace, GatewayDeploymentName)
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

	data := make(map[string]string, len(sourceCM.Data)+1)
	for k, v := range sourceCM.Data {
		data[k] = v
	}
	if _, ok := data["ca.crt"]; !ok {
		if v, ok := data["ca-bundle.crt"]; ok {
			data["ca.crt"] = v
		}
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
		Data: data,
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
		if err := kc.EnsureE2ETokenExchange(ctx, existingUUID); err != nil {
			return fmt.Errorf("reconcile e2e token-exchange on keycloak client %s: %w", kcClientID, err)
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

// routeTLSIssuer resolves the cert-manager ClusterIssuer that mints a per-host
// public certificate for a Route, from GATEWAY_ROUTE_TLS_ISSUER.
func routeTLSIssuer() string {
	return strings.TrimSpace(os.Getenv("GATEWAY_ROUTE_TLS_ISSUER"))
}

// readInjectedRouteCert returns the certificate and key that the cert-manager
// openshift-routes controller has injected into the named Route's spec.tls, or
// empty strings when the Route or those fields are absent.
func readInjectedRouteCert(ctx context.Context, dynamicClient dynamic.Interface, namespace, routeName string) (string, string) {
	gvr := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	existing, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, routeName, metav1.GetOptions{})
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

	listenerName := sharedGatewayListenerName()
	if os.Getenv("GATEWAY_API_HTTP_LISTENER_NAME") == "" {
		log.Printf("INFO using Gateway %s/%s listener %s for tenant %s (GATEWAY_API_HTTP_LISTENER_NAME unset; a NoMatchingParent GRPCRoute will not self-heal)", gwNS, gwName, listenerName, namespace)
	} else {
		log.Printf("INFO using Gateway %s/%s listener %s for tenant %s", gwNS, gwName, listenerName, namespace)
	}

	parentRef := map[string]interface{}{
		"name":        gwName,
		"namespace":   gwNS,
		"sectionName": listenerName,
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
