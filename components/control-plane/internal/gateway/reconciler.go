package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	// Step 1: EnvironmentReady
	report(ConditionEnvironmentReady, StatusInProgress, "")

	// Feature prerequisites are evaluated through the shared, refreshable
	// dependency discovery (CTRL-CORE-18). Each is checked for its own
	// group/version/resource, so a stale or unreachable unrelated API (for example
	// projectcalico.org/v3) never makes these look absent, and an unknown/
	// unavailable result is retryable rather than terminal (CTRL-CORE-19).
	deps := opts.Dependencies
	if deps == nil {
		deps = NewDependencyDiscovery(clientset.Discovery())
	}

	// The Gateway API is the only way a gateway is exposed (OSG-ROUT-02). Without
	// it the gateway would run unreachable, so the reconcile fails and retries. A
	// confirmed-absent result reports the resource to install; an unknown/
	// unavailable result is waiting/retryable with bounded backoff.
	if gwAPI := deps.Evaluate(DependencyGatewayAPI); gwAPI.State != DependencyPresent {
		report(ConditionEnvironmentReady, StatusFailed, gatewayAPIConditionMessage(gwAPI))
		return fmt.Errorf("gateway %s: %w", nsConfig.Name, gatewayAPIDependencyError(gwAPI))
	}

	// cert-manager is the sole TLS certificate strategy (OSG-TLS-01). Evaluate it
	// as a feature prerequisite: present enables the chart's cert-manager
	// resources; confirmed-absent reports the prerequisite to install; unknown/
	// unavailable is waiting/retryable. The controller and its other reconciliation
	// work keep running regardless (CTRL-CORE-17, CTRL-CORE-19).
	switch certManager := deps.Evaluate(DependencyCertManager); certManager.State {
	case DependencyPresent:
		opts.HasCertManager = true
	case DependencyAbsent:
		report(ConditionEnvironmentReady, StatusFailed, certManagerAbsentMessage)
		return fmt.Errorf("gateway %s: %w", nsConfig.Name, errCertManagerAbsent)
	default: // DependencyUnknown
		report(ConditionEnvironmentReady, StatusFailed, certManagerUnavailableMessage)
		return fmt.Errorf("gateway %s: cert-manager dependency unavailable: %w", nsConfig.Name, certManager.Err)
	}

	if err := EnsureManagedNamespace(ctx, clientset, nsConfig.Name, opts.ControlPlaneNamespace); err != nil {
		report(ConditionEnvironmentReady, StatusFailed, "Environment preparation failed - unable to set up the gateway namespace")
		return fmt.Errorf("ensure namespace %s: %w", nsConfig.Name, err)
	}

	if err := ValidateGatewayConfig(nsConfig.Gateway); err != nil {
		report(ConditionEnvironmentReady, StatusFailed, "Environment preparation failed - the gateway configuration is invalid")
		return fmt.Errorf("invalid gateway configuration: %w", err)
	}

	// A routed gateway is reachable at an external
	// hostname (gw-<namespace>.<base-domain>, or an explicit Route.Host). The
	// gateway pod terminates TLS with its own per-tenant CA, and the shared
	// Gateway re-encrypts to it and validates it with a BackendTLSPolicy, so the
	// server certificate must list the external
	// hostname as a SAN or clients fail verification. The controller derives
	// that hostname, so it -- not the operator -- injects it into the cert SANs
	// here, before cert-manager mints the certificate below.
	if nsConfig.Gateway.Route.Enabled {
		injectIngressHostnames(ctx, &nsConfig)
	}

	// Vault-backed gateways fail closed: verify the per-cluster Vault is reachable
	// (unauthenticated sys/health over TLS trusted by the serving CA) before we
	// provision the workload. The authenticated identity check runs post-deploy, once
	// the gateway ServiceAccount exists. See openshell-gateway-credentials.spec.md
	// (Fail Closed When Vault Is Unavailable).
	if isVaultDriver(effectiveCredentialDriver(nsConfig.Gateway.CredentialDriver)) {
		if err := probeVaultReachable(ctx, clientset); err != nil {
			report(ConditionEnvironmentReady, StatusFailed, "Vault credential backend is unreachable")
			return fmt.Errorf("vault reachability probe failed for %s: %w", nsConfig.Name, err)
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
		slog.ErrorContext(ctx, "Gateway database provisioning failed", "namespace", nsConfig.Name, "error", err)
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

	additionalCA, err := reconcileDestinationCABundle(ctx, clientset, opts.ControlPlaneNamespace, nsConfig.Name)
	if err != nil {
		report(ConditionGatewayDeployed, StatusFailed, "Destination TLS trust bundle is unavailable or invalid")
		return err
	}

	// Copy trusted CA bundle (for OIDC issuer verification)
	hasTrustedCA := reconcileTrustedCABundle(ctx, clientset, opts.ControlPlaneNamespace, nsConfig.Name)

	// For a Vault-backed gateway, mirror the per-cluster Vault serving CA into the
	// tenant namespace so the gateway can verify Vault's TLS (HYPERSHELL-369). The
	// source Vault and the gateway are on the same cluster, so this is a local copy,
	// mirroring reconcileTrustedCABundle.
	var hasVaultCA bool
	if isVaultDriver(effectiveCredentialDriver(nsConfig.Gateway.CredentialDriver)) {
		hasVaultCA = reconcileVaultCABundle(ctx, clientset, nsConfig.Name)
		if !hasVaultCA {
			// Fail closed: a Vault-backed gateway must be able to verify Vault's TLS.
			// If the serving CA could not be propagated (source Secret missing or
			// unreadable), do NOT deploy a gateway that would silently fall back to
			// platform trust roots and be unable to verify Vault. This matches
			// openshell-gateway-credentials.spec.md (Vault Serving-CA Propagation:
			// "the gateway SHALL NOT be deployed … fail-closed … naming the missing
			// CA source"). Retryable: a later reconcile deploys once the CA appears.
			caNS, caName := VaultCASource()
			report(ConditionGatewayDeployed, StatusFailed, "Vault credential backend selected but its serving CA is unavailable")
			return fmt.Errorf("vault serving CA not propagated for %s: source Secret %s/%s missing or unreadable", nsConfig.Name, caNS, caName)
		}
		// Create the gateway's OWN per-tenant Vault KV mount (openshell-gw/<namespace>)
		// before the workload starts, through the control plane's scoped provisioner
		// identity (it can manage only openshell-gw/* mounts, never read a secret or
		// rewrite a policy). Idempotent. Fail closed: a gateway must have its mount before
		// it runs. See openshell-gateway-credentials.spec.md (Provider Credential Isolation).
		if err := EnsureVaultTenantMount(ctx, clientset, opts, nsConfig.Name); err != nil {
			report(ConditionGatewayDeployed, StatusFailed, "Vault credential backend mount could not be provisioned")
			return fmt.Errorf("ensure vault tenant mount for %s: %w", nsConfig.Name, err)
		}
	}

	// Reconcile OpenShift SCC binding BEFORE Helm install
	// (sandbox pods need privileged SCC to schedule)
	if opts.IsOpenShift {
		if err := reconcileOpenShiftSCC(ctx, dynamicClient, nsConfig.Name); err != nil {
			slog.WarnContext(ctx, "Failed to reconcile OpenShift SCC binding", "namespace", nsConfig.Name, "error", err)
		}
	}

	// Deploy gateway via Helm
	// The chart handles: Deployment, Services, RBAC, cert-manager and the
	// credential KEK
	if err := deployGatewayViaHelm(ctx, helmClient, nsConfig, opts, hasTrustedCA, hasVaultCA, additionalCA); err != nil {
		report(ConditionGatewayDeployed, StatusFailed, "Gateway deployment failed - unable to deploy the gateway workload")
		return fmt.Errorf("deploy gateway via helm in %s: %w", nsConfig.Name, err)
	}

	// Vault-backed gateways: the workload (and its ServiceAccount) now exists, so
	// verify the gateway's own identity can authenticate to the per-cluster Vault
	// before it advances to Running. Fail closed + retry on denial. See
	// openshell-gateway-credentials.spec.md (Fail Closed When Vault Is Unavailable).
	if isVaultDriver(effectiveCredentialDriver(nsConfig.Gateway.CredentialDriver)) {
		if err := probeVaultAuth(ctx, clientset, nsConfig.Name); err != nil {
			report(ConditionGatewayDeployed, StatusFailed, "Vault credential backend rejected the gateway identity")
			return fmt.Errorf("vault auth probe failed for %s: %w", nsConfig.Name, err)
		}
	}

	// Console reconciliation and route-address publishing are not managed by
	// the Helm chart. Reconcile them after the Helm release so the gateway
	// workload is already deployed.
	if nsConfig.Gateway.Route.Enabled {
		if err := reconcileGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig, opts); err != nil {
			report(ConditionGatewayDeployed, StatusFailed, gatewayDeployedConditionMessage(nsConfig, err))
			return fmt.Errorf("reconcile Gateway API resources in %s: %w", nsConfig.Name, err)
		}
	} else {
		if err := DeleteGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig.Name, opts); err != nil {
			slog.WarnContext(ctx, "Failed to remove Gateway API resources", "namespace", nsConfig.Name, "error", err)
		}
	}

	report(ConditionGatewayDeployed, StatusComplete, "")

	slog.InfoContext(ctx, "Gateway reconciled", "namespace", nsConfig.Name)
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
			slog.WarnContext(ctx, "Failed to uninstall helm release", "namespace", namespace, "error", err)
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
			slog.WarnContext(ctx, "Failed to delete ClusterRoleBinding", "name", crbName, "error", err)
			// This cluster-scoped binding has no owning namespace to cascade-reap
			// it and no reconciler that reclaims leaked bindings, so a failure here
			// is a silent orphan unless it is recorded durably.
			recordOrphan(ctx, opts, "ClusterRoleBinding", crbName,
				fmt.Sprintf("delete failed during gateway deletion: %v", err))
		}
	} else {
		slog.InfoContext(ctx, "Deleted ClusterRoleBinding", "name", crbName)
	}

	if opts.KeycloakClient != nil && opts.GatewayID != "" {
		kcClientID := opts.GatewayClientID
		if kcClientID == "" {
			return fmt.Errorf("gateway identity is required for cleanup")
		}
		if err := opts.KeycloakClient.DeleteGatewayServiceAccountClients(ctx, opts.GatewayID); err != nil {
			// Do not delete the parent clients while an OpenShell gateway service
			// account may still be enabled. Returning an error makes teardown retry.
			return fmt.Errorf("delete gateway service-account clients: %w", err)
		}
		slog.InfoContext(ctx, "Deleted keycloak service-account clients", "gateway_id", opts.GatewayID)

		// The console namespaced resources are swept by label above, but the
		// console Keycloak client must be deleted explicitly (it lives in the
		// realm, not the namespace). Best-effort: log the orphan on failure.
		consoleClientID := kcClientID + "-console"
		if err := opts.KeycloakClient.DeleteConsoleClient(ctx, consoleClientID); err != nil {
			slog.WarnContext(ctx, "Failed to delete console client (orphaned)", "client_id", consoleClientID, "error", err)
			recordOrphan(ctx, opts, "KeycloakClient", consoleClientID,
				fmt.Sprintf("delete failed during gateway deletion: %v", err))
		} else {
			slog.InfoContext(ctx, "Deleted console client", "client_id", consoleClientID)
		}

		if err := opts.KeycloakClient.DeleteGatewayClient(ctx, kcClientID); err != nil {
			slog.WarnContext(ctx, "Failed to delete keycloak client (orphaned)", "client_id", kcClientID, "error", err)
			recordOrphan(ctx, opts, "KeycloakClient", kcClientID,
				fmt.Sprintf("delete failed during gateway deletion: %v", err))
		} else {
			slog.InfoContext(ctx, "Deleted keycloak client", "client_id", kcClientID)
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
			slog.InfoContext(ctx, "Cleaned up credential RBAC", "namespace", credNS)
		}
	}

	slog.InfoContext(ctx, "Gateway out-of-namespace resources cleaned up", "namespace", namespace)
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
// deleted here. The caller invokes it only when the namespace was left in place.
//
// The sweep attempts every resource type regardless of an individual failure and
// aggregates the failures into the returned error rather than swallowing them, so
// a partial sweep is retried and never silently leaks an owned resource
// (GTW-DEL-03). A resource type whose CRD is not installed (list returns
// NotFound) is skipped without failing the sweep (GTW-DEL-02.01), and a target
// already absent at delete time is treated as success.
func DeleteLabeledNamespaceResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	namespace string,
	opts ReconcileOpts,
) error {
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
	// Always enumerate cert-manager resources for cleanup: a missing CRD or API
	// group is skipped per-GVR below (IsNotFound), so this stays correct on
	// clusters without cert-manager and no longer depends on a frozen startup
	// capability flag (CTRL-CORE-17, CTRL-CORE-18).
	namespacedResources = append(namespacedResources,
		schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"},
		schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	)
	namespacedResources = append(namespacedResources,
		schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"},
		schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"},
		schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "backendtlspolicies"},
	)

	var errs []error
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
			slog.WarnContext(ctx, "Failed to list resources for cleanup", "kind", gvr.Resource, "namespace", namespace, "error", err)
			errs = append(errs, fmt.Errorf("list %s in %s: %w", gvr.Resource, namespace, err))
			continue
		}
		for i := range list.Items {
			name := list.Items[i].GetName()
			if err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
				if !k8serrors.IsNotFound(err) {
					slog.WarnContext(ctx, "Failed to delete resource",
						"kind", gvr.Resource,
						"name", name,
						"namespace", namespace,
						"error", err,
					)
					errs = append(errs, fmt.Errorf("delete %s %s in %s: %w", gvr.Resource, name, namespace, err))
				}
				continue
			}
			slog.InfoContext(ctx, "Deleted resource", "kind", gvr.Resource, "name", name, "namespace", namespace)
		}
	}
	return errors.Join(errs...)
}

