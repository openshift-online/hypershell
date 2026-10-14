package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	// agentRuntimeManagedByLabel marks every cluster resource this reconciler owns
	// so a namespace-scoped cleanup can select them and so operators can attribute
	// them to the control plane.
	agentRuntimeManagedByLabel = "app.kubernetes.io/managed-by"
	agentRuntimeManagedByValue = "hypershell-control-plane"
	// agentRuntimeIDLabel ties every owned resource back to its AgentRuntime.
	agentRuntimeIDLabel = "hypershell.openshift.io/agent-runtime-id"

	// agentServiceAccountName is the coordinator's ServiceAccount. Its token is not
	// auto-mounted (data-model.spec.md "AgentRuntime Cluster Resource Provisioning").
	agentServiceAccountName = "agent"
	// agentCronJobName is the coordinator CronJob.
	agentCronJobName = "coordinator"
	// agentDenyIngressPolicyName is the default-deny-all-ingress NetworkPolicy.
	agentDenyIngressPolicyName = "default-deny-ingress"
	// agentSecretMountRoot is where file-valued SecretSources are mounted.
	agentSecretMountRoot = "/var/run/secrets/agent"
)

// externalSecretGVR is the external-secrets.io ExternalSecret resource the
// reconciler renders one of per SecretSource. It is a CRD, so it is applied
// through the dynamic client.
var externalSecretGVR = schema.GroupVersionResource{
	Group:    "external-secrets.io",
	Version:  "v1beta1",
	Resource: "externalsecrets",
}

var dns1123Invalid = regexp.MustCompile(`[^a-z0-9-]+`)

// agentRuntimeNamespace is the deterministic Kubernetes namespace an AgentRuntime's
// resources live in. AgentRuntime carries no namespace field, so it is derived
// from the server-assigned id (a KSUID), lowercased to satisfy DNS-1123. The id is
// globally unique, so the derived namespace is too.
func agentRuntimeNamespace(ar *pb.AgentRuntime) string {
	return sanitizeDNS1123("agent-" + ar.GetMetadata().GetId())
}

// sanitizeDNS1123 lowercases and strips a value to a DNS-1123 label fragment,
// collapsing invalid runs to a single hyphen and trimming leading/trailing
// hyphens. It is used for resource names derived from user-supplied SecretSource
// names and server ids.
func sanitizeDNS1123(name string) string {
	s := dns1123Invalid.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = strings.Trim(s[:63], "-")
	}
	if s == "" {
		s = "unnamed"
	}
	return s
}

// agentRuntimeLabels are applied to every owned resource for ownership and cleanup.
func agentRuntimeLabels(ar *pb.AgentRuntime) map[string]string {
	return map[string]string{
		agentRuntimeManagedByLabel: agentRuntimeManagedByValue,
		agentRuntimeIDLabel:        ar.GetMetadata().GetId(),
	}
}

// applyNamespace creates or updates the agent namespace. Namespaces are not
// mutated beyond labels, so an existing namespace only has its labels reconciled.
func applyNamespace(ctx context.Context, kube kubernetes.Interface, name string, labels map[string]string) error {
	desired := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
	}
	existing, err := kube.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := kube.CoreV1().Namespaces().Create(ctx, desired, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create namespace %s: %w", name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get namespace %s: %w", name, err)
	}
	existing.Labels = mergeLabels(existing.Labels, labels)
	if _, err := kube.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update namespace %s: %w", name, err)
	}
	return nil
}

// applyServiceAccount creates or updates the coordinator ServiceAccount with token
// auto-mount disabled.
func applyServiceAccount(ctx context.Context, kube kubernetes.Interface, namespace string, labels map[string]string) error {
	automount := false
	desired := &corev1.ServiceAccount{
		ObjectMeta:                   metav1.ObjectMeta{Name: agentServiceAccountName, Namespace: namespace, Labels: labels},
		AutomountServiceAccountToken: &automount,
	}
	existing, err := kube.CoreV1().ServiceAccounts(namespace).Get(ctx, agentServiceAccountName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := kube.CoreV1().ServiceAccounts(namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create service account %s/%s: %w", namespace, agentServiceAccountName, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get service account %s/%s: %w", namespace, agentServiceAccountName, err)
	}
	existing.Labels = mergeLabels(existing.Labels, labels)
	existing.AutomountServiceAccountToken = &automount
	if _, err := kube.CoreV1().ServiceAccounts(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update service account %s/%s: %w", namespace, agentServiceAccountName, err)
	}
	return nil
}

