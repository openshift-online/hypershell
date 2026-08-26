package gateways

import (
	"context"
	"sync"
)

// DeletionCleaner disables and removes a gateway's
// OpenShellGatewayServiceAccounts, then invokes finalize to delete the gateway
// row. The cleaner runs finalize while it still holds the per-gateway lifecycle
// barrier so a concurrent service-account create cannot observe the gateway as
// still present after cleanup completes but before the row is gone.
type DeletionCleaner func(ctx context.Context, gatewayID string, finalize func(context.Context) error) error

var (
	deletionCleanerMu sync.RWMutex
	deletionCleaner   DeletionCleaner
)

// RegisterDeletionCleaner installs the service-account cleanup barrier. The
// gateway is not soft-deleted until this callback confirms that its
// OpenShellGatewayServiceAccounts are disabled and removed.
func RegisterDeletionCleaner(cleaner DeletionCleaner) {
	deletionCleanerMu.Lock()
	defer deletionCleanerMu.Unlock()
	deletionCleaner = cleaner
}

// cleanBeforeDeletion runs the registered cleanup barrier and, through it, the
// finalize callback that deletes the gateway row. When no cleaner is registered
// finalize still runs so the gateway can be deleted.
func cleanBeforeDeletion(ctx context.Context, gatewayID string, finalize func(context.Context) error) error {
	deletionCleanerMu.RLock()
	cleaner := deletionCleaner
	deletionCleanerMu.RUnlock()
	if cleaner == nil {
		return finalize(ctx)
	}
	return cleaner(ctx, gatewayID, finalize)
}