// Sentinel errors for the dedicated-certificate provisioning path. They let
// gatewayDeployedConditionMessage surface the actionable cause in the
// user-visible ConditionGatewayDeployed message (API UX review finding 2)
// instead of the generic network-routing text, which a UI user cannot tell
// apart from an actual network failure.
var (
	// errExternalIssuerUnset: the controller deployment is missing the issuer
	// configuration; only the platform operator can fix it.
	errExternalIssuerUnset = errors.New("dedicated certificate provisioning is unavailable: GATEWAY_EXTERNAL_ISSUER is not set on the controller (contact the platform operator)")
	// errExternalCertificateNotIssued: cert-manager did not produce the
	// external certificate Secret within the provisioning wait window.
	errExternalCertificateNotIssued = errors.New("the dedicated gateway certificate was not issued in time; check the openshell-gateway-external Certificate's status conditions and the issuer named by GATEWAY_EXTERNAL_ISSUER")
)

// gatewayDeployedConditionMessage translates a Gateway API resource
// reconciliation failure into the user-visible ConditionGatewayDeployed
// message. The generic message covers the shared-Gateway path; the
// dedicated-certificate sentinels carry their actionable causes.
func gatewayDeployedConditionMessage(nsConfig NamespaceConfig, err error) string {
	switch {
	case errors.Is(err, errExternalIssuerUnset):
		return "Gateway deployment failed - " + errExternalIssuerUnset.Error()
	case errors.Is(err, errExternalCertificateNotIssued):
		return "Gateway deployment failed - " + errExternalCertificateNotIssued.Error()
	default:
		return "Gateway deployment failed - unable to configure network routing"
	}
}

