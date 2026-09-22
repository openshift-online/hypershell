package gateway

import (
	"context"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// DatabaseReconciler provisions and removes a gateway's database on the
// PostgreSQL server the control plane's mounted admin credentials point at.
// Reconcile creates the per-gateway database and role and writes the tenant
// credentials Secret. Delete drops them. A non-nil error from either signals
// that the work is incomplete and the caller should retry.
type DatabaseReconciler interface {
	Reconcile(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, tenantNamespace, gatewayID string) error
	Delete(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, gatewayID string) error
}

// newDatabaseReconciler constructs the DatabaseReconciler for opts. The admin
// credentials directory is required: it is validated at controller startup, so
// an empty value here is a wiring error, not an operator error.
func newDatabaseReconciler(opts ReconcileOpts) (DatabaseReconciler, error) {
	if opts.databaseReconciler != nil {
		return opts.databaseReconciler, nil
	}
	if opts.Database.AdminCredentialsDir == "" {
		return nil, fmt.Errorf("gateway database admin credentials directory is required for database reconciliation")
	}
	return &databaseReconciler{cfg: opts.Database}, nil
}
