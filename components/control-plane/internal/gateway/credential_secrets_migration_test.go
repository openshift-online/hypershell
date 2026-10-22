package gateway

import (
	"context"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const credSecretsName = "openshell-gateway-credential-secrets"

var (
	credSecretsRoleGVR = schema.GroupVersionResource{
		Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles",
	}
	credSecretsRoleBindingGVR = schema.GroupVersionResource{
		Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings",
	}
)

func credSecretsRBACListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		credSecretsRoleGVR:        "RoleList",
		credSecretsRoleBindingGVR: "RoleBindingList",
	}
}

func credSecretsRBACObject(kind, namespace string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("rbac.authorization.k8s.io/v1")
	obj.SetKind(kind)
	obj.SetName(credSecretsName)
	obj.SetNamespace(namespace)
	if labels != nil {
		obj.SetLabels(labels)
	}
	return obj
}

var (
	legacyControlPlaneLabels = map[string]string{
		"app.kubernetes.io/managed-by": "hypershell-control-plane",
		"hypershell.redhat.io/managed": "true",
	}
	helmOwnedLabels    = map[string]string{"app.kubernetes.io/managed-by": "Helm"}
	foreignOwnedLabels = map[string]string{"app.kubernetes.io/managed-by": "some-other-operator"}
)

func exists(t *testing.T, dc *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource, namespace string) bool {
	t.Helper()
	_, err := dc.Resource(gvr).Namespace(namespace).Get(context.Background(), credSecretsName, metav1.GetOptions{})
	if err == nil {
		return true
	}
	if k8serrors.IsNotFound(err) {
		return false
	}
	t.Fatalf("unexpected error reading %s: %v", gvr.Resource, err)
	return false
}

// TestDeleteControlPlaneOwnedCredentialSecretsRBAC verifies the migration cleanup
// removes only the control plane's own legacy Role/RoleBinding and never a
// Helm-owned, foreign-owned, or unlabeled object, inspecting each resource
// independently so a partial stale state is fully cleaned.
func TestDeleteControlPlaneOwnedCredentialSecretsRBAC(t *testing.T) {
	const ns = "openshell-tenant"

	cases := []struct {
		name            string
		roleLabels      map[string]string // nil means the Role is absent
		bindingLabels   map[string]string // nil means the RoleBinding is absent
		wantRole        bool
		wantRoleBinding bool
	}{
		{
			name:          "legacy control-plane objects are both deleted",
			roleLabels:    legacyControlPlaneLabels,
			bindingLabels: legacyControlPlaneLabels,
		},
		{
			name:            "helm-owned objects are left untouched",
			roleLabels:      helmOwnedLabels,
			bindingLabels:   helmOwnedLabels,
			wantRole:        true,
			wantRoleBinding: true,
		},
		{
			name:            "foreign-owned objects are left untouched",
			roleLabels:      foreignOwnedLabels,
			bindingLabels:   foreignOwnedLabels,
			wantRole:        true,
			wantRoleBinding: true,
		},
		{
			name:            "unlabeled objects are left untouched",
			roleLabels:      map[string]string{},
			bindingLabels:   map[string]string{},
			wantRole:        true,
			wantRoleBinding: true,
		},
		{
			name:          "partial state: a lone legacy RoleBinding is cleaned even with no Role",
			roleLabels:    nil,
			bindingLabels: legacyControlPlaneLabels,
		},
		{
			name:            "independent inspection: legacy Role removed, Helm RoleBinding kept",
			roleLabels:      legacyControlPlaneLabels,
			bindingLabels:   helmOwnedLabels,
			wantRoleBinding: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seed []runtime.Object
			if tc.roleLabels != nil {
				seed = append(seed, credSecretsRBACObject("Role", ns, tc.roleLabels))
			}
			if tc.bindingLabels != nil {
				seed = append(seed, credSecretsRBACObject("RoleBinding", ns, tc.bindingLabels))
			}
			dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), credSecretsRBACListKinds(), seed...)

			deleteControlPlaneOwnedCredentialSecretsRBAC(context.Background(), dc, ns)

			if got := exists(t, dc, credSecretsRoleGVR, ns); got != tc.wantRole {
				t.Errorf("Role present = %v, want %v", got, tc.wantRole)
			}
			if got := exists(t, dc, credSecretsRoleBindingGVR, ns); got != tc.wantRoleBinding {
				t.Errorf("RoleBinding present = %v, want %v", got, tc.wantRoleBinding)
			}
		})
	}
}
