package reconciler

import (
	"bytes"
	"context"
	"crypto/sha256"
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

var routeGVR = schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}

const consoleCertificateRouteName = "openshell-console"

var managedCertificateSecretNames = map[string]struct{}{
	"openshell-server-tls":           {},
	"openshell-client-tls":           {},
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

func readySandboxCount(pods []*corev1.Pod, sandboxes []*unstructured.Unstructured, eligible map[string]struct{}) int {
	provisioned := make(map[string]struct{}, len(sandboxes))
	for _, sandbox := range sandboxes {
		if sandbox == nil {
			continue
		}
		if _, ok := eligible[sandbox.GetNamespace()]; !ok {
			continue
		}
		provisioned[sandbox.GetNamespace()+"\x00"+sandbox.GetName()] = struct{}{}
	}
	ready := make(map[string]struct{})
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		var sandboxName string
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "Sandbox" {
				sandboxName = owner.Name
				break
			}
		}
		key := pod.Namespace + "\x00" + sandboxName
		if sandboxName == "" {
			continue
		}
		if _, ok := provisioned[key]; !ok {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				ready[key] = struct{}{}
				break
			}
		}
	}
	return len(ready)
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
	secrets, err := r.client.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list TLS secrets: %w", err)
	}
	routes, err := r.dynamicClient.Resource(routeGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// Kubernetes clusters without OpenShift Routes legitimately do not expose
		// this API; their certificate population is secret-backed only.
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return selectCertificateMaterials(secrets.Items, nil, eligible), nil
		}
		return nil, fmt.Errorf("list console Routes: %w", err)
	}
	return selectCertificateMaterials(secrets.Items, routes.Items, eligible), nil
}

func selectCertificateMaterials(secrets []corev1.Secret, routes []unstructured.Unstructured, eligible map[string]struct{}) [][]byte {
	materials := make([][]byte, 0)
	seen := make(map[[sha256.Size]byte]struct{})
	appendMaterial := func(material []byte) {
		// Each missing expected value is an independent unavailable certificate.
		// Only non-empty material can safely identify the same logical certificate
		// represented by both a Secret and an OpenShift Route.
		if len(bytes.TrimSpace(material)) == 0 {
			materials = append(materials, nil)
			return
		}
		identity := certificateMaterialIdentity(material)
		if _, exists := seen[identity]; exists {
			return
		}
		seen[identity] = struct{}{}
		materials = append(materials, material)
	}

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
		appendMaterial(secret.Data[corev1.TLSCertKey])
	}
	for i := range routes {
		route := &routes[i]
		if _, ok := eligible[route.GetNamespace()]; !ok {
			continue
		}
		if route.GetName() != consoleCertificateRouteName {
			continue
		}
		termination, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "termination")
		if termination == "" {
			continue
		}
		certificate, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "certificate")
		appendMaterial([]byte(certificate))
	}
	return materials
}

func certificateMaterialIdentity(material []byte) [sha256.Size]byte {
	if block, _ := pem.Decode(material); block != nil && block.Type == "CERTIFICATE" {
		return sha256.Sum256(block.Bytes)
	}
	return sha256.Sum256(bytes.TrimSpace(material))
}
