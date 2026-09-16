package gateway

import (
	"context"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// DatabaseReconciler provisions and removes a gateway's database on the
// PostgreSQL server registered as its ManagedDatabase. Reconcile creates the
// per-gateway database and role and writes the tenant credentials Secret.
// Delete drops them. A non-nil error signals a transient failure that the
// caller should retry; cleanup is unconditional and single-shot (no tombstone,
// no retry queue), so a failed drop is logged rather than retried.
type DatabaseReconciler interface {
	Reconcile(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, tenantNamespace, gatewayID string) error
	Delete(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, gatewayID string) error
}

// newDatabaseReconciler constructs the DatabaseReconciler for opts. The admin
// credentials namespace (ManagedDatabase.connection_secret) is required: without
// it there is no server to issue DDL against.
func newDatabaseReconciler(opts ReconcileOpts) (DatabaseReconciler, error) {
	if opts.ExternalDB.CredentialsNamespace == "" {
		return nil, fmt.Errorf("database connection_secret (credentials namespace) is required for gateway database reconciliation")
	}
	return &externalDatabaseReconciler{cfg: opts.ExternalDB}, nil
}
