package gateways

import (
	"context"
	"sync"
)

type DeletionCleaner func(context.Context, string) error

var (
	deletionCleanerMu sync.RWMutex
	deletionCleaner   DeletionCleaner
)

// RegisterDeletionCleaner installs the service-account cleanup barrier. The
// gateway is not soft-deleted until this callback confirms that its machine
// identities are disabled and removed.
func RegisterDeletionCleaner(cleaner DeletionCleaner) {
	deletionCleanerMu.Lock()
	defer deletionCleanerMu.Unlock()
	deletionCleaner = cleaner
}

func cleanBeforeDeletion(ctx context.Context, gatewayID string) error {
	deletionCleanerMu.RLock()
	cleaner := deletionCleaner
	deletionCleanerMu.RUnlock()
	if cleaner == nil {
		return nil
	}
	return cleaner(ctx, gatewayID)
}
