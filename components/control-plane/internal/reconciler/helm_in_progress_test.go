package reconciler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

// A young pending Helm release retries with reason helm_operation_in_progress
// instead of failing the gateway; other errors are not matched.
func TestHelmInProgressRetry(t *testing.T) {
	inProgress := &gateway.HelmOperationInProgressError{Namespace: "gw-ns", Revision: 3, Status: "pending-upgrade"}
	err := fmt.Errorf("deploy gateway via helm in gw-ns: %w", inProgress)

	retryErr, ok := helmInProgressRetry("gw", err)
	if !ok {
		t.Fatal("in-progress error not recognised")
	}
	if got := watcher.RetryableReason(retryErr); got != cpotel.ReasonHelmOperationInProgress {
		t.Fatalf("retry reason = %q, want %q", got, cpotel.ReasonHelmOperationInProgress)
	}
	var target *gateway.HelmOperationInProgressError
	if !errors.As(retryErr, &target) || target.Revision != 3 {
		t.Fatalf("retry error lost the cause: %v", retryErr)
	}

	if _, ok := helmInProgressRetry("gw", errors.New("helm upgrade: boom")); ok {
		t.Fatal("ordinary helm error treated as in progress")
	}
}