// DeleteGatewayAPIResources reconciles the desired *absence* of a gateway's
// route: it removes the GRPCRoute, BackendTLSPolicy, backend-CA ConfigMap, the
// dedicated-certificate resources (external Certificate, per-tenant Gateway,
// sandbox HTTPRoute) when present, tears down the console (which follows the
// route), and clears the stored route_address. It attempts every deletion
// regardless of individual failures and returns their joined errors (nil once
// everything is absent), so a caller can retry until the route and its console
// are fully gone rather than stopping on partial cleanup. Idempotent: absent
// resources are ignored.
func DeleteGatewayAPIResources(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, namespace string, opts ReconcileOpts) error {
	var errs []error

	if err := deleteGatewayAPIRouteResources(ctx, dynamicClient, clientset, namespace); err != nil {
		errs = append(errs, err)
	}

	// The console follows the route, so removing the route removes the console.
	if err := deleteConsole(ctx, dynamicClient, clientset, namespace, opts); err != nil {
		errs = append(errs, err)
	}

	if opts.UpdateRouteAddress != nil {
		if err := opts.UpdateRouteAddress(ctx, ""); err != nil {
			errs = append(errs, fmt.Errorf("clear routeAddress in %s: %w", namespace, err))
		} else {
			slog.InfoContext(ctx, "Cleared routeAddress for gateway", "namespace", namespace)
		}
	}

	if len(errs) == 0 {
		slog.InfoContext(ctx, "Gateway API resources removed", "namespace", namespace)
	}
	return errors.Join(errs...)
}

// deleteGatewayAPIRouteResources removes the route-owned Kubernetes resources:
// the GRPCRoute, BackendTLSPolicy, backend-CA ConfigMap, and the
// dedicated-certificate resources (external Certificate, per-tenant Gateway,
// sandbox HTTPRoute). It takes a kubernetes.Interface rather than the concrete
// client type so tests can drive it with a fake clientset (the console teardown
// in DeleteGatewayAPIResources requires the concrete type and stays in the
// wrapper). Idempotent: absent resources are ignored, and all errors are joined.
func deleteGatewayAPIRouteResources(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, namespace string) error {
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

	// Dedicated-certificate resources (only present on opted-in gateways; their
	// absence for non-opted-in gateways is the expected state, so the deletes are
	// unconditionally attempted and NotFound is ignored). A removed gateway must
	// not leave a live listener serving its hostname or wildcard, nor a
	// certificate renewing for it.
	certGVR := schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
	if err := dynamicClient.Resource(certGVR).Namespace(namespace).Delete(ctx, externalCertificateName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete external Certificate in %s: %w", namespace, err))
	}

	gatewayGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}
	if err := dynamicClient.Resource(gatewayGVR).Namespace(namespace).Delete(ctx, perTenantGatewayName(namespace), metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete per-tenant Gateway in %s: %w", namespace, err))
	}

	httpRouteGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
	if err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Delete(ctx, sandboxHTTPRouteName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete sandbox HTTPRoute in %s: %w", namespace, err))
	}

	if err := clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, backendCAConfigMapName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete backend CA ConfigMap in %s: %w", namespace, err))
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

// RouteResourcesAbsent reports whether the gateway's Gateway API resources and
// all console resources are absent. It also checks the external Keycloak
// console client. Unknown state
// must never be read as absence.
//
// It returns (true, nil) only when every probed resource is confirmed absent.
// The first resource found present short-circuits to (false, nil). Any probe
// that fails for a reason other than Kubernetes NotFound is returned as an error
// so the caller treats absence as unconfirmed (and re-runs teardown) rather than
// trusting an unknown state. The Keycloak client probe is skipped when
// consoleClient is nil or consoleClientID is empty (no Keycloak configured).
func RouteResourcesAbsent(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, namespace string, consoleClient ConsoleClientChecker, consoleClientID string) (bool, error) {
	grpcRouteGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}
	btlsGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "backendtlspolicies"}
	httpRouteGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
	gatewayGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	certGVR := schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}

	type dynamicProbe struct {
		gvr  schema.GroupVersionResource
		name string
	}
	// The dedicated-certificate resources are only present for opted-in
	// gateways; probing them unconditionally is correct (absence for
	// non-opted-in gateways is the expected state).
	dynamicProbes := []dynamicProbe{
		{grpcRouteGVR, "openshell-gateway"},
		{btlsGVR, "openshell-gateway"},
		{certGVR, externalCertificateName},
		{gatewayGVR, perTenantGatewayName(namespace)},
		{httpRouteGVR, sandboxHTTPRouteName},
		{httpRouteGVR, consoleName},
		{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, consoleName},
	}
	for _, p := range dynamicProbes {
		if _, err := dynamicClient.Resource(p.gvr).Namespace(namespace).Get(ctx, p.name, metav1.GetOptions{}); err == nil {
			return false, nil
		} else if !k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("probe %s/%s in %s: %w", p.gvr.Resource, p.name, namespace, err)
		}
	}

	if _, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, backendCAConfigMapName, metav1.GetOptions{}); err == nil {
		return false, nil
	} else if !k8serrors.IsNotFound(err) {
		return false, fmt.Errorf("probe configmap %s in %s: %w", backendCAConfigMapName, namespace, err)
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
// gateway server certificate). It distinguishes a genuinely absent CA from a
// failed read (OSG-ROUT-14): a NotFound Secret, or a Secret present without a
// ca.crt key, is a genuine "no CA" state and returns ("", nil) so callers may
// proceed without the backend-CA material. Any other read error (for example a
// transient API failure) is returned so the caller retries instead of reading an
// unconfirmed read as "no CA" and disabling backend TLS verification.
func readServerTLSCA(ctx context.Context, clientset kubernetes.Interface, namespace string) (string, error) {
	tlsSecret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, "openshell-server-tls", metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("read openshell-server-tls secret in %s: %w", namespace, err)
	}
	if ca, ok := tlsSecret.Data["ca.crt"]; ok {
		return string(ca), nil
	}
	return "", nil
}

