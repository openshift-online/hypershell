package gateway

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Names of the quota objects the control plane manages in each gateway
// namespace. Kept stable so update-or-create and delete-when-empty always
// address the same objects across reconciliations.
const (
	GatewayResourceQuotaName = "hypershell-gateway-quota"
	GatewayLimitRangeName    = "hypershell-gateway-limits"
)

// QuotaConfig is the control-plane-side translation of a GatewayProfile. It
// carries the namespace-level ResourceQuota totals and the container-level
// LimitRange defaults/maxima the reconciler enforces on a gateway namespace.
// String fields are Kubernetes resource quantities ("2", "500m", "8Gi"); an
// empty string means "not set". Count fields are non-negative; zero means
// "not set". A nil *QuotaConfig means the gateway has no profile (legacy),
// which reconciles toward absence of both objects.
type QuotaConfig struct {
	CPURequestTotal       string
	CPULimitTotal         string
	MemoryRequestTotal    string
	MemoryLimitTotal      string
	EphemeralStorageTotal string
	PodCount              int32
	PVCCount              int32

	ContainerCPURequestDefault    string
	ContainerCPULimitMax          string
	ContainerMemoryRequestDefault string
	ContainerMemoryLimitMax       string
}

// ReconcileNamespaceQuota drives the namespace's managed ResourceQuota and
// LimitRange toward the desired state derived from quota. It is idempotent:
// create when absent and desired is non-empty, update when the spec diverges,
// no-op when it matches, and delete the managed object when the desired state
// is empty (nil quota, or a profile that omits the relevant fields). Only
// objects carrying the managed label are ever deleted.
func ReconcileNamespaceQuota(ctx context.Context, clientset kubernetes.Interface, namespace string, quota *QuotaConfig) error {
	if err := reconcileResourceQuota(ctx, clientset, namespace, quota); err != nil {
		return err
	}
	return reconcileLimitRange(ctx, clientset, namespace, quota)
}

func reconcileResourceQuota(ctx context.Context, clientset kubernetes.Interface, namespace string, quota *QuotaConfig) error {
	desired, nonEmpty, err := desiredResourceQuotaSpec(quota)
	if err != nil {
		return fmt.Errorf("build ResourceQuota spec for %s: %w", namespace, err)
	}

	client := clientset.CoreV1().ResourceQuotas(namespace)
	existing, getErr := client.Get(ctx, GatewayResourceQuotaName, metav1.GetOptions{})
	if getErr != nil {
		if !k8serrors.IsNotFound(getErr) {
			return fmt.Errorf("get ResourceQuota %s in %s: %w", GatewayResourceQuotaName, namespace, getErr)
		}
		if !nonEmpty {
			return nil
		}
		rq := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GatewayResourceQuotaName,
				Namespace: namespace,
				Labels:    managedResourceLabels(),
			},
			Spec: corev1.ResourceQuotaSpec{Hard: desired},
		}
		if _, err := client.Create(ctx, rq, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create ResourceQuota %s in %s: %w", GatewayResourceQuotaName, namespace, err)
		}
		log.Printf("INFO created ResourceQuota %s in namespace %s", GatewayResourceQuotaName, namespace)
		return nil
	}

	if !nonEmpty {
		if !isManagedObject(existing.Labels) {
			return nil
		}
		if err := client.Delete(ctx, GatewayResourceQuotaName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete ResourceQuota %s in %s: %w", GatewayResourceQuotaName, namespace, err)
		}
		log.Printf("INFO deleted ResourceQuota %s in namespace %s", GatewayResourceQuotaName, namespace)
		return nil
	}

	if apiequality.Semantic.DeepEqual(existing.Spec.Hard, desired) {
		return nil
	}
	existing.Spec.Hard = desired
	if existing.Labels == nil {
		existing.Labels = managedResourceLabels()
	}
	if _, err := client.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update ResourceQuota %s in %s: %w", GatewayResourceQuotaName, namespace, err)
	}
	log.Printf("INFO updated ResourceQuota %s in namespace %s", GatewayResourceQuotaName, namespace)
	return nil
}