// applyDenyIngressNetworkPolicy creates or updates a NetworkPolicy that selects
// every pod in the namespace and admits no ingress (default-deny ingress).
func applyDenyIngressNetworkPolicy(ctx context.Context, kube kubernetes.Interface, namespace string, labels map[string]string) error {
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: agentDenyIngressPolicyName, Namespace: namespace, Labels: labels},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			// No Ingress rules: all ingress is denied.
		},
	}
	existing, err := kube.NetworkingV1().NetworkPolicies(namespace).Get(ctx, agentDenyIngressPolicyName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := kube.NetworkingV1().NetworkPolicies(namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create network policy %s/%s: %w", namespace, agentDenyIngressPolicyName, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get network policy %s/%s: %w", namespace, agentDenyIngressPolicyName, err)
	}
	existing.Labels = mergeLabels(existing.Labels, labels)
	existing.Spec = desired.Spec
	if _, err := kube.NetworkingV1().NetworkPolicies(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update network policy %s/%s: %w", namespace, agentDenyIngressPolicyName, err)
	}
	return nil
}

// secretSourceKeyMapping is one {source_key, target_key} rename pair.
type secretSourceKeyMapping struct {
	SourceKey string `json:"source_key"`
	TargetKey string `json:"target_key"`
}

// secretName is the Kubernetes Secret an ExternalSecret syncs a SecretSource into.
// It matches the ExternalSecret name so the CronJob can reference it. A short hash
// of the original (unsanitized) name is appended so two SecretSources whose names
// sanitize to the same DNS-1123 label (for example "GitHub_Key" and "github-key")
// never collide into one ExternalSecret - which would overwrite one secret and, for
// file-valued purposes, produce duplicate CronJob volume names the API rejects.
func secretName(source *pb.SecretSource) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(source.GetName()))
	suffix := fmt.Sprintf("%08x", h.Sum32())
	base := sanitizeDNS1123(source.GetName())
	// Keep room for the "-" + 8-char suffix within the 63-char DNS-1123 limit.
	if len(base) > 63-9 {
		base = strings.Trim(base[:63-9], "-")
	}
	return base + "-" + suffix
}

// buildExternalSecret renders the ExternalSecret for one SecretSource. When the
// SecretSource carries key_mappings only those keys are extracted and renamed;
// otherwise every field at the backend path is extracted under its original name.
func buildExternalSecret(ar *pb.AgentRuntime, namespace string, source *pb.SecretSource) (*unstructured.Unstructured, error) {
	name := secretName(source)
	spec := map[string]interface{}{
		"refreshInterval": "1h",
		"secretStoreRef": map[string]interface{}{
			"name": secretStoreName(source.GetBackend()),
			"kind": "ClusterSecretStore",
		},
		"target": map[string]interface{}{
			"name":           name,
			"creationPolicy": "Owner",
		},
	}

	if km := strings.TrimSpace(source.GetKeyMappings()); km != "" && km != "null" {
		var mappings []secretSourceKeyMapping
		if err := json.Unmarshal([]byte(km), &mappings); err != nil {
			return nil, fmt.Errorf("parse key_mappings for secret source %q: %w", source.GetName(), err)
		}
		data := make([]interface{}, 0, len(mappings))
		for _, m := range mappings {
			target := m.TargetKey
			if target == "" {
				target = m.SourceKey
			}
			data = append(data, map[string]interface{}{
				"secretKey": target,
				"remoteRef": map[string]interface{}{
					"key":      source.GetPath(),
					"property": m.SourceKey,
				},
			})
		}
		spec["data"] = data
	} else {
		spec["dataFrom"] = []interface{}{
			map[string]interface{}{
				"extract": map[string]interface{}{"key": source.GetPath()},
			},
		}
	}

	labels := map[string]interface{}{}
	for k, v := range agentRuntimeLabels(ar) {
		labels[k] = v
	}

	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": externalSecretGVR.Group + "/" + externalSecretGVR.Version,
		"kind":       "ExternalSecret",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"labels":    labels,
		},
		"spec": spec,
	}}, nil
}

