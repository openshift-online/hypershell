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

// sanitizeDNS1123WithHash is sanitizeDNS1123 that preserves uniqueness across
// truncation: when the sanitized value exceeds the 63-char DNS-1123 limit it
// appends an 8-char hash of the full sanitized value, so two distinct inputs that
// share a 63-char prefix do not collapse to the same label. A plain prefix
// truncation would drop the disambiguating tail (for secret volume names, the
// per-source hash secretName already carries), producing duplicate pod volume
// names the CronJob API rejects.
func sanitizeDNS1123WithHash(name string) string {
	s := dns1123Invalid.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "unnamed"
	}
	if len(s) <= 63 {
		return s
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	suffix := fmt.Sprintf("%08x", h.Sum32())
	base := strings.Trim(s[:63-9], "-")
	if base == "" {
		base = "secret"
	}
	return base + "-" + suffix
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

// secretName is the Kubernetes Secret the reconciler places a SecretSource value
// into. The CronJob references it by this name. A short hash of the original
// (unsanitized) name is appended so two SecretSources whose names sanitize to the
// same DNS-1123 label (for example "GitHub_Key" and "github-key") never collide
// into one Secret - which would overwrite one value and, for file-valued purposes,
// produce duplicate CronJob volume names the API rejects.
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

// buildSecretData translates a stored SecretSource value into the Kubernetes
// Secret data map, replacing the removed ExternalSecret key mapping (ESO is no
// longer in the architecture; see control-plane.spec.md "AgentRuntime Cluster
// Resource Provisioning" and security.spec.md "Secret Store Access Is
// Single-Directional per Component"). It mirrors the semantics the ExternalSecret
// used to express, done in-process over the single value the control plane reads
// from the store. The resulting key names MUST match what the coordinator env
// wiring (wireSecretSource) references for each purpose, so the two stay
// consistent.
//
//   - With key_mappings: the stored value is a JSON object; only the named source
//     keys are extracted and renamed to their target keys (the in-process analog of
//     the ExternalSecret per-key remoteRef/property extraction).
//   - gateway-identity: the stored value is a JSON object; each field is placed
//     under its own key (endpoint, issuer, client_id, audience, client_secret), the
//     keys the coordinator env vars reference.
//   - api-key: the raw value is placed under the "api-key" key.
//   - service-account-key: the raw value is placed under "key.json" (the file the
//     coordinator mounts).
//   - app-private-key: the raw value is placed under "key.pem".
//   - any other purpose: a JSON object is expanded field by field; a non-structured
//     opaque value is placed under a single "value" key.
func buildSecretData(source *pb.SecretSource, value string) (map[string][]byte, error) {
	if km := strings.TrimSpace(source.GetKeyMappings()); km != "" && km != "null" {
		var mappings []secretSourceKeyMapping
		if err := json.Unmarshal([]byte(km), &mappings); err != nil {
			return nil, fmt.Errorf("parse key_mappings for secret source %q: %w", source.GetName(), err)
		}
		fields, err := decodeSecretObject(value)
		if err != nil {
			return nil, fmt.Errorf("secret source %q value is not a JSON object but declares key_mappings: %w", source.GetName(), err)
		}
		data := make(map[string][]byte, len(mappings))
		for _, m := range mappings {
			target := m.TargetKey
			if target == "" {
				target = m.SourceKey
			}
			fieldValue, ok := fields[m.SourceKey]
			if !ok {
				return nil, fmt.Errorf("secret source %q value has no key %q named by key_mappings", source.GetName(), m.SourceKey)
			}
			data[target] = []byte(fieldValue)
		}
		return data, nil
	}

	switch source.GetPurpose() {
	case "gateway-identity":
		fields, err := decodeSecretObject(value)
		if err != nil {
			return nil, fmt.Errorf("secret source %q (gateway-identity) value must be a JSON object: %w", source.GetName(), err)
		}
		data := make(map[string][]byte, len(fields))
		for k, v := range fields {
			data[k] = []byte(v)
		}
		return data, nil
	case "api-key":
		return map[string][]byte{"api-key": []byte(value)}, nil
	case "service-account-key":
		return map[string][]byte{"key.json": []byte(value)}, nil
	case "app-private-key":
		return map[string][]byte{"key.pem": []byte(value)}, nil
	}

	if fields, err := decodeSecretObject(value); err == nil {
		data := make(map[string][]byte, len(fields))
		for k, v := range fields {
			data[k] = []byte(v)
		}
		return data, nil
	}
	// A non-structured opaque value: place it under a single conventional key.
	return map[string][]byte{"value": []byte(value)}, nil
}

// decodeSecretObject parses a stored value as a flat JSON object of string fields.
// It returns an error when the value is not a JSON object, so a caller that
// requires structured data (key_mappings) can fail at provisioning time rather
// than place an empty or malformed Secret.
func decodeSecretObject(value string) (map[string]string, error) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, fmt.Errorf("value is not a JSON object")
	}
	var fields map[string]string
	// Deliberately do NOT wrap the json error: a decode error can echo bytes of the
	// malformed input, which here is the secret value (security.spec.md: a raw stored
	// secret value SHALL NOT appear in logs or errors).
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return nil, fmt.Errorf("value is not a decodable JSON object of string fields")
	}
	return fields, nil
}

// buildSecret renders the Kubernetes Secret for one SecretSource from the value
// the control plane read from the store. The Secret carries the ownership labels
// so a removed SecretSource's Secret can be pruned.
func buildSecret(ar *pb.AgentRuntime, namespace string, source *pb.SecretSource, value string) (*corev1.Secret, error) {
	data, err := buildSecretData(source, value)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName(source),
			Namespace: namespace,
			Labels:    agentRuntimeLabels(ar),
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}, nil
}

// applySecret creates or updates one placed Secret. Placement is one-shot per
// reconcile: the control plane reads the store and writes the Secret directly, with
// no ExternalSecret and no operator-driven resync (security.spec.md).
func applySecret(ctx context.Context, kube kubernetes.Interface, namespace string, desired *corev1.Secret) error {
	name := desired.Name
	existing, err := kube.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := kube.CoreV1().Secrets(namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create secret %s/%s: %w", namespace, name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	existing.Labels = mergeLabels(existing.Labels, desired.Labels)
	existing.Data = desired.Data
	existing.Type = desired.Type
	if _, err := kube.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret %s/%s: %w", namespace, name, err)
	}
	return nil
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

// secretEnv builds an env var sourced from a key in a placed Secret.
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
	// Hash-preserving truncation: secret already carries a per-source hash suffix
	// (secretName), so the volume name must keep it to stay unique once "secret-"
	// pushes the label past 63 chars.
	volName := sanitizeDNS1123WithHash("secret-" + secret)
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
// ServiceAccount, NetworkPolicy, placed Secrets, and CronJob it contains. A
// missing namespace is treated as already deleted.
func deleteAgentRuntimeNamespace(ctx context.Context, kube kubernetes.Interface, namespace string) error {
	err := kube.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete namespace %s: %w", namespace, err)
	}
	return nil
}