// reconcileBackendTLS reconciles the backend-CA ConfigMap and the dependent
// BackendTLSPolicy as an ordered, fail-closed pair (OSG-ROUT-14, OSG-ROUT-15,
// OSG-ROUT-16).
//
//   - The CA read distinguishes a genuinely absent CA from a failed read: a read
//     failure is surfaced so the pass retries, and a genuinely absent CA returns
//     without creating backend-TLS material.
//   - The backend-CA ConfigMap write must succeed before the dependent
//     BackendTLSPolicy is created; a ConfigMap write failure is surfaced and stops
//     before the policy, so the policy never references a missing or stale CA.
//   - A BackendTLSPolicy error is tolerated only when the CRD is not installed
//     (the Gateway API transport policy is unavailable, which surfaces as a
//     NotFound from the dynamic client); every other error (RBAC, conflict, API
//     outage) is surfaced so the pass retries.
func reconcileBackendTLS(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, namespace string) error {
	caData, err := readServerTLSCA(ctx, clientset, namespace)
	if err != nil {
		return err
	}
	if caData == "" {
		return nil
	}

	backendCA := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backendCAConfigMapName,
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

	existing, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, backendCAConfigMapName, metav1.GetOptions{})
	switch {
	case err == nil:
		backendCA.ResourceVersion = existing.ResourceVersion
		if _, err := clientset.CoreV1().ConfigMaps(namespace).Update(ctx, backendCA, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update backend CA ConfigMap in %s: %w", namespace, err)
		}
	case k8serrors.IsNotFound(err):
		if _, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, backendCA, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create backend CA ConfigMap in %s: %w", namespace, err)
		}
	default:
		return fmt.Errorf("read backend CA ConfigMap in %s: %w", namespace, err)
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
							"name":  backendCAConfigMapName,
						},
					},
					"hostname": fmt.Sprintf("openshell-gateway.%s.svc.cluster.local", namespace),
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, btlsPolicy); err != nil {
		// Tolerate ONLY the "CRD not installed" case. reconcileResource creates the
		// policy when it is absent, so it only returns a NotFound when the
		// BackendTLSPolicy resource type itself is unknown to the API server (the
		// Gateway API transport policy is unavailable, e.g. before OpenShift 4.22).
		// Every other error (RBAC denial, conflict, API outage) must surface so the
		// pass retries rather than disabling backend TLS verification silently.
		if k8serrors.IsNotFound(err) {
			slog.WarnContext(ctx, "BackendTLSPolicy CRD not installed; skipping backend TLS policy (requires Gateway API transport policy support, e.g. OpenShift 4.22+)", "namespace", namespace)
			return nil
		}
		return fmt.Errorf("reconcile BackendTLSPolicy in %s: %w", namespace, err)
	}
	return nil
}

func waitForSecret(ctx context.Context, clientset kubernetes.Interface, namespace, name string, timeout time.Duration) error {
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
			slog.InfoContext(ctx, "Secret is available", "namespace", namespace, "name", name)
			return nil
		}

		if watchCtx.Err() != nil {
			return fmt.Errorf("timed out waiting for secret %s/%s: %w", namespace, name, watchCtx.Err())
		}
		slog.InfoContext(ctx, "Watch for secret closed early; re-establishing", "namespace", namespace, "name", name)
	}
}

// GatewayDeploymentName is the name of the primary gateway workload Deployment
// whose readiness gates the Gateway `Running` phase.
const GatewayDeploymentName = "openshell-gateway"

// backendCAConfigMapName is the ConfigMap holding the CA certificate the
// Gateway's BackendTLSPolicy validates the backend pod certificate against. The
// create, delete, and teardown-readiness paths MUST agree on this name or the
// ConfigMap leaks on teardown and the readiness probe watches the wrong object;
// see openshell-gateway.spec.md.
const backendCAConfigMapName = "openshell-gateway-backend-ca"

// Names of the resources only a dedicated-certificate gateway
// (route.certificate) owns. The create, delete, and teardown-readiness paths
// MUST agree on them or the resources leak on teardown and the absence probe
// watches the wrong objects, exactly like backendCAConfigMapName above.
const (
	// externalCertificateName is the cert-manager Certificate covering both
	// the gateway's own hostname and its sandbox wildcard.
	externalCertificateName = "openshell-gateway-external"
	// externalCertificateSecretName is the Secret cert-manager issues
	// externalCertificateName into; the per-tenant Gateway's listeners
	// reference it.
	externalCertificateSecretName = "openshell-gateway-external-tls"
	// sandboxHTTPRouteName is the wildcard-hostname HTTPRoute routing sandbox
	// service traffic to the gateway's own listener.
	sandboxHTTPRouteName = "openshell-gateway-sandbox"
)

// gateway Service backend ports the tenant routes forward to. These MUST agree
// with the openshell Helm chart's Service ports (charts/openshell/values.yaml
// service.port / service.httpPort and templates/service.yaml); the route and
// the Service are reconciled by different surfaces and are coupled only by these
// numbers. Both Service ports target the same gateway pod container port; see
// openshell-gateway-routing.spec.md OSG-ROUT-13 (Dual-Protocol Backend).
const (
	// gatewayServiceGRPCPort is the Service port declaring appProtocol: grpc.
	// The GRPCRoute forwards here; the ingress proxy dials it over HTTP/2.
	gatewayServiceGRPCPort = int64(8080)
	// gatewayServiceHTTPPort is the Service port declaring appProtocol: http.
	// The sandbox HTTPRoute forwards here so the ingress proxy dials the backend
	// over HTTP/1.1, which a WebSocket upgrade requires (HTTP/2 cannot carry it).
	gatewayServiceHTTPPort = int64(8090)
)

