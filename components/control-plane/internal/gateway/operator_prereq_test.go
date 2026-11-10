package gateway

import (
	"context"
	"strings"
	"testing"

	nodev1 "k8s.io/api/node/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestValidateOperatorPrerequisites covers the fail-closed operator prerequisite
// checks (OSG-OPWS-04): a missing namespace label key, a missing sandbox
// RuntimeClass config value, and an approved RuntimeClass absent on the cluster
// each fail with an actionable error and never downgrade the mode; all present
// with the RuntimeClass installed succeeds.
func TestValidateOperatorPrerequisites(t *testing.T) {
	ctx := context.Background()

	t.Run("missing namespace label key fails closed", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		opts := ReconcileOpts{OperatorSandboxRuntimeClass: "kata"}
		err := validateOperatorPrerequisites(ctx, client, opts)
		if err == nil {
			t.Fatal("expected error for missing operator namespace label key")
		}
		if !strings.Contains(err.Error(), "GATEWAY_OPERATOR_NAMESPACE_LABEL_KEY") {
			t.Errorf("error should name the missing config var: %v", err)
		}
	})

	t.Run("missing sandbox runtime class config fails closed", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		opts := ReconcileOpts{OperatorNamespaceLabelKey: "oce.example.com/tenant"}
		err := validateOperatorPrerequisites(ctx, client, opts)
		if err == nil {
			t.Fatal("expected error for missing operator sandbox runtime class")
		}
		if !strings.Contains(err.Error(), "GATEWAY_OPERATOR_SANDBOX_RUNTIME_CLASS") {
			t.Errorf("error should name the missing config var: %v", err)
		}
	})

	t.Run("approved runtime class absent on cluster fails closed", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		opts := ReconcileOpts{
			OperatorNamespaceLabelKey:   "oce.example.com/tenant",
			OperatorSandboxRuntimeClass: "kata",
		}
		err := validateOperatorPrerequisites(ctx, client, opts)
		if err == nil {
			t.Fatal("expected error for a RuntimeClass missing from the cluster")
		}
		if !strings.Contains(err.Error(), "kata") {
			t.Errorf("error should name the missing RuntimeClass: %v", err)
		}
	})

	t.Run("all prerequisites met succeeds", func(t *testing.T) {
		client := fake.NewSimpleClientset(&nodev1.RuntimeClass{
			ObjectMeta: metav1.ObjectMeta{Name: "kata"},
			Handler:    "kata",
		})
		opts := ReconcileOpts{
			OperatorNamespaceLabelKey:   "oce.example.com/tenant",
			OperatorSandboxRuntimeClass: "kata",
		}
		if err := validateOperatorPrerequisites(ctx, client, opts); err != nil {
			t.Fatalf("validateOperatorPrerequisites() error = %v, want nil", err)
		}
	})
}
