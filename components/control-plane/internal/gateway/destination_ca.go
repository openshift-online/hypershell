package gateway

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// destinationCABundle reads operator-controlled roots, independently of OIDC,
// Vault and gateway mTLS trust. A configured but unavailable bundle fails closed.
func destinationCABundle(ctx context.Context, client kubernetes.Interface, namespace string) (string, string, error) {
	source := strings.TrimSpace(os.Getenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME"))
	if source == "" {
		return "", "", nil
	}
	cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, source, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("read destination CA ConfigMap %s/%s: %w", namespace, source, err)
	}
	bundle := cm.Data["ca.crt"]
	if bundle == "" {
		bundle = cm.Data["ca-bundle.crt"]
	}
	if !x509.NewCertPool().AppendCertsFromPEM([]byte(bundle)) {
		return "", "", fmt.Errorf("destination CA ConfigMap %s/%s has no PEM certificates in ca.crt or ca-bundle.crt", namespace, source)
	}
	// A new name changes the Deployment volume and triggers a gateway rollout.
	// Retain old generations until namespace cleanup so existing pods can mount them.
	digest := sha256.Sum256([]byte(bundle))
	return fmt.Sprintf("openshell-destination-ca-%x", digest[:8]), bundle, nil
}

func reconcileDestinationCABundle(ctx context.Context, client kubernetes.Interface, sourceNamespace, namespace string) (string, error) {
	name, bundle, err := destinationCABundle(ctx, client, sourceNamespace)
	if err != nil || name == "" {
		return name, err
	}
	cms := client.CoreV1().ConfigMaps(namespace)
	cm, err := cms.Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		cm = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{
			"app.kubernetes.io/managed-by": "hypershell-control-plane",
			"hypershell.redhat.io/managed": "true",
		}}, Data: map[string]string{"ca.crt": bundle}}
		_, err = cms.Create(ctx, cm, metav1.CreateOptions{})
	} else if err == nil && cm.Data["ca.crt"] != bundle {
		cm.Data = map[string]string{"ca.crt": bundle}
		_, err = cms.Update(ctx, cm, metav1.UpdateOptions{})
	}
	if err != nil {
		return "", fmt.Errorf("reconcile destination CA %s/%s: %w", namespace, name, err)
	}
	return name, nil
}

// DestinationCATrustNeedsUpdate lets operator trust changes bypass the gateway
// spec generation gate. Controller startup rechecks existing gateways.
func DestinationCATrustNeedsUpdate(ctx context.Context, client kubernetes.Interface, sourceNamespace, namespace string) (bool, error) {
	if strings.TrimSpace(os.Getenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME")) == "" {
		return false, nil
	}
	if namespace == "" {
		return false, fmt.Errorf("gateway namespace is required for destination TLS trust")
	}
	name, err := reconcileDestinationCABundle(ctx, client, sourceNamespace, namespace)
	if err != nil {
		return false, err
	}
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, "openshell-gateway", metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return name != "", nil
	}
	if err != nil {
		return false, fmt.Errorf("read gateway deployment: %w", err)
	}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "additional-ca-source" && volume.ConfigMap != nil {
			return volume.ConfigMap.Name != name, nil
		}
	}
	return name != "", nil
}