// perTenantGatewayName returns the name of the per-tenant Gateway API Gateway
// a dedicated-certificate gateway is served by (gw-<namespace>). It mirrors the
// hostname derivation (gw-<namespace>.<base-domain>) so the Gateway, its
// listeners, and the hostname baked into the routes cannot drift apart. It
// matches the exposure adapter's same-named helper.
func perTenantGatewayName(namespace string) string {
	return "gw-" + namespace
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
// openshell-gateway-health.spec.md.
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
// revision during a rollout. See openshell-gateway-health.spec.md.
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

// ObserveGatewayRollout reports the gateway Deployment's revision-aware readiness
// and whether a new revision is still rolling out. The health reconciler uses
// rollingOut to leave an in-progress rollout to the provisioning path (which owns
// the Provisioning -> Running/Degraded transition and preserves the last-good
// workload) rather than flapping the phase. It returns rollingOut=false with
// reason "deployment not found" when the Deployment does not yet exist. See
// openshell-gateway-health.spec.md.
func ObserveGatewayRollout(ctx context.Context, clientset kubernetes.Interface, namespace, name string) (ready bool, rollingOut bool, reason string, err error) {
	deploy, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, false, "deployment not found", nil
		}
		return false, false, "", fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}
	complete, rollingOut, reason := deploymentRolloutComplete(deploy)
	return complete, rollingOut, reason, nil
}

// gatewayReadyPollInterval is how often WaitForGatewayReady re-observes the
// gateway workload while waiting for readiness. It is a package variable so tests
// can shorten it; production keeps the 2s cadence.
var gatewayReadyPollInterval = 2 * time.Second

// WaitForGatewayReady blocks until the openshell-gateway Deployment reaches
// readiness or the timeout elapses. Readiness is judged on the new revision
// (see ObserveGatewayRollout), so a still-Ready old pod cannot let a defective
// revision be reported ready during a rollout. It returns ready=true on
// readiness, or ready=false with the last observed reason when the provisioning
// readiness window expires without the workload becoming ready.
// See openshell-gateway-health.spec.md.
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
			ready, _, reason, err := ObserveGatewayRollout(ctx, clientset, namespace, GatewayDeploymentName)
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
			slog.InfoContext(ctx, "Created resource", "kind", gvk.Kind, "name", name, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("get %s %s: %w", gvk.Kind, name, err)
	}

	if gvk.Kind == "Job" {
		slog.DebugContext(ctx, "Job already exists, skipping update", "name", name)
		return nil
	}

	if gvk.Kind == "PersistentVolumeClaim" {
		slog.DebugContext(ctx, "PVC already exists, skipping update", "name", name)
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
	}

	if resource, ok := mapping[kind]; ok {
		return resource
	}

	slog.Debug("Unknown kind, using naive plural", "kind", kind)
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
		slog.InfoContext(ctx, "Created privileged SCC binding for openshell-gateway-sandbox", "namespace", namespace)
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
				slog.WarnContext(ctx, "Failed to create trusted CA ConfigMap", "namespace", targetNamespace, "error", err)
				return false
			}
			slog.InfoContext(ctx, "Copied trusted CA ConfigMap", "namespace", targetNamespace)
			return true
		}
		slog.WarnContext(ctx, "Failed to get trusted CA ConfigMap", "namespace", targetNamespace, "error", err)
		return false
	}

	targetCM.ResourceVersion = existing.ResourceVersion
	if _, err := clientset.CoreV1().ConfigMaps(targetNamespace).Update(ctx, targetCM, metav1.UpdateOptions{}); err != nil {
		slog.WarnContext(ctx, "Failed to update trusted CA ConfigMap", "namespace", targetNamespace, "error", err)
		return false
	}
	return true
}

// reconcileVaultCABundle mirrors the per-cluster Vault serving CA into the gateway's
// tenant namespace as the openshell-gateway-vault-ca ConfigMap (key ca.crt), so a
// Vault-backed gateway can verify Vault's TLS (HYPERSHELL-369). The source Secret
// (openshell-gateway-vault-tls in openshell-vault, overridable via
// GATEWAY_VAULT_CA_SECRET) lives on the same cluster as the gateway, so this is a
// local copy, mirroring reconcileTrustedCABundle. It returns true when the target
// ConfigMap is present after reconciliation, so the caller wires the chart's
// caConfigMapName only when the gateway can actually mount it. The ConfigMap carries
// the management label so tenant-namespace cleanup reclaims it on deletion.
func reconcileVaultCABundle(ctx context.Context, clientset *kubernetes.Clientset, targetNamespace string) bool {
	srcNamespace, srcName := VaultCASource()

	secret, err := clientset.CoreV1().Secrets(srcNamespace).Get(ctx, srcName, metav1.GetOptions{})
	if err != nil {
		slog.WarnContext(ctx, "Vault serving CA source Secret not readable; gateway falls back to platform trust roots", "namespace", srcNamespace, "secret", srcName, "error", err)
		return false
	}
	caPEM, ok := secret.Data[VaultCAKey]
	if !ok || len(caPEM) == 0 {
		slog.WarnContext(ctx, "Vault serving CA source Secret has no CA key", "namespace", srcNamespace, "secret", srcName, "key", VaultCAKey)
		return false
	}

	targetCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      VaultCATargetConfigMapName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		Data: map[string]string{VaultCAKey: string(caPEM)},
	}

	existing, err := clientset.CoreV1().ConfigMaps(targetNamespace).Get(ctx, VaultCATargetConfigMapName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := clientset.CoreV1().ConfigMaps(targetNamespace).Create(ctx, targetCM, metav1.CreateOptions{}); err != nil {
				slog.WarnContext(ctx, "Failed to create vault CA ConfigMap", "namespace", targetNamespace, "error", err)
				return false
			}
			slog.InfoContext(ctx, "Copied vault serving CA ConfigMap", "namespace", targetNamespace)
			return true
		}
		slog.WarnContext(ctx, "Failed to get vault CA ConfigMap", "namespace", targetNamespace, "error", err)
		return false
	}

	targetCM.ResourceVersion = existing.ResourceVersion
	if _, err := clientset.CoreV1().ConfigMaps(targetNamespace).Update(ctx, targetCM, metav1.UpdateOptions{}); err != nil {
		slog.WarnContext(ctx, "Failed to update vault CA ConfigMap", "namespace", targetNamespace, "error", err)
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

	if opts.GatewayID == "" {
		return fmt.Errorf("gateway ID is required for keycloak provisioning")
	}
	kcClientID := opts.GatewayClientID
	if kcClientID == "" {
		return fmt.Errorf("gateway client ID is required for keycloak provisioning")
	}

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
		slog.InfoContext(ctx, "Reconciled keycloak client", "client_id", kcClientID, "uuid", existingUUID)
	} else {
		clientUUID, err := kc.ProvisionGatewayClient(ctx, kcClientID)
		if err != nil {
			return fmt.Errorf("provision keycloak client %s: %w", kcClientID, err)
		}
		slog.InfoContext(ctx, "Provisioned keycloak client", "client_id", kcClientID, "uuid", clientUUID)
	}

	// The controller persists this configuration before the pass captures its
	// desired snapshot (see DesiredOIDCConfig), so the gateway deploys the same
	// value the API server stores for the generation being applied.
	nsConfig.Gateway.OIDC = DesiredOIDCConfig(opts.Keycloak, kcClientID)

	return nil
}

