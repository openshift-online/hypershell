package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"

	runtimeconfig "github.com/openshift-online/hypershell/components/control-plane/internal/config"
	"github.com/pelletier/go-toml/v2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

const imagePullGatewayLabel = "hypershell.redhat.io/image-pull-gateway"
const imagePullNamespaceUID = "hypershell.redhat.io/image-pull-namespace-uid"

// ReconcileSandboxImagePullRoles grants access only to explicitly selected image
// Roles. Shared workspace mode keeps every sandbox under the gateway namespace.
// Other modes need a separate namespace lifecycle contract and are rejected.
func ReconcileSandboxImagePullRoles(ctx context.Context, client kubernetes.Interface, namespace, instance, gatewayID string) error {
	roles, err := runtimeconfig.SandboxImagePullRoles()
	if err != nil {
		return err
	}
	if len(roles) == 0 {
		return DeleteSandboxImagePullRoles(ctx, client, namespace, instance, gatewayID)
	}
	if client == nil || instance == "" || gatewayID == "" {
		return fmt.Errorf("sandbox image pull grants require controller and gateway identity")
	}
	ns, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read image pull gateway namespace: %w", err)
	}
	if !IsManagedNamespace(ns, instance) || ns.UID == "" || ns.DeletionTimestamp != nil {
		return fmt.Errorf("image pull namespace is not active and owned by this controller")
	}
	cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, "openshell-gateway-config", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read sandbox image pull configuration: %w", err)
	}
	serviceAccount, err := sandboxImagePullAccount(cm.Data["gateway.toml"])
	if err != nil {
		return errors.Join(err, DeleteSandboxImagePullRoles(ctx, client, namespace, instance, gatewayID))
	}
	if _, err := client.CoreV1().ServiceAccounts(namespace).Get(ctx, serviceAccount, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("read sandbox image pull service account: %w", err)
	}
	desired := map[string]*rbacv1.RoleBinding{}
	for _, selected := range roles {
		role, err := client.RbacV1().Roles(selected.Namespace).Get(ctx, selected.Role, metav1.GetOptions{})
		if err == nil {
			err = validateImagePullRole(role)
		}
		if err != nil {
			return errors.Join(fmt.Errorf("validate image pull Role %s/%s: %w", selected.Namespace, selected.Role, err), DeleteSandboxImagePullRoles(ctx, client, namespace, instance, gatewayID))
		}
		binding := imagePullBinding(ns, instance, gatewayID, serviceAccount, selected)
		desired[binding.Namespace+"/"+binding.Name] = binding
	}
	if err := pruneImagePullBindings(ctx, client, namespace, instance, gatewayID, desired); err != nil {
		return err
	}
	for _, binding := range desired {
		api := client.RbacV1().RoleBindings(binding.Namespace)
		existing, err := api.Get(ctx, binding.Name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			if _, err := api.Create(ctx, binding, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create sandbox image pull binding: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read sandbox image pull binding: %w", err)
		}
		if !ownsImagePullBinding(existing, namespace, instance, gatewayID) || existing.Annotations[imagePullNamespaceUID] != string(ns.UID) {
			return fmt.Errorf("sandbox image pull binding name is owned by another resource")
		}
		if existing.DeletionTimestamp != nil {
			return fmt.Errorf("sandbox image pull binding deletion is pending")
		}
		if existing.RoleRef != binding.RoleRef {
			if err := deleteImagePullBinding(ctx, client, existing); err != nil {
				return err
			}
			if _, err := api.Create(ctx, binding, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("replace sandbox image pull binding: %w", err)
			}
			continue
		}
		if reflect.DeepEqual(existing.Subjects, binding.Subjects) && reflect.DeepEqual(existing.Labels, binding.Labels) && reflect.DeepEqual(existing.OwnerReferences, binding.OwnerReferences) {
			continue
		}
		binding.ResourceVersion = existing.ResourceVersion
		binding.Finalizers = existing.Finalizers
		if _, err := api.Update(ctx, binding, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update sandbox image pull binding: %w", err)
		}
	}
	return nil
}

func sandboxImagePullAccount(source string) (string, error) {
	var config struct {
		OpenShell struct {
			Drivers struct {
				Kubernetes struct {
					WorkspaceMode  string `toml:"workspace_mode"`
					ServiceAccount string `toml:"service_account_name"`
				} `toml:"kubernetes"`
			} `toml:"drivers"`
		} `toml:"openshell"`
	}
	if err := toml.Unmarshal([]byte(source), &config); err != nil {
		return "", fmt.Errorf("invalid gateway TOML for sandbox image pull configuration")
	}
	driver := config.OpenShell.Drivers.Kubernetes
	if driver.WorkspaceMode != "" && driver.WorkspaceMode != "shared" {
		return "", fmt.Errorf("sandbox image pull RoleBindings require shared workspace mode")
	}
	if driver.ServiceAccount == "" || len(validation.IsDNS1123Subdomain(driver.ServiceAccount)) != 0 {
		return "", fmt.Errorf("sandbox image pull configuration requires an explicit valid service account")
	}
	return driver.ServiceAccount, nil
}

