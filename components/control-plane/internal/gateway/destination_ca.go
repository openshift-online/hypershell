package gateway

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// destinationCABundle reads operator-controlled roots, independently of OIDC,
// Vault and gateway mTLS trust. A configured but unavailable bundle fails closed.
func destinationCABundle(ctx context.Context, client kubernetes.Interface, namespace string) (string, string, error) {
	source := destinationCASourceName()
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

// DestinationCATrustResult reports what RefreshDestinationCATrust did for a
// converged gateway.
type DestinationCATrustResult int

const (
	// DestinationCATrustCurrent means the workload already mounts the desired
	// trust generation, or destination trust is not configured.
	DestinationCATrustCurrent DestinationCATrustResult = iota
	// DestinationCATrustRotated means the workload's trust volume now references
	// the new content-addressed generation. Only that reference changed, so the
	// gateway rolls out with its existing images and chart.
	DestinationCATrustRotated
	// DestinationCATrustDeferred means the workload has no trust volume to
	// retarget (trust was first enabled after the gateway converged, or the
	// workload is missing). Adding trust needs a chart render, which happens on
	// the gateway's next spec reconciliation, never as a side effect of a trust
	// change.
	DestinationCATrustDeferred
)

// gatewayDeploymentName is the gateway workload the chart renders for the
// openshell-gateway release.
const gatewayDeploymentName = "openshell-gateway"

// additionalCAVolumeName is the chart's pod volume for destination trust.
const additionalCAVolumeName = "additional-ca-source"

// RefreshDestinationCATrust converges destination trust on a gateway whose
// spec has converged, without a Helm upgrade (OSG-TLS-07.03, OSG-HLTH-03.06).
//
// A Helm upgrade here would render the controller's current chart and images,
// so a trust rotation would upgrade every gateway in the fleet. The chart uses
// the trust ConfigMap name in exactly one place, the additional-ca-source pod
// volume; every other reference only tests whether trust is enabled. Retargeting
// that one volume is therefore the whole difference a same-chart, same-image
// render would apply, and it rolls the gateway with nothing else changed. The
// release's stored values keep the old name until the next spec reconciliation
// renders the current one, and Helm's three-way merge accepts the live value.
func RefreshDestinationCATrust(ctx context.Context, client kubernetes.Interface, sourceNamespace, namespace string) (DestinationCATrustResult, error) {
	if destinationCASourceName() == "" {
		return DestinationCATrustCurrent, nil
	}
	if namespace == "" {
		return DestinationCATrustCurrent, fmt.Errorf("gateway namespace is required for destination TLS trust")
	}
	name, err := reconcileDestinationCABundle(ctx, client, sourceNamespace, namespace)
	if err != nil {
		return DestinationCATrustCurrent, err
	}
	deployments := client.AppsV1().Deployments(namespace)
	deployment, err := deployments.Get(ctx, gatewayDeploymentName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return DestinationCATrustDeferred, nil
	}
	if err != nil {
		return DestinationCATrustCurrent, fmt.Errorf("read gateway deployment: %w", err)
	}
	for i, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != additionalCAVolumeName || volume.ConfigMap == nil {
			continue
		}
		if volume.ConfigMap.Name == name {
			return DestinationCATrustCurrent, nil
		}
		// The test operations make the patch fail rather than retarget a
		// different volume if the pod template changed since the read.
		volumePath := fmt.Sprintf("/spec/template/spec/volumes/%d", i)
		patch, err := json.Marshal([]map[string]string{
			{"op": "test", "path": volumePath + "/name", "value": additionalCAVolumeName},
			{"op": "test", "path": volumePath + "/configMap/name", "value": volume.ConfigMap.Name},
			{"op": "replace", "path": volumePath + "/configMap/name", "value": name},
		})
		if err != nil {
			return DestinationCATrustCurrent, fmt.Errorf("encode destination CA volume patch: %w", err)
		}
		if _, err := deployments.Patch(ctx, gatewayDeploymentName, types.JSONPatchType, patch,
			metav1.PatchOptions{FieldManager: "hypershell-control-plane"}); err != nil {
			return DestinationCATrustCurrent, fmt.Errorf("retarget destination CA volume of %s/%s to %s: %w", namespace, gatewayDeploymentName, name, err)
		}
		return DestinationCATrustRotated, nil
	}
	return DestinationCATrustDeferred, nil
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
