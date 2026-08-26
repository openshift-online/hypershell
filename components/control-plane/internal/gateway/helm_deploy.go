package gateway

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// deployGatewayViaHelm installs or upgrades a gateway using the Helm chart.
func deployGatewayViaHelm(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	helmClient *helm.ShellClient,
	nsConfig NamespaceConfig,
	opts ReconcileOpts,
) error {
	// Build Helm values from gateway configuration
	valuesBuilder := &helm.ValuesBuilder{
		Gateway: helm.GatewayConfig{
			Image:           nsConfig.Gateway.Image,
			SupervisorImage: nsConfig.Gateway.SupervisorImage,
			ServerDnsNames:  nsConfig.Gateway.ServerDnsNames,
			OIDC: helm.OIDCConfig{
				Issuer:      nsConfig.Gateway.OIDC.Issuer,
				Audience:    nsConfig.Gateway.OIDC.Audience,
				RolesClaim:  nsConfig.Gateway.OIDC.RolesClaim,
				AdminRole:   nsConfig.Gateway.OIDC.AdminRole,
				UserRole:    nsConfig.Gateway.OIDC.UserRole,
				ScopesClaim: nsConfig.Gateway.OIDC.ScopesClaim,
			},
			Route: helm.RouteConfig{
				Host:    nsConfig.Gateway.Route.Host,
				Enabled: nsConfig.Gateway.Route.Enabled,
			},
			CredentialDriver: convertCredentialDriver(nsConfig.Gateway.CredentialDriver),
		},
		Namespace:                  nsConfig.Name,
		HasCertManager:             opts.HasCertManager,
		IsOpenShift:                opts.IsOpenShift,
		HasGatewayAPI:              opts.HasGatewayAPI,
		GatewayAPIGatewayName:      getGatewayAPIGatewayName(),
		GatewayAPIGatewayNamespace: getGatewayAPIGatewayNamespace(),
		IngressBaseDomain:          opts.IngressBaseDomain,
		ExternalCAIssuerName:       opts.ExternalCAIssuerName,
		ExternalCAIssuerKind:       opts.ExternalCAIssuerKind,
	}

	values, err := valuesBuilder.Build()
	if err != nil {
		return fmt.Errorf("build helm values: %w", err)
	}

	// Adopt cluster-scoped resources before Helm install/upgrade. The chart
	// creates a ClusterRole and ClusterRoleBinding with a fixed name. In
	// multi-tenant setups (multiple gateways on the same cluster), only one
	// Helm release can own them. Transfer ownership to the current release so
	// the install succeeds, and maintain per-namespace bindings for all SAs.
	if clientset != nil {
		adoptHelmClusterResources(ctx, clientset, nsConfig.Name)
	}

	// Check if Helm release exists
	status, err := helmClient.GetReleaseStatus(ctx, nsConfig.Name)
	if err != nil {
		return fmt.Errorf("check helm release status: %w", err)
	}

	// Install or upgrade the release
	if status == nil {
		// No release exists, install it
		log.Printf("INFO installing helm release in namespace %s", nsConfig.Name)
		if err := helmClient.Install(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("helm install: %w", err)
		}
	} else if status.Status == "failed" || status.Status == "pending-install" {
		// Failed install, retry via upgrade
		log.Printf("INFO retrying failed helm release in namespace %s (status: %s)", nsConfig.Name, status.Status)
		if err := helmClient.Upgrade(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("helm upgrade: %w", err)
		}
	} else {
		// Release exists and is not failed, upgrade it
		log.Printf("INFO upgrading helm release in namespace %s (current status: %s)", nsConfig.Name, status.Status)
		if err := helmClient.Upgrade(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("helm upgrade: %w", err)
		}
	}

	log.Printf("INFO helm release deployed in namespace %s", nsConfig.Name)

	// Ensure a per-namespace ClusterRoleBinding exists so this gateway's SA
	// retains node-reader permissions even when the shared Helm-managed
	// ClusterRoleBinding is adopted by a different release.
	if clientset != nil {
		ensureGatewayClusterRoleBinding(ctx, clientset, nsConfig.Name)
	}

	return nil
}

const helmClusterResourceName = "openshell-gateway-node-reader"