func validateImagePullRole(role *rbacv1.Role) error {
	if len(role.Rules) == 0 {
		return fmt.Errorf("image pull Role has no rules")
	}
	for _, rule := range role.Rules {
		if !reflect.DeepEqual(rule.APIGroups, []string{"image.openshift.io"}) || !reflect.DeepEqual(rule.Resources, []string{"imagestreams/layers"}) || !reflect.DeepEqual(rule.Verbs, []string{"get"}) || len(rule.NonResourceURLs) != 0 || len(rule.ResourceNames) == 0 {
			return fmt.Errorf("image pull Role may only get named OpenShift image stream layers")
		}
		for _, name := range rule.ResourceNames {
			if name == "" || len(validation.IsDNS1123Subdomain(name)) != 0 {
				return fmt.Errorf("image pull Role requires explicit valid image stream names")
			}
		}
	}
	return nil
}

func imagePullBinding(ns *corev1.Namespace, instance, gatewayID, account string, role runtimeconfig.ImagePullRole) *rbacv1.RoleBinding {
	digest := sha256.Sum256([]byte(gatewayID + "\x00" + role.Namespace + "\x00" + role.Role))
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "hypershell-image-pull-" + hex.EncodeToString(digest[:16]), Namespace: role.Namespace,
			Labels:          map[string]string{ManagedLabel: ManagedLabelValue, InstanceLabel: instance, imagePullGatewayLabel: gatewayID},
			Annotations:     map[string]string{imagePullNamespaceUID: string(ns.UID)},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Namespace", Name: ns.Name, UID: ns.UID}}},
		RoleRef:  rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Role},
		Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: account, Namespace: ns.Name}},
	}
}

func ownsImagePullBinding(binding *rbacv1.RoleBinding, namespace, instance, gatewayID string) bool {
	if binding.Annotations[imagePullNamespaceUID] == "" {
		return false
	}
	for _, owner := range binding.OwnerReferences {
		if owner.APIVersion == "v1" && owner.Kind == "Namespace" && owner.Name == namespace && string(owner.UID) == binding.Annotations[imagePullNamespaceUID] {
			return true
		}
	}
	// Repair a removed owner reference only when all durable ownership markers agree.
	return len(binding.OwnerReferences) == 0 && binding.Labels[ManagedLabel] == ManagedLabelValue && binding.Labels[InstanceLabel] == instance && binding.Labels[imagePullGatewayLabel] == gatewayID
}

func DeleteSandboxImagePullRoles(ctx context.Context, client kubernetes.Interface, namespace, instance, gatewayID string) error {
	if client == nil || namespace == "" || instance == "" || gatewayID == "" {
		return nil
	}
	return pruneImagePullBindings(ctx, client, namespace, instance, gatewayID, nil)
}

func pruneImagePullBindings(ctx context.Context, client kubernetes.Interface, namespace, instance, gatewayID string, keep map[string]*rbacv1.RoleBinding) error {
	selector := labels.Set{ManagedLabel: ManagedLabelValue, InstanceLabel: instance, imagePullGatewayLabel: gatewayID}.AsSelector().String()
	bindings, err := client.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list owned sandbox image pull bindings: %w", err)
	}
	var errs []error
	for i := range bindings.Items {
		binding := &bindings.Items[i]
		if keep[binding.Namespace+"/"+binding.Name] != nil {
			continue
		}
		if !ownsImagePullBinding(binding, namespace, instance, gatewayID) {
			errs = append(errs, fmt.Errorf("sandbox image pull binding has conflicting ownership"))
			continue
		}
		if err := deleteImagePullBinding(ctx, client, binding); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func deleteImagePullBinding(ctx context.Context, client kubernetes.Interface, binding *rbacv1.RoleBinding) error {
	api := client.RbacV1().RoleBindings(binding.Namespace)
	options := metav1.DeleteOptions{}
	if binding.UID != "" {
		options.Preconditions = &metav1.Preconditions{UID: &binding.UID}
	}
	if err := api.Delete(ctx, binding.Name, options); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete sandbox image pull binding: %w", err)
	}
	if _, err := api.Get(ctx, binding.Name, metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
		if err != nil {
			return fmt.Errorf("verify sandbox image pull binding deletion: %w", err)
		}
		return fmt.Errorf("sandbox image pull binding deletion is pending")
	}
	return nil
}