func reconcileLimitRange(ctx context.Context, clientset kubernetes.Interface, namespace string, quota *QuotaConfig) error {
	desired, nonEmpty, err := desiredLimitRangeItem(quota)
	if err != nil {
		return fmt.Errorf("build LimitRange spec for %s: %w", namespace, err)
	}

	client := clientset.CoreV1().LimitRanges(namespace)
	existing, getErr := client.Get(ctx, GatewayLimitRangeName, metav1.GetOptions{})
	if getErr != nil {
		if !k8serrors.IsNotFound(getErr) {
			return fmt.Errorf("get LimitRange %s in %s: %w", GatewayLimitRangeName, namespace, getErr)
		}
		if !nonEmpty {
			return nil
		}
		lr := &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GatewayLimitRangeName,
				Namespace: namespace,
				Labels:    managedResourceLabels(),
			},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{desired}},
		}
		if _, err := client.Create(ctx, lr, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create LimitRange %s in %s: %w", GatewayLimitRangeName, namespace, err)
		}
		log.Printf("INFO created LimitRange %s in namespace %s", GatewayLimitRangeName, namespace)
		return nil
	}

	if !nonEmpty {
		if !isManagedObject(existing.Labels) {
			return nil
		}
		if err := client.Delete(ctx, GatewayLimitRangeName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete LimitRange %s in %s: %w", GatewayLimitRangeName, namespace, err)
		}
		log.Printf("INFO deleted LimitRange %s in namespace %s", GatewayLimitRangeName, namespace)
		return nil
	}

	desiredLimits := []corev1.LimitRangeItem{desired}
	if apiequality.Semantic.DeepEqual(existing.Spec.Limits, desiredLimits) {
		return nil
	}
	existing.Spec.Limits = desiredLimits
	if existing.Labels == nil {
		existing.Labels = managedResourceLabels()
	}
	if _, err := client.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update LimitRange %s in %s: %w", GatewayLimitRangeName, namespace, err)
	}
	log.Printf("INFO updated LimitRange %s in namespace %s", GatewayLimitRangeName, namespace)
	return nil
}

// desiredResourceQuotaSpec builds the ResourceQuota hard map from quota,
// omitting empty/zero fields. It reports whether any field is set; an empty
// map means no ResourceQuota should exist.
func desiredResourceQuotaSpec(quota *QuotaConfig) (corev1.ResourceList, bool, error) {
	hard := corev1.ResourceList{}
	if quota == nil {
		return hard, false, nil
	}

	quantities := []struct {
		name  corev1.ResourceName
		value string
	}{
		{corev1.ResourceRequestsCPU, quota.CPURequestTotal},
		{corev1.ResourceLimitsCPU, quota.CPULimitTotal},
		{corev1.ResourceRequestsMemory, quota.MemoryRequestTotal},
		{corev1.ResourceLimitsMemory, quota.MemoryLimitTotal},
		{corev1.ResourceRequestsEphemeralStorage, quota.EphemeralStorageTotal},
	}
	for _, q := range quantities {
		if q.value == "" {
			continue
		}
		parsed, err := resource.ParseQuantity(q.value)
		if err != nil {
			return nil, false, fmt.Errorf("invalid quantity %q for %s: %w", q.value, q.name, err)
		}
		hard[q.name] = parsed
	}
	if quota.PodCount > 0 {
		hard[corev1.ResourcePods] = *resource.NewQuantity(int64(quota.PodCount), resource.DecimalSI)
	}
	if quota.PVCCount > 0 {
		hard[corev1.ResourcePersistentVolumeClaims] = *resource.NewQuantity(int64(quota.PVCCount), resource.DecimalSI)
	}

	return hard, len(hard) > 0, nil
}

// desiredLimitRangeItem builds the Container LimitRangeItem from quota,
// omitting empty fields. It reports whether any container-level field is set;
// when false no LimitRange should exist.
func desiredLimitRangeItem(quota *QuotaConfig) (corev1.LimitRangeItem, bool, error) {
	item := corev1.LimitRangeItem{
		Type:           corev1.LimitTypeContainer,
		DefaultRequest: corev1.ResourceList{},
		Max:            corev1.ResourceList{},
	}
	if quota == nil {
		return item, false, nil
	}

	defaults := []struct {
		name  corev1.ResourceName
		value string
	}{
		{corev1.ResourceCPU, quota.ContainerCPURequestDefault},
		{corev1.ResourceMemory, quota.ContainerMemoryRequestDefault},
	}
	for _, d := range defaults {
		if d.value == "" {
			continue
		}
		parsed, err := resource.ParseQuantity(d.value)
		if err != nil {
			return item, false, fmt.Errorf("invalid defaultRequest quantity %q for %s: %w", d.value, d.name, err)
		}
		item.DefaultRequest[d.name] = parsed
	}

	maxima := []struct {
		name  corev1.ResourceName
		value string
	}{
		{corev1.ResourceCPU, quota.ContainerCPULimitMax},
		{corev1.ResourceMemory, quota.ContainerMemoryLimitMax},
	}
	for _, m := range maxima {
		if m.value == "" {
			continue
		}
		parsed, err := resource.ParseQuantity(m.value)
		if err != nil {
			return item, false, fmt.Errorf("invalid max quantity %q for %s: %w", m.value, m.name, err)
		}
		item.Max[m.name] = parsed
	}

	nonEmpty := len(item.DefaultRequest) > 0 || len(item.Max) > 0
	// Drop empty sub-maps so a match against a freshly-decoded object (which
	// leaves nil maps) compares equal and does not trigger spurious updates.
	if len(item.DefaultRequest) == 0 {
		item.DefaultRequest = nil
	}
	if len(item.Max) == 0 {
		item.Max = nil
	}
	return item, nonEmpty, nil
}

func managedResourceLabels() map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue,
		ManagedLabel:   ManagedLabelValue,
	}
}

func isManagedObject(labels map[string]string) bool {
	return labels[ManagedLabel] == ManagedLabelValue
}
