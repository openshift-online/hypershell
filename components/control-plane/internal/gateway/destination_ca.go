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
// userBundle is the gateway's API-validated additional_ca_bundle (empty when the
// gateway declares no user trust). The two tiers compose per the merge policy
// (destinationCAMergeMode): by default the effective bundle concatenates the user
// and operator PEMs, so fleet-wide operator trust keeps applying alongside
// user-authored trust; "user-only" makes a user bundle replace the operator tier
// for that gateway. Gateways without a user bundle always take the operator tier.
func destinationCABundle(ctx context.Context, client kubernetes.Interface, namespace, userBundle string) (string, string, error) {
	operatorBundle, err := operatorDestinationCABundle(ctx, client, namespace)
	if err != nil {
		return "", "", err
	}
	// Validate the user tier on its own, before composition: a valid operator
	// bundle must not mask invalid user material inside the merged PEM (the API
	// boundary already rejects it; this is defense in depth for direct writes).
	if userBundle != "" && !x509.NewCertPool().AppendCertsFromPEM([]byte(userBundle)) {
		return "", "", fmt.Errorf("gateway user destination CA bundle has no PEM certificates")
	}
	bundle := operatorBundle
	switch {
	case userBundle == "":
		// operator tier only (or nothing)
	case operatorBundle == "" || destinationCAMergeMode() == "user-only":
		bundle = userBundle
	default:
		bundle = userBundle + "\n" + operatorBundle
	}
	if bundle == "" {
		return "", "", nil
	}
	if !x509.NewCertPool().AppendCertsFromPEM([]byte(bundle)) {
		return "", "", fmt.Errorf("destination CA bundle for %s has no PEM certificates", namespace)
	}
	// A new name changes the Deployment volume and triggers a gateway rollout.
	// Retain old generations until namespace cleanup so existing pods can mount them.
	digest := sha256.Sum256([]byte(bundle))
	return fmt.Sprintf("openshell-destination-ca-%x", digest[:8]), bundle, nil
}

// operatorDestinationCABundle reads the operator-configured hub-wide trust
// source. Empty name means the operator tier is not configured (or the cluster
// lacks the required capability), in which case no operator roots apply.
func operatorDestinationCABundle(ctx context.Context, client kubernetes.Interface, namespace string) (string, error) {
	source := destinationCASourceName()
	if source == "" {
		return "", nil
	}
	cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, source, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read destination CA ConfigMap %s/%s: %w", namespace, source, err)
	}
	bundle := cm.Data["ca.crt"]
	if bundle == "" {
		bundle = cm.Data["ca-bundle.crt"]
	}
	if !x509.NewCertPool().AppendCertsFromPEM([]byte(bundle)) {
		return "", fmt.Errorf("destination CA ConfigMap %s/%s has no PEM certificates in ca.crt or ca-bundle.crt", namespace, source)
	}
	return bundle, nil
}

// destinationCAMergeMode is the hub-wide composition policy for the user and
// operator destination-CA tiers: "merge" (default) concatenates both bundles
// when a gateway declares user trust; "user-only" makes the user bundle replace
// the operator tier for that gateway.
func destinationCAMergeMode() string {
	if mode := strings.TrimSpace(os.Getenv("GATEWAY_ADDITIONAL_CA_MERGE_MODE")); mode != "" {
		return mode
	}
	return "merge"
}

func reconcileDestinationCABundle(ctx context.Context, client kubernetes.Interface, sourceNamespace, namespace, userBundle string) (string, error) {
	name, bundle, err := destinationCABundle(ctx, client, sourceNamespace, userBundle)
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

// DestinationCATrustNeedsUpdate lets operator or user trust changes bypass the
// gateway spec generation gate. Controller startup rechecks existing gateways.
// userBundle is the gateway's additional_ca_bundle; a change to it changes the
// content-addressed ConfigMap name, which this check detects through the
// Deployment volume comparison below.
func DestinationCATrustNeedsUpdate(ctx context.Context, client kubernetes.Interface, sourceNamespace, namespace, userBundle string) (bool, error) {
	if destinationCASourceName() == "" && userBundle == "" {
		return false, nil
	}
	if namespace == "" {
		return false, fmt.Errorf("gateway namespace is required for destination TLS trust")
	}
	name, err := reconcileDestinationCABundle(ctx, client, sourceNamespace, namespace, userBundle)
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

// destinationCASourceName applies an optional operator capability selector using
// the same capability declaration that the controller registers with the API.
func destinationCASourceName() string {
	required := strings.TrimSpace(os.Getenv("GATEWAY_ADDITIONAL_CA_REQUIRED_CAPABILITY"))
	if required != "" {
		matched := false
		for _, capability := range strings.Split(os.Getenv("HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES"), ",") {
			if strings.TrimSpace(capability) == required {
				matched = true
				break
			}
		}
		if !matched {
			return ""
		}
	}
	return strings.TrimSpace(os.Getenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME"))
}