// secretStoreName maps a SecretSource backend to the conventional
// ClusterSecretStore name the external-secrets operator is expected to expose for
// that backend. Configuration, not code, selects the concrete backend.
func secretStoreName(backend string) string {
	switch backend {
	case "aws-secrets-manager":
		return "aws-secrets-manager"
	case "kubernetes":
		return "kubernetes"
	case "vault":
		return "vault-backend"
	default:
		return "vault-backend"
	}
}

// applyExternalSecret creates or updates one ExternalSecret through the dynamic
// client, carrying the existing resourceVersion forward on update.
func applyExternalSecret(ctx context.Context, dyn dynamic.Interface, namespace string, obj *unstructured.Unstructured) error {
	client := dyn.Resource(externalSecretGVR).Namespace(namespace)
	name := obj.GetName()
	existing, err := client.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := client.Create(ctx, obj, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create external secret %s/%s: %w", namespace, name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get external secret %s/%s: %w", namespace, name, err)
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	// Merge labels so a label another controller added to the owned ExternalSecret
	// survives the update, consistent with the typed resources' mergeLabels path.
	obj.SetLabels(mergeLabels(existing.GetLabels(), obj.GetLabels()))
	// status is an operator-owned subresource; carry it forward so a spec update
	// never clobbers the sync state the reconciler later reads for Degraded
	// detection.
	if existingStatus, found, _ := unstructured.NestedMap(existing.Object, "status"); found {
		if err := unstructured.SetNestedMap(obj.Object, existingStatus, "status"); err != nil {
			return fmt.Errorf("preserve external secret %s/%s status: %w", namespace, name, err)
		}
	}
	if _, err := client.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update external secret %s/%s: %w", namespace, name, err)
	}
	return nil
}

// externalSecretSyncFailed reports whether an ExternalSecret is in a sync-error
// state (its Ready condition is False), returning the reason/message for the
// AgentRuntime status. A freshly created ExternalSecret has no status yet and is
// treated as still syncing (not failed), so a brand-new runtime is not flapped to
// Degraded before the external-secrets operator has had a chance to reconcile.
func externalSecretSyncFailed(obj *unstructured.Unstructured) (bool, string) {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false, ""
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		if condType == "Ready" && condStatus == "False" {
			msg, _ := cond["message"].(string)
			reason, _ := cond["reason"].(string)
			if msg == "" {
				msg = reason
			}
			return true, msg
		}
	}
	return false, ""
}

// secretEnvWiring is the env vars, volumes, and volume mounts a SecretSource
// contributes to the coordinator container, derived from its purpose.
type secretEnvWiring struct {
	env          []corev1.EnvVar
	volumes      []corev1.Volume
	volumeMounts []corev1.VolumeMount
}

// wireSecretSource maps a SecretSource's purpose to the controller-generated env
// vars (and, for file-valued credentials, a mounted volume) per the SecretSource
// purpose table in data-model.spec.md. An unknown purpose contributes nothing,
// so an operator never has to spell out env var names.
func wireSecretSource(source *pb.SecretSource) secretEnvWiring {
	name := secretName(source)
	var w secretEnvWiring
	switch source.GetPurpose() {
	case "gateway-identity":
		w.env = append(w.env,
			secretEnv("GATEWAY_ENDPOINT", name, "endpoint"),
			secretEnv("OIDC_ISSUER", name, "issuer"),
			secretEnv("OIDC_CLIENT_ID", name, "client_id"),
			secretEnv("OIDC_AUDIENCE", name, "audience"),
			secretEnv("OPENSHELL_OIDC_CLIENT_SECRET", name, "client_secret"),
		)
	case "api-key":
		w.env = append(w.env, secretEnv("API_KEY", name, "api-key"))
	case "service-account-key":
		mountPath := agentSecretMountRoot + "/" + name
		w.env = append(w.env, corev1.EnvVar{Name: "SA_JSON_KEY_FILE", Value: mountPath + "/key.json"})
		w.volumes, w.volumeMounts = secretFileMount(name, mountPath)
	case "app-private-key":
		mountPath := agentSecretMountRoot + "/" + name
		w.env = append(w.env, corev1.EnvVar{Name: "APP_PRIVATE_KEY_FILE", Value: mountPath + "/key.pem"})
		w.volumes, w.volumeMounts = secretFileMount(name, mountPath)
	}
	return w
}

