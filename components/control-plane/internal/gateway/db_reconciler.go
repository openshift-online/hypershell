package gateway

import (
	"context"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// DatabaseReconciler is implemented by each database provider (cnpg, deployment, external).
// Reconcile provisions or updates database resources for a gateway tenant namespace.
// Delete removes out-of-namespace database resources. A non-nil error signals a transient
// failure that the caller should retry; terminal failures (e.g. missing admin secret) are
// handled internally by returning nil so gateway finalization is not stranded.
// CNPG and deployment providers always return nil.
type DatabaseReconciler interface {
	Reconcile(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, tenantNamespace, gatewayID, rotateAnnotation string) error
	Delete(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, gatewayID string) error
}

// newDatabaseReconciler constructs the correct DatabaseReconciler for opts.DatabaseProvider.
// Returns an error for invalid configurations (missing required fields, CNPG not available).
// Returns a noopDatabaseReconciler for the legacy empty-provider case.
func newDatabaseReconciler(opts ReconcileOpts) (DatabaseReconciler, error) {
	switch opts.DatabaseProvider {
	case "":
		return &noopDatabaseReconciler{}, nil
	case "cnpg":
		if opts.CNPG.ClusterNamespace == "" {
			return nil, fmt.Errorf("CNPG cluster namespace is required for gateway database reconciliation")
		}
		if !opts.HasCNPG {
			return nil, fmt.Errorf("CNPG operator is required but not available on the cluster: gateway deployment blocked")
		}
		return &cnpgDatabaseReconciler{cnpg: opts.CNPG}, nil
	case "deployment":
		if opts.DeploymentDBNamespace == "" {
			return nil, fmt.Errorf("deployment database namespace is required for gateway database reconciliation")
		}
		return &deploymentDatabaseReconciler{dbNamespace: opts.DeploymentDBNamespace}, nil
	case "external":
		if opts.ExternalDB.SecretName == "" {
			return nil, fmt.Errorf("external database connection_secret is required for gateway database reconciliation")
		}
		return &externalDatabaseReconciler{cfg: opts.ExternalDB}, nil
	default:
		return nil, fmt.Errorf("unsupported database provider %q", opts.DatabaseProvider)
	}
}

// noopDatabaseReconciler handles legacy gateways with no DatabaseProvider set.
// It preserves any existing database resources without touching them.
type noopDatabaseReconciler struct{}

func (r *noopDatabaseReconciler) Reconcile(_ context.Context, _ dynamic.Interface, _ kubernetes.Interface, _, _, _ string) error {
	return nil
}

func (r *noopDatabaseReconciler) Delete(_ context.Context, _ dynamic.Interface, _ kubernetes.Interface, _ string) error {
	return nil
}