// adoptHelmClusterResources updates the Helm ownership annotations on
// cluster-scoped resources so the current release can adopt them.
func adoptHelmClusterResources(ctx context.Context, clientset *kubernetes.Clientset, namespace string) {
	helmAnnotations := map[string]string{
		"meta.helm.sh/release-name":      helm.ReleaseName,
		"meta.helm.sh/release-namespace": namespace,
	}
	helmLabel := map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
	}

	cr, err := clientset.RbacV1().ClusterRoles().Get(ctx, helmClusterResourceName, metav1.GetOptions{})
	if err == nil {
		annotations := cr.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		needsUpdate := false
		for k, v := range helmAnnotations {
			if annotations[k] != v {
				annotations[k] = v
				needsUpdate = true
			}
		}
		labels := cr.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		for k, v := range helmLabel {
			if labels[k] != v {
				labels[k] = v
				needsUpdate = true
			}
		}
		if needsUpdate {
			cr.SetAnnotations(annotations)
			cr.SetLabels(labels)
			if _, err := clientset.RbacV1().ClusterRoles().Update(ctx, cr, metav1.UpdateOptions{}); err != nil {
				log.Printf("WARN failed to adopt ClusterRole %s for namespace %s: %v", helmClusterResourceName, namespace, err)
			}
		}
	}

	crb, err := clientset.RbacV1().ClusterRoleBindings().Get(ctx, helmClusterResourceName, metav1.GetOptions{})
	if err == nil {
		annotations := crb.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		needsUpdate := false
		for k, v := range helmAnnotations {
			if annotations[k] != v {
				annotations[k] = v
				needsUpdate = true
			}
		}
		labels := crb.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		for k, v := range helmLabel {
			if labels[k] != v {
				labels[k] = v
				needsUpdate = true
			}
		}
		if needsUpdate {
			crb.SetAnnotations(annotations)
			crb.SetLabels(labels)
			if _, err := clientset.RbacV1().ClusterRoleBindings().Update(ctx, crb, metav1.UpdateOptions{}); err != nil {
				log.Printf("WARN failed to adopt ClusterRoleBinding %s for namespace %s: %v", helmClusterResourceName, namespace, err)
			}
		}
	}
}

// ensureGatewayClusterRoleBinding creates or updates a per-namespace
// ClusterRoleBinding that grants the gateway ServiceAccount node-reader
// permissions. This is separate from the Helm-managed ClusterRoleBinding to
// support multiple gateways on the same cluster.
func ensureGatewayClusterRoleBinding(ctx context.Context, clientset *kubernetes.Clientset, namespace string) {
	name := fmt.Sprintf("%s-%s", helmClusterResourceName, namespace)
	desired := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     helmClusterResourceName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "openshell-gateway",
				Namespace: namespace,
			},
		},
	}

	existing, err := clientset.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, desired, metav1.CreateOptions{}); err != nil {
				log.Printf("WARN failed to create per-namespace ClusterRoleBinding %s: %v", name, err)
			}
		}
		return
	}

	desired.ResourceVersion = existing.ResourceVersion
	if _, err := clientset.RbacV1().ClusterRoleBindings().Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		log.Printf("WARN failed to update per-namespace ClusterRoleBinding %s: %v", name, err)
	}
}

// convertCredentialDriver converts gateway.CredentialDriverConfig to helm.CredentialDriverConfig.
func convertCredentialDriver(driver *CredentialDriverConfig) *helm.CredentialDriverConfig {
	if driver == nil {
		return nil
	}
	return &helm.CredentialDriverConfig{
		Type: driver.Type,
	}
}

// getGatewayAPIGatewayName returns the name of the shared Gateway API Gateway resource.
func getGatewayAPIGatewayName() string {
	// Read from environment variable (required when Gateway API is available)
	// See specs/platform/openshell-gateway-routing.spec.md
	return getEnv("GATEWAY_API_GATEWAY_NAME", "")
}

// getGatewayAPIGatewayNamespace returns the namespace of the shared Gateway API Gateway resource.
func getGatewayAPIGatewayNamespace() string {
	// Read from environment variable
	return getEnv("GATEWAY_API_GATEWAY_NAMESPACE", "")
}

// getEnv retrieves an environment variable with a fallback default.
func getEnv(key, fallback string) string {
	if value := getEnvHelper(key); value != "" {
		return value
	}
	return fallback
}

// getEnvHelper retrieves environment variables (uses os package).
func getEnvHelper(key string) string {
	return os.Getenv(key)
}