// secretEnv builds an env var sourced from a key in a synced Secret.
func secretEnv(envName, secret, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secret},
				Key:                  key,
			},
		},
	}
}

// secretFileMount builds the volume and mount that surface a file-valued secret in
// the coordinator container.
func secretFileMount(secret, mountPath string) ([]corev1.Volume, []corev1.VolumeMount) {
	volName := sanitizeDNS1123("secret-" + secret)
	return []corev1.Volume{{
			Name:         volName,
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: secret}},
		}}, []corev1.VolumeMount{{
			Name:      volName,
			MountPath: mountPath,
			ReadOnly:  true,
		}}
}

// buildCoordinatorCronJob renders the coordinator CronJob: the schedule from the
// AgentRuntime cron, the image from coordinator_image, the env wired from each
// SecretSource by purpose, a restricted SecurityContext, and the non-auto-mounting
// coordinator ServiceAccount.
func buildCoordinatorCronJob(ar *pb.AgentRuntime, namespace string, sources []*pb.SecretSource) *batchv1.CronJob {
	labels := agentRuntimeLabels(ar)

	var env []corev1.EnvVar
	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount
	for _, s := range sources {
		w := wireSecretSource(s)
		env = append(env, w.env...)
		volumes = append(volumes, w.volumes...)
		mounts = append(mounts, w.volumeMounts...)
	}

	automount := false
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	container := corev1.Container{
		Name:            "coordinator",
		Image:           ar.GetCoordinatorImage(),
		Env:             env,
		VolumeMounts:    mounts,
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &runAsNonRoot,
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}

	concurrency := batchv1.AllowConcurrent
	switch ar.GetConcurrencyPolicy() {
	case string(batchv1.ForbidConcurrent):
		concurrency = batchv1.ForbidConcurrent
	case string(batchv1.ReplaceConcurrent):
		concurrency = batchv1.ReplaceConcurrent
	}

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: agentCronJobName, Namespace: namespace, Labels: labels},
		Spec: batchv1.CronJobSpec{
			Schedule:          ar.GetCron(),
			ConcurrencyPolicy: concurrency,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							ServiceAccountName:           agentServiceAccountName,
							AutomountServiceAccountToken: &automount,
							RestartPolicy:                corev1.RestartPolicyNever,
							Containers:                   []corev1.Container{container},
							Volumes:                      volumes,
						},
					},
				},
			},
		},
	}
}

// applyCronJob creates or updates the coordinator CronJob.
func applyCronJob(ctx context.Context, kube kubernetes.Interface, namespace string, desired *batchv1.CronJob) error {
	existing, err := kube.BatchV1().CronJobs(namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := kube.BatchV1().CronJobs(namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create cronjob %s/%s: %w", namespace, desired.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get cronjob %s/%s: %w", namespace, desired.Name, err)
	}
	existing.Labels = mergeLabels(existing.Labels, desired.Labels)
	existing.Spec = desired.Spec
	if _, err := kube.BatchV1().CronJobs(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update cronjob %s/%s: %w", namespace, desired.Name, err)
	}
	return nil
}

// mergeLabels overlays desired labels onto existing ones without dropping labels
// set by other controllers.
func mergeLabels(existing, desired map[string]string) map[string]string {
	if existing == nil {
		existing = map[string]string{}
	}
	for k, v := range desired {
		existing[k] = v
	}
	return existing
}

// deleteAgentRuntimeNamespace deletes the agent namespace, which cascades to the
// ServiceAccount, NetworkPolicy, ExternalSecrets, and CronJob it contains. A
// missing namespace is treated as already deleted.
func deleteAgentRuntimeNamespace(ctx context.Context, kube kubernetes.Interface, namespace string) error {
	err := kube.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete namespace %s: %w", namespace, err)
	}
	return nil
}
