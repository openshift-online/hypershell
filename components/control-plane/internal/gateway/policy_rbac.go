package gateway

import (
	"context"
	"fmt"
	"reflect"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

// PolicyBundleReaderName names the Role and RoleBinding that let the gateway
// pod's service account read its policy bundle ConfigMap (OSG-CORE-19).
const PolicyBundleReaderName = "hypershell-policy-bundle-reader"

// policyBundleConfigMap mirrors policybundle.ConfigMapName (kept local to
// avoid an import cycle).
const policyBundleConfigMap = "hypershell-policy-bundle"

// EnsurePolicyBundleReader update-or-creates a Role granting get, list, and
// watch on exactly the hypershell-policy-bundle ConfigMap, bound to the
// gateway's service account, so the interceptor sidecar watches its bundle
// through the Kubernetes API with no other access.
func EnsurePolicyBundleReader(ctx context.Context, client kubernetes.Interface, namespace string) error {
	labels := map[string]string{"app.kubernetes.io/managed-by": "hypershell-controller"}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: PolicyBundleReaderName, Namespace: namespace, Labels: labels},
		Rules: []rbacv1.PolicyRule{{
			APIGroups:     []string{""},
			Resources:     []string{"configmaps"},
			ResourceNames: []string{policyBundleConfigMap},
			Verbs:         []string{"get", "list", "watch"},
		}},
	}
	roles := client.RbacV1().Roles(namespace)
	existing, err := roles.Get(ctx, PolicyBundleReaderName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, err := roles.Create(ctx, role, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create role: %w", err)
		}
	case err != nil:
		return fmt.Errorf("get role: %w", err)
	case !reflect.DeepEqual(existing.Rules, role.Rules):
		updated := existing.DeepCopy()
		updated.Rules = role.Rules
		if _, err := roles.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update role: %w", err)
		}
	}

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: PolicyBundleReaderName, Namespace: namespace, Labels: labels},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: PolicyBundleReaderName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: helm.ReleaseName, Namespace: namespace}},
	}
	bindings := client.RbacV1().RoleBindings(namespace)
	existingBinding, err := bindings.Get(ctx, PolicyBundleReaderName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, err := bindings.Create(ctx, binding, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create role binding: %w", err)
		}
	case err != nil:
		return fmt.Errorf("get role binding: %w", err)
	case !reflect.DeepEqual(existingBinding.Subjects, binding.Subjects) || existingBinding.RoleRef != binding.RoleRef:
		// roleRef is immutable: replace the binding.
		if err := bindings.Delete(ctx, PolicyBundleReaderName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("replace role binding: %w", err)
		}
		if _, err := bindings.Create(ctx, binding, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("recreate role binding: %w", err)
		}
	}
	return nil
}