// DesiredOIDCConfig returns the OIDC configuration the control plane generates
// for a gateway whose Keycloak client ID is clientID. It depends only on the
// Keycloak configuration and the client ID, not on the Keycloak client existing,
// so the controller can resolve and persist it before it applies any workload
// configuration.
func DesiredOIDCConfig(kcConfig *KeycloakConfig, clientID string) OIDCConfig {
	oidcConfig := OIDCConfig{
		ClientID:   clientID,
		Audience:   clientID,
		JwksTTL:    3600,
		RolesClaim: "hypershell.roles",
		AdminRole:  "openshell-admin",
		UserRole:   "openshell-user",
	}
	if kcConfig != nil {
		oidcConfig.Issuer = keycloak.NewClient(kcConfig.ServerURL, kcConfig.Realm, "", "").Issuer()
	}
	// The Keycloak Admin API server URL must be reachable in-cluster, but the
	// gateway's client-facing issuer (consumed by the gateway pod, console, and
	// CLI) may need to be a separately reachable URL. When GATEWAY_OIDC_ISSUER_URL
	// is set it overrides the admin-derived issuer; it MUST equal Keycloak's
	// KC_HOSTNAME so the token `iss` claim validates. Unset preserves 98's default.
	if issuerURL := os.Getenv("GATEWAY_OIDC_ISSUER_URL"); issuerURL != "" {
		oidcConfig.Issuer = issuerURL
	}
	return oidcConfig
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
		slog.WarnContext(ctx, "Failed to delete credential secrets RoleBinding", "namespace", namespace, "error", err)
	}
	if err := dynamicClient.Resource(roleGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		slog.WarnContext(ctx, "Failed to delete credential secrets Role", "namespace", namespace, "error", err)
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

	slog.InfoContext(ctx, "Reconciled credential secrets RBAC", "credential_namespace", credentialNamespace, "namespace", gatewayNamespace)
	return nil
}

// errGatewayAPIMissing reports a cluster where discovery confirms the Gateway API
// is absent. Gateways are exposed only through the Gateway API, so a gateway
// cannot be provisioned there until the operator installs it (OSG-ROUT-02.02).
var errGatewayAPIMissing = errors.New("the Gateway API is not installed on this cluster")

// errGatewayAPIUnavailable reports that Gateway API discovery could not be
// completed (transport, authorization, or stale-group error). The state is
// unknown/unavailable, so the affected work is waiting/retryable rather than
// treated as absent (OSG-ROUT-02.03).
var errGatewayAPIUnavailable = errors.New("the Gateway API could not be discovered")

// gatewayAPIMissingMessage is the condition message for a confirmed-absent
// Gateway API: a prerequisite to install.
const gatewayAPIMissingMessage = "Gateway API is not installed on this cluster; the gateway cannot be exposed"

// gatewayAPIUnavailableMessage is the condition message for an unknown/
// unavailable Gateway API: waiting and retryable.
const gatewayAPIUnavailableMessage = "Gateway API is temporarily unavailable - waiting to reconcile the gateway's ingress"

// cert-manager prerequisite sentinels and condition messages (OSG-TLS-01).
var errCertManagerAbsent = errors.New("cert-manager is not installed on this cluster")

const (
	// certManagerAbsentMessage is the condition message for a confirmed-absent
	// cert-manager: a prerequisite to install (OSG-TLS-01.02).
	certManagerAbsentMessage = "cert-manager is not installed on this cluster; install cert-manager (cert-manager.io/v1 Certificate) before the gateway can be provisioned"
	// certManagerUnavailableMessage is the condition message for an unknown/
	// unavailable cert-manager: waiting and retryable (OSG-TLS-01.03).
	certManagerUnavailableMessage = "cert-manager is temporarily unavailable - waiting to provision the gateway's certificates"
)

// gatewayAPIDependencyError maps a Gateway API dependency result to the sentinel
// error carried out of ReconcileGateway, so callers can distinguish a
// confirmed-absent prerequisite from an unknown/unavailable (retryable) state.
func gatewayAPIDependencyError(res DependencyResult) error {
	if res.State == DependencyAbsent {
		return errGatewayAPIMissing
	}
	if res.Err != nil {
		return fmt.Errorf("%w: %w", errGatewayAPIUnavailable, res.Err)
	}
	return errGatewayAPIUnavailable
}

