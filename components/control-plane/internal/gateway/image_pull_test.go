package gateway

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
)

func imagePullTestClient(t *testing.T) *fake.Clientset {
	t.Helper()
	t.Setenv("GATEWAY_SANDBOX_IMAGE_PULL_ROLES", `[{"namespace":"images","role":"runner-pull"}]`)
	return fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway", UID: "namespace-uid", Labels: ManagedNamespaceLabels("controller")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "openshell-gateway-config", Namespace: "gateway"}, Data: map[string]string{"gateway.toml": "[openshell.drivers.kubernetes]\nservice_account_name = 'sandbox'"}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "sandbox", Namespace: "gateway"}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "runner-pull", Namespace: "images"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{"image.openshift.io"}, Resources: []string{"imagestreams/layers"}, ResourceNames: []string{"runner"}, Verbs: []string{"get"}}}},
	)
}

func imagePullTestBinding(t *testing.T, client *fake.Clientset) *rbacv1.RoleBinding {
	t.Helper()
	list, err := client.RbacV1().RoleBindings("images").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("expected one image pull binding, got %v, %v", list, err)
	}
	return &list.Items[0]
}

func TestImagePullBindingReconcilesIdentityAndOwnership(t *testing.T) {
	client := imagePullTestClient(t)
	ctx := context.Background()
	reconcile := func() {
		t.Helper()
		if err := ReconcileSandboxImagePullRoles(ctx, client, "gateway", "controller", "gateway-id"); err != nil {
			t.Fatal(err)
		}
	}
	reconcile()
	binding := imagePullTestBinding(t, client)
	if len(binding.Subjects) != 1 || binding.Subjects[0].Kind != "ServiceAccount" || binding.Subjects[0].Name != "sandbox" || binding.Subjects[0].Namespace != "gateway" || binding.OwnerReferences[0].UID != "namespace-uid" || binding.RoleRef.Name != "runner-pull" {
		t.Fatalf("unexpected image pull grant: %#v", binding)
	}
	client.ClearActions()
	reconcile()
	for _, action := range client.Actions() {
		if action.GetVerb() == "create" || action.GetVerb() == "update" || action.GetVerb() == "delete" {
			t.Fatal("unchanged grant caused a write")
		}
	}
	binding.Subjects = []rbacv1.Subject{{Kind: "Group", Name: "system:serviceaccounts"}}
	binding.OwnerReferences = nil
	if _, err := client.RbacV1().RoleBindings("images").Update(ctx, binding, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	reconcile()
	binding = imagePullTestBinding(t, client)
	if len(binding.OwnerReferences) != 1 || binding.Subjects[0].Name != "sandbox" || binding.Subjects[0].Kind != "ServiceAccount" {
		t.Fatal("grant drift was not repaired")
	}
	binding.Labels = nil
	if _, err := client.RbacV1().RoleBindings("images").Update(ctx, binding, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	reconcile()
	if imagePullTestBinding(t, client).Labels[imagePullGatewayLabel] != "gateway-id" {
		t.Fatal("owned grant labels were not repaired")
	}
	t.Setenv("GATEWAY_SANDBOX_IMAGE_PULL_ROLES", "[]")
	reconcile()
	list, err := client.RbacV1().RoleBindings("images").List(ctx, metav1.ListOptions{})
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("removed configuration retained grants: %v, %v", list, err)
	}
}

func TestImagePullBindingRejectsForeignOwnershipAndBroadRoles(t *testing.T) {
	for _, kind := range []string{"foreign binding", "broad Role", "other controller", "managed workspaces"} {
		t.Run(kind, func(t *testing.T) {
			client := imagePullTestClient(t)
			ctx := context.Background()
			if err := ReconcileSandboxImagePullRoles(ctx, client, "gateway", "controller", "gateway-id"); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "foreign binding":
				binding := imagePullTestBinding(t, client)
				binding.Annotations[imagePullNamespaceUID] = "another-uid"
				binding.OwnerReferences[0].UID = "another-uid"
				if _, err := client.RbacV1().RoleBindings("images").Update(ctx, binding, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			case "broad Role":
				role, err := client.RbacV1().Roles("images").Get(ctx, "runner-pull", metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				role.Rules[0].ResourceNames = nil
				if _, err := client.RbacV1().Roles("images").Update(ctx, role, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			case "other controller":
				ns, err := client.CoreV1().Namespaces().Get(ctx, "gateway", metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				ns.Labels[InstanceLabel] = "other-controller"
				if _, err := client.CoreV1().Namespaces().Update(ctx, ns, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			case "managed workspaces":
				cm, err := client.CoreV1().ConfigMaps("gateway").Get(ctx, "openshell-gateway-config", metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				cm.Data["gateway.toml"] += "\nworkspace_mode = 'managed'"
				if _, err := client.CoreV1().ConfigMaps("gateway").Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			client.ClearActions()
			if err := ReconcileSandboxImagePullRoles(ctx, client, "gateway", "controller", "gateway-id"); err == nil {
				t.Fatal("unsafe image pull configuration was accepted")
			}
			if kind == "foreign binding" || kind == "other controller" {
				for _, action := range client.Actions() {
					if action.GetVerb() == "update" || action.GetVerb() == "delete" {
						t.Fatal("changed another owner's resource")
					}
				}
			} else {
				list, err := client.RbacV1().RoleBindings("images").List(ctx, metav1.ListOptions{})
				if err != nil || len(list.Items) != 0 {
					t.Fatalf("unsafe grant remained active: %v, %v", list, err)
				}
			}
		})
	}
}

func TestImagePullBindingRetriesCreationAndDeletionFailures(t *testing.T) {
	client := imagePullTestClient(t)
	ctx := context.Background()
	blocked := true
	client.PrependReactor("create", "rolebindings", func(kubetesting.Action) (bool, runtime.Object, error) {
		if blocked {
			return true, nil, errors.New("bind forbidden")
		}
		return false, nil, nil
	})
	if err := ReconcileSandboxImagePullRoles(ctx, client, "gateway", "controller", "gateway-id"); err == nil {
		t.Fatal("grant creation failure was lost")
	}
	blocked = false
	if err := ReconcileSandboxImagePullRoles(ctx, client, "gateway", "controller", "gateway-id"); err != nil {
		t.Fatal(err)
	}
	// A second gateway's binding must survive deletion of the first gateway.
	other := imagePullTestBinding(t, client).DeepCopy()
	other.Name += "-other"
	other.Labels[imagePullGatewayLabel] = "other-gateway"
	if _, err := client.RbacV1().RoleBindings("images").Create(ctx, other, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	blocked = true
	client.PrependReactor("delete", "rolebindings", func(kubetesting.Action) (bool, runtime.Object, error) {
		return blocked, nil, nil // Model a finalizer that accepts but delays deletion.
	})
	if err := DeleteSandboxImagePullRoles(ctx, client, "gateway", "controller", "gateway-id"); err == nil {
		t.Fatal("pending grant deletion was reported as complete")
	}
	blocked = false
	if err := DeleteSandboxImagePullRoles(ctx, client, "gateway", "controller", "gateway-id"); err != nil {
		t.Fatal(err)
	}
	remaining := imagePullTestBinding(t, client)
	if remaining.Labels[imagePullGatewayLabel] != "other-gateway" {
		t.Fatal("another gateway's grant was removed")
	}
}
