package reconciler

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var certificateGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}

var managedCertificateSecretNames = map[string]struct{}{
	"openshell-server-tls":           {},
	"openshell-client-tls":           {},
	"openshell-server-external-tls":  {},
	"openshell-gateway-external-tls": {},
}

type oidcClientInspector interface {
	GatewayClientSynced(ctx context.Context, clientID string) (bool, error)
}

type certificateHealth struct {
	Valid, Expired, Unavailable int
	DaysToExpiryMin             *float64
}

func evaluateCertificateHealth(materials [][]byte, now time.Time) certificateHealth {
	result := certificateHealth{}
	for _, material := range materials {
		block, _ := pem.Decode(material)
		if block == nil || block.Type != "CERTIFICATE" {
			result.Unavailable++
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			result.Unavailable++
			continue
		}
		if !cert.NotAfter.After(now) {
			result.Expired++
			continue
		}
		result.Valid++
		days := cert.NotAfter.Sub(now).Hours() / 24
		if math.IsNaN(days) || math.IsInf(days, 0) {
			continue
		}
		if result.DaysToExpiryMin == nil || days < *result.DaysToExpiryMin {
			value := days
			result.DaysToExpiryMin = &value
		}
	}
	return result
}

func readySandboxCount(sandboxes []*unstructured.Unstructured, eligible map[string]struct{}) int {
	ready := 0
	for _, sandbox := range sandboxes {
		if sandbox == nil {
			continue
		}
		if _, ok := eligible[sandbox.GetNamespace()]; !ok {
			continue
		}
		conditions, found, err := unstructured.NestedSlice(sandbox.Object, "status", "conditions")
		if err != nil || !found {
			continue
		}
		for _, raw := range conditions {
			condition, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			conditionType, _, _ := unstructured.NestedString(condition, "type")
			conditionStatus, _, _ := unstructured.NestedString(condition, "status")
			observedGeneration, hasObservedGeneration, _ := unstructured.NestedInt64(condition, "observedGeneration")
			if conditionType == "Ready" && conditionStatus == "True" && hasObservedGeneration && observedGeneration == sandbox.GetGeneration() {
				ready++
				break
			}
		}
	}
	return ready
}

func desiredOIDCClientIDs(gateways []*pb.Gateway) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(gateways))
	for _, item := range gateways {
		if item == nil || item.Oidc == nil {
			continue
		}
		var config gateway.OIDCConfig
		if err := json.Unmarshal([]byte(item.GetOidc()), &config); err != nil {
			continue
		}
		clientID := config.ClientID
		if clientID == "" {
			clientID = config.Audience
		}
		if clientID == "" {
			continue
		}
		if _, exists := seen[clientID]; exists {
			continue
		}
		seen[clientID] = struct{}{}
		out = append(out, clientID)
	}
	return out
}

func (r *SandboxAttentionReconciler) listCertificateMaterials(ctx context.Context, eligible map[string]struct{}) ([][]byte, error) {
	certificates, err := r.dynamicClient.Resource(certificateGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list managed Certificates: %w", err)
	}
	secrets, err := r.client.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list TLS secrets: %w", err)
	}
	return selectCertificateMaterials(secrets.Items, certificates.Items, eligible), nil
}

func selectCertificateMaterials(secrets []corev1.Secret, certificates []unstructured.Unstructured, eligible map[string]struct{}) [][]byte {
	secretMaterials := make(map[string][]byte)

	for i := range secrets {
		secret := &secrets[i]
		if secret.Type != corev1.SecretTypeTLS {
			continue
		}
		if _, ok := eligible[secret.Namespace]; !ok {
			continue
		}
		if _, managed := managedCertificateSecretNames[secret.Name]; !managed {
			continue
		}
		secretMaterials[secret.Namespace+"\x00"+secret.Name] = secret.Data[corev1.TLSCertKey]
	}
	materials := make([][]byte, 0, len(certificates))
	seenIdentities := make(map[string]struct{})
	for i := range certificates {
		certificate := &certificates[i]
		if _, ok := eligible[certificate.GetNamespace()]; !ok {
			continue
		}
		secretName, _, _ := unstructured.NestedString(certificate.Object, "spec", "secretName")
		if _, managed := managedCertificateSecretNames[secretName]; !managed {
			continue
		}
		identity := certificate.GetNamespace() + "\x00" + secretName
		if _, seen := seenIdentities[identity]; seen {
			continue
		}
		seenIdentities[identity] = struct{}{}
		materials = append(materials, secretMaterials[identity])
	}
	return materials
}
