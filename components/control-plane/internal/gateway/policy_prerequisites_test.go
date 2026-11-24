package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Verifies: CTRL-CORE-22.06, PLAT-PGP-12.03
func TestPolicyPrerequisitesWriteTheBundleAfterTheReaderBeforeHelm(t *testing.T) {
	ctx := context.Background()
	kube := fake.NewSimpleClientset()
	var calls []string
	opts := ReconcileOpts{EnsurePolicyBundle: func(ctx context.Context, namespace string) error {
		// The sidecar's read access exists by the time the bundle is written.
		if _, err := kube.RbacV1().Roles(namespace).Get(ctx, PolicyBundleReaderName, metav1.GetOptions{}); err != nil {
			t.Errorf("bundle written before the reader Role: %v", err)
		}
		calls = append(calls, namespace)
		return nil
	}}
	if msg, err := ensurePolicyPrerequisites(ctx, kube, "openshell-abc", opts); err != nil {
		t.Fatalf("prerequisites: %s: %v", msg, err)
	}
	if len(calls) != 1 || calls[0] != "openshell-abc" {
		t.Fatalf("the bundle must be written once for the gateway namespace, got %v", calls)
	}
}

// Verifies: CTRL-CORE-22.06, PLAT-PGP-12.03
func TestPolicyPrerequisitesFailTheReconcileWhenTheBundleCannotBeWritten(t *testing.T) {
	ctx := context.Background()
	notYet := errors.New("platform policy settings carry no fleet defaults yet")
	msg, err := ensurePolicyPrerequisites(ctx, fake.NewSimpleClientset(), "openshell-abc",
		ReconcileOpts{EnsurePolicyBundle: func(context.Context, string) error { return notYet }})
	if !errors.Is(err, notYet) || !strings.Contains(msg, "policy bundle is not available yet") {
		t.Fatalf("an unbuildable bundle must fail the reconcile so it retries, got %q, %v", msg, err)
	}
	if _, err := ensurePolicyPrerequisites(ctx, fake.NewSimpleClientset(), "openshell-abc", ReconcileOpts{}); err == nil {
		t.Fatal("an interceptor without a bundle writer must fail the reconcile")
	}
}