// gatewayAPIConditionMessage is the EnvironmentReady message for a non-present
// Gateway API dependency, distinguishing confirmed-absent (a prerequisite to
// install) from unknown/unavailable (waiting/retryable), per OSG-HLTH-07.
func gatewayAPIConditionMessage(res DependencyResult) string {
	if res.State == DependencyAbsent {
		return gatewayAPIMissingMessage
	}
	return gatewayAPIUnavailableMessage
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

// gatewayExternalIssuerName returns the name of the cert-manager issuer that
// mints the dedicated per-gateway external certificate, from
// GATEWAY_EXTERNAL_ISSUER. Empty means the dedicated-certificate feature is
// unavailable on this controller: an opted-in gateway fails provisioning with
// a clear error instead of silently falling back to the shared wildcard.
func gatewayExternalIssuerName() string {
	return strings.TrimSpace(os.Getenv("GATEWAY_EXTERNAL_ISSUER"))
}

// gatewayExternalIssuerKind returns the kind of the external certificate
// issuer (GATEWAY_EXTERNAL_ISSUER_KIND), defaulting to ClusterIssuer.
func gatewayExternalIssuerKind() string {
	if kind := strings.TrimSpace(os.Getenv("GATEWAY_EXTERNAL_ISSUER_KIND")); kind != "" {
		return kind
	}
	return "ClusterIssuer"
}

// gatewayAPIGatewayClass returns the GatewayClass the per-tenant Gateway is
// created with (GATEWAY_API_GATEWAY_CLASS), defaulting to openshift-default.
func gatewayAPIGatewayClass() string {
	if class := strings.TrimSpace(os.Getenv("GATEWAY_API_GATEWAY_CLASS")); class != "" {
		return class
	}
	return "openshift-default"
}

// deriveGatewayHostname resolves the external hostname for a tenant gateway.
// An explicit Route.Host wins; otherwise it is
// derived as gw-<namespace>.<GATEWAY_API_BASE_DOMAIN>. The gateway's server
// certificate SANs (ServerDnsNames/ExternalDns) must cover this hostname.
func deriveGatewayHostname(nsConfig NamespaceConfig) (string, error) {
	baseDomain := os.Getenv("GATEWAY_API_BASE_DOMAIN")
	if h := nsConfig.Gateway.Route.Host; h != "" {
		// An explicit host that falls under the operator's shared base domain
		// MUST be this tenant's own slot (gw-<namespace>.<base-domain>).
		// Otherwise a tenant could set Route.Host to another tenant's derived
		// host and hijack its route under the shared wildcard. Hosts outside the base domain
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

// injectIngressHostnames adds the external ingress hostname to the gateway's
// server certificate SANs and fills an empty Route.Host with it. The gateway
// pod terminates TLS with its own per-tenant CA and the shared Gateway
// re-encrypts to it, so the server certificate must list the
// external hostname as a SAN or clients fail verification. The controller
// derives the hostname, so it -- not the operator -- injects it into the cert
// SANs here, before cert-manager mints the certificate.
//
// A dedicated-certificate gateway (route.certificate) additionally gets the
// wildcard *.gw-<namespace>.<base-domain> SAN: the gateway strips the "*."
// prefix and derives its exposed-service base domain from it, so without the
// wildcard entry it cannot generate valid sandbox service URLs. See
// docs/investigations/per-gateway-certificates.md (Gap C).
func injectIngressHostnames(ctx context.Context, nsConfig *NamespaceConfig) {
	hostname, err := deriveGatewayHostname(*nsConfig)
	if err != nil {
		slog.WarnContext(ctx, "Cannot add ingress hostname to gateway certificate SANs", "namespace", nsConfig.Name, "error", err)
		return
	}
	nsConfig.Gateway.ServerDnsNames = appendDNSNameIfMissing(nsConfig.Gateway.ServerDnsNames, hostname)
	if nsConfig.Gateway.Route.Certificate != nil {
		nsConfig.Gateway.ServerDnsNames = appendDNSNameIfMissing(nsConfig.Gateway.ServerDnsNames, "*."+hostname)
	}
	if nsConfig.Gateway.Route.Host == "" {
		nsConfig.Gateway.Route.Host = hostname
	}
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

// buildExternalCertificate returns the cert-manager Certificate covering a
// dedicated-certificate gateway's own hostname and its sandbox wildcard. One
// certificate with both names is a single ACME order; the wildcard mandates a
// DNS-01-capable issuer (named by GATEWAY_EXTERNAL_ISSUER on the controller).
func buildExternalCertificate(namespace, hostname string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata": map[string]interface{}{
			"name":      externalCertificateName,
			"namespace": namespace,
			"labels": map[string]interface{}{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		"spec": map[string]interface{}{
			"secretName": externalCertificateSecretName,
			"dnsNames":   []interface{}{hostname, "*." + hostname},
			// Always replace the Secret on renewal so the Gateway listeners
			// serving it never present a stale certificate.
			"rotationPolicy": "Always",
			"issuerRef": map[string]interface{}{
				"name":  gatewayExternalIssuerName(),
				"kind":  gatewayExternalIssuerKind(),
				"group": "cert-manager.io",
			},
		},
	}}
}

// buildPerTenantGateway returns the per-tenant Gateway API Gateway a
// dedicated-certificate gateway is served by. Both listeners terminate TLS with
// the same external certificate Secret on port 443 with distinct,
// non-overlapping hostnames: "grpc" carries the gateway's own gRPC traffic on
// the exact hostname, "sandbox" carries sandbox service and console traffic on
// the wildcard. The external-dns infrastructure annotation publishes both DNS
// records pointing at this Gateway's own load balancer (the same mechanism the
// shared Gateway uses; on clusters without external-dns the operator adds the
// two records manually at provision time).
func buildPerTenantGateway(namespace, hostname string) *unstructured.Unstructured {
	tlsConfig := func() map[string]interface{} {
		return map[string]interface{}{
			"mode": "Terminate",
			"certificateRefs": []interface{}{
				map[string]interface{}{
					"group": "",
					"kind":  "Secret",
					"name":  externalCertificateSecretName,
				},
			},
		}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata": map[string]interface{}{
			"name":      perTenantGatewayName(namespace),
			"namespace": namespace,
			"labels": map[string]interface{}{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		"spec": map[string]interface{}{
			"gatewayClassName": gatewayAPIGatewayClass(),
			"infrastructure": map[string]interface{}{
				"annotations": map[string]interface{}{
					"external-dns.kubernetes.io/hostname": hostname + ",*." + hostname,
				},
			},
			"listeners": []interface{}{
				map[string]interface{}{
					"name":     "grpc",
					"hostname": hostname,
					"port":     int64(443),
					"protocol": "HTTPS",
					"tls":      tlsConfig(),
					"allowedRoutes": map[string]interface{}{
						"kinds": []interface{}{
							map[string]interface{}{"group": "gateway.networking.k8s.io", "kind": "GRPCRoute"},
						},
						"namespaces": map[string]interface{}{"from": "Same"},
					},
				},
				map[string]interface{}{
					"name":     "sandbox",
					"hostname": "*." + hostname,
					"port":     int64(443),
					"protocol": "HTTPS",
					"tls":      tlsConfig(),
					"allowedRoutes": map[string]interface{}{
						"kinds": []interface{}{
							map[string]interface{}{"group": "gateway.networking.k8s.io", "kind": "HTTPRoute"},
						},
						"namespaces": map[string]interface{}{"from": "Same"},
					},
				},
			},
		},
	}}
}

// buildSandboxHTTPRoute returns the wildcard-hostname HTTPRoute carrying
// sandbox service traffic to the gateway's own listener, parented on the
// per-tenant Gateway's "sandbox" listener.
func buildSandboxHTTPRoute(namespace, hostname string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata": map[string]interface{}{
			"name":      sandboxHTTPRouteName,
			"namespace": namespace,
			"labels": map[string]interface{}{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		"spec": map[string]interface{}{
			"hostnames": []interface{}{"*." + hostname},
			"parentRefs": []interface{}{
				map[string]interface{}{
					"name":        perTenantGatewayName(namespace),
					"sectionName": "sandbox",
				},
			},
			"rules": []interface{}{
				map[string]interface{}{
					"backendRefs": []interface{}{
						map[string]interface{}{
							"name": "openshell-gateway",
							// Sandbox-service traffic (plain HTTP and WebSocket)
							// forwards to the Service's HTTP/1.1 port so the
							// ingress proxy dials the backend over HTTP/1.1; the
							// appProtocol: grpc (HTTP/2) port cannot carry a
							// WebSocket upgrade. See OSG-ROUT-13.
							"port": gatewayServiceHTTPPort,
						},
					},
				},
			},
		},
	}}
}

func reconcileGatewayAPIResources(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, nsConfig NamespaceConfig, opts ReconcileOpts) error {
	// The route-owned resources are reconciled through a kubernetes.Interface
	// core so tests can drive them with a fake clientset; the console call below
	// requires the concrete client type (console.go) and stays in this wrapper.
	if err := reconcileGatewayAPIRouteResources(ctx, dynamicClient, clientset, nsConfig, opts); err != nil {
		return err
	}

	// The console follows the route: it is deployed in the same pass that creates
	// the route resources. A console failure must not fail the gateway route
	// reconciliation, so it is logged and the reconcile continues.
	images := opts.Images
	if images == nil {
		images = StaticImageDefaults{}
	}
	if err := reconcileConsole(ctx, dynamicClient, clientset, nsConfig, opts, images); err != nil {
		slog.WarnContext(ctx, "Failed to reconcile console", "namespace", nsConfig.Name, "error", err)
	}

	return nil
}

// reconcileGatewayAPIRouteResources creates the Gateway API resources exposing
// a routed tenant gateway: the GRPCRoute onto the shared Gateway, or - for a
// dedicated-certificate gateway (route.certificate) - the external
// cert-manager Certificate, the per-tenant Gateway, the re-parented GRPCRoute,
// and the wildcard sandbox HTTPRoute. It takes a kubernetes.Interface rather
// than the concrete client type so tests can drive it with a fake clientset
// (the console reconciliation in reconcileGatewayAPIResources requires the
// concrete type and stays in the wrapper).
func reconcileGatewayAPIRouteResources(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, nsConfig NamespaceConfig, opts ReconcileOpts) error {
	namespace := nsConfig.Name
	routeConfig := nsConfig.Gateway.Route

	// A dedicated-certificate gateway (route.certificate) is served by its own
	// per-tenant Gateway, so the shared Gateway is not required; instead the
	// controller must know which cluster issuer mints the external certificate.
	// The issuer is cluster infrastructure, never user input (see
	// docs/investigations/per-gateway-certificates.md section 7, decision 2).
	dedicatedCert := routeConfig.Certificate != nil
	var gwName, gwNS string
	if dedicatedCert {
		if gatewayExternalIssuerName() == "" {
			slog.WarnContext(ctx, "route.certificate requires GATEWAY_EXTERNAL_ISSUER to be set on the controller")
			return fmt.Errorf("route.certificate requires GATEWAY_EXTERNAL_ISSUER to be set on the controller: %w", errExternalIssuerUnset)
		}
	} else {
		gwName = gatewayIngressName()
		if gwName == "" {
			slog.WarnContext(ctx, "GATEWAY_API_GATEWAY_NAME is required -- set it to the name of a pre-existing Gateway resource")
			return fmt.Errorf("GATEWAY_API_GATEWAY_NAME is required")
		}
		gwNS = gatewayIngressNamespace()
	}

	// Derive the external hostname through the Gateway Exposure adapter's shared
	// helper so the hostname baked into the GRPCRoute cannot drift from the
	// address published through the port.
	hostname, ok := exposure.DeriveGatewayAPIHost(namespace, routeConfig.Host)
	if !ok {
		slog.WarnContext(ctx, "Cannot derive GRPCRoute hostname: GATEWAY_API_BASE_DOMAIN not set")
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
			slog.WarnContext(ctx, "Failed to resolve routeAddress for gateway", "namespace", namespace, "error", err)
		} else if routeAddress != "" {
			if err := opts.UpdateRouteAddress(ctx, routeAddress); err != nil {
				slog.WarnContext(ctx, "Failed to publish routeAddress for gateway", "route_address", routeAddress, "namespace", namespace, "error", err)
			} else {
				slog.InfoContext(ctx, "Published routeAddress for gateway", "route_address", routeAddress, "namespace", namespace)
			}
		}
	}

	if dedicatedCert {
		// One cert, one DNS-01 order: the exact name serves the gateway's own
		// gRPC host; the wildcard covers every <sandbox>--<service> label below
		// it. cert-manager owns issuance and rotation from here on.
		if err := reconcileResource(ctx, dynamicClient, buildExternalCertificate(namespace, hostname)); err != nil {
			return fmt.Errorf("reconcile external Certificate: %w", err)
		}

		// The Gateway's listeners reference the issued Secret, so wait for
		// cert-manager to mint it before creating them (mirroring the
		// openshell-server-tls wait below).
		if err := waitForSecret(ctx, clientset, namespace, externalCertificateSecretName, 60*time.Second); err != nil {
			return fmt.Errorf("wait for external TLS secret in %s: %w: %w", namespace, err, errExternalCertificateNotIssued)
		}

		// The wait above can run for up to a minute; mirror the RouteStillDesired
		// re-check below so this in-flight pass does not create the per-tenant
		// Gateway behind a concurrent teardown. Fail closed on unknown intent.
		if opts.RouteStillDesired != nil {
			desired, err := opts.RouteStillDesired(ctx)
			if err != nil {
				return fmt.Errorf("re-check route intent in %s: %w", namespace, err)
			}
			if !desired {
				slog.InfoContext(ctx, "Gateway no longer routed after external TLS wait; skipping per-tenant Gateway creation (health loop owns teardown)", "namespace", namespace)
				return nil
			}
		}

		if err := reconcileResource(ctx, dynamicClient, buildPerTenantGateway(namespace, hostname)); err != nil {
			return fmt.Errorf("reconcile per-tenant Gateway: %w", err)
		}
	}

	var parentRef map[string]interface{}
	if dedicatedCert {
		// Same namespace as the GRPCRoute, so no ReferenceGrant is needed.
		parentRef = map[string]interface{}{
			"name":        perTenantGatewayName(namespace),
			"namespace":   namespace,
			"sectionName": "grpc",
		}
	} else {
		listenerName := sharedGatewayListenerName()
		if os.Getenv("GATEWAY_API_HTTP_LISTENER_NAME") == "" {
			slog.InfoContext(ctx, "Using Gateway listener for tenant (GATEWAY_API_HTTP_LISTENER_NAME unset; a NoMatchingParent GRPCRoute will not self-heal)",
				"gateway_namespace", gwNS,
				"gateway_name", gwName,
				"listener", listenerName,
				"namespace", namespace,
			)
		} else {
			slog.InfoContext(ctx, "Using Gateway listener for tenant",
				"gateway_namespace", gwNS,
				"gateway_name", gwName,
				"listener", listenerName,
				"namespace", namespace,
			)
		}

		parentRef = map[string]interface{}{
			"name":        gwName,
			"namespace":   gwNS,
			"sectionName": listenerName,
		}
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
								// gRPC traffic keeps the appProtocol: grpc
								// (HTTP/2) port, unchanged by OSG-ROUT-13.
								"port": gatewayServiceGRPCPort,
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

	if dedicatedCert {
		// Sandbox service traffic (ExposeService HTTP virtual-hosting, plus the
		// per-gateway console) arrives on *.gw-<namespace>... hosts and is
		// virtual-hosted by the gateway's own listener. The BackendTLSPolicy
		// below re-encrypts this leg with the pod's internal cert, same as the
		// gRPC path.
		if err := reconcileResource(ctx, dynamicClient, buildSandboxHTTPRoute(namespace, hostname)); err != nil {
			return fmt.Errorf("reconcile sandbox HTTPRoute: %w", err)
		}
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
			slog.InfoContext(ctx, "Gateway no longer routed after TLS wait; skipping route/console resource creation (health loop owns teardown)", "namespace", namespace)
			return nil
		}
	}

	if err := reconcileBackendTLS(ctx, dynamicClient, clientset, namespace); err != nil {
		return err
	}

	// The route address is published deterministically at the top of this
	// function, so no readiness-gated discovery is required here.

	slog.InfoContext(ctx, "Gateway API resources reconciled", "namespace", namespace, "hostname", hostname)
	return nil
}
