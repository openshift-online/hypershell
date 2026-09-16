package roleBindings

import (
	"context"
	"testing"

	"github.com/openshift-online/rh-trex-ai/pkg/errors"
)

func TestLoadRoleBindingWithRetryRetriesNotFound(t *testing.T) {
	attempts := 0
	want := &RoleBinding{}
	got, err := loadRoleBindingWithRetry(context.Background(), func() (*RoleBinding, *errors.ServiceError) {
		attempts++
		if attempts < 3 {
			return nil, errors.NotFound("pending")
		}
		return want, nil
	})
	if err != nil {
		t.Fatalf("loadRoleBindingWithRetry() error = %v", err)
	}
	if got != want {
		t.Fatalf("loadRoleBindingWithRetry() = %v, want the committed binding", got)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestLoadRoleBindingWithRetryDoesNotRetryOtherErrors(t *testing.T) {
	attempts := 0
	_, err := loadRoleBindingWithRetry(context.Background(), func() (*RoleBinding, *errors.ServiceError) {
		attempts++
		return nil, errors.GeneralError("boom")
	})
	if err == nil {
		t.Fatal("loadRoleBindingWithRetry() error = nil, want GeneralError")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on non-NotFound)", attempts)
	}
}
