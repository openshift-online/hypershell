package gateway

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

// PolicyDefaultsHashNeedsUpdate lets a fleet-defaults change bypass the gateway
// spec generation gate, like DestinationCATrustNeedsUpdate (PLAT-PGP-08.04): it
// reports whether the gateway workload's pod template carries a
// policy-defaults hash other than want. A missing annotation (a gateway
// rendered before the sidecar or before the hash existed) is drift. A gateway
// with no workload yet is not: its provisioning renders the current hash.
func PolicyDefaultsHashNeedsUpdate(ctx context.Context, client kubernetes.Interface, namespace, want string) (bool, error) {
	if namespace == "" {
		return false, fmt.Errorf("gateway namespace is required for the policy defaults check")
	}
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, helm.ReleaseName, metav1.GetOptions{})
	if err == nil {
		return deployment.Spec.Template.Annotations[helm.PolicyDefaultsHashAnnotation] != want, nil
	}
	if !k8serrors.IsNotFound(err) {
		return false, fmt.Errorf("read gateway deployment: %w", err)
	}
	statefulSet, err := client.AppsV1().StatefulSets(namespace).Get(ctx, helm.ReleaseName, metav1.GetOptions{})
	if err == nil {
		return statefulSet.Spec.Template.Annotations[helm.PolicyDefaultsHashAnnotation] != want, nil
	}
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("read gateway statefulset: %w", err)
}
