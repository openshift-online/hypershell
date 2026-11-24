package gateway

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Verifies: OSG-CORE-19.02
func TestEnsurePolicyBundleReaderGrantsOnlyTheBundle(t *testing.T) {
	kube := fake.NewSimpleClientset()
	ctx := context.Background()
	for i := 0; i < 2; i++ { // idempotent
		if err := EnsurePolicyBundleReader(ctx, kube, "ns-g"); err != nil {
			t.Fatal(err)
		}
	}
	role, err := kube.RbacV1().Roles("ns-g").Get(ctx, PolicyBundleReaderName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := role.Rules[0]
	if len(role.Rules) != 1 || r.ResourceNames[0] != "hypershell-policy-bundle" || len(r.Verbs) != 3 {
		t.Fatalf("role must grant read of the bundle ConfigMap only: %+v", role.Rules)
	}
	rb, err := kube.RbacV1().RoleBindings("ns-g").Get(ctx, PolicyBundleReaderName, metav1.GetOptions{})
	if err != nil || rb.Subjects[0].Name != "openshell-gateway" {
		t.Fatalf("binding: %+v %v", rb, err)
	}
}
