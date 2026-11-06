package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testDestinationCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestDestinationCAReconciliation(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME", "corporate-ca")
	client := fake.NewSimpleClientset()
	if _, err := reconcileDestinationCABundle(ctx, client, "cp", "tenant"); err == nil {
		t.Fatal("missing configured source must fail")
	}
	source := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "corporate-ca", Namespace: "cp"}, Data: map[string]string{"ca-bundle.crt": "invalid"}}
	if _, err := client.CoreV1().ConfigMaps("cp").Create(ctx, source, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := reconcileDestinationCABundle(ctx, client, "cp", "tenant"); err == nil {
		t.Fatal("invalid certificate must fail")
	}
	source.Data["ca-bundle.crt"] = testDestinationCA(t)
	if _, err := client.CoreV1().ConfigMaps("cp").Update(ctx, source, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	name, err := reconcileDestinationCABundle(ctx, client, "cp", "tenant")
	if err != nil {
		t.Fatal(err)
	}
	cm, err := client.CoreV1().ConfigMaps("tenant").Get(ctx, name, metav1.GetOptions{})
	if err != nil || cm.Data["ca.crt"] != source.Data["ca-bundle.crt"] {
		t.Fatalf("destination CA not copied: %v", err)
	}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "openshell-gateway", Namespace: "tenant"}}
	if _, err := client.AppsV1().Deployments("tenant").Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	assertUpdate := func(want bool) {
		t.Helper()
		got, err := DestinationCATrustNeedsUpdate(ctx, client, "cp", "tenant")
		if err != nil || got != want {
			t.Fatalf("needs update = %v, %v; want %v", got, err, want)
		}
	}
	assertUpdate(true)
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "additional-ca-source", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}}}}}
	if _, err := client.AppsV1().Deployments("tenant").Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	assertUpdate(false)
	// Rotation must change the volume reference, even with no Gateway spec change.
	source.Data["ca-bundle.crt"] = testDestinationCA(t)
	if _, err := client.CoreV1().ConfigMaps("cp").Update(ctx, source, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	assertUpdate(true)
	next, err := reconcileDestinationCABundle(ctx, client, "cp", "tenant")
	if err != nil || next == name {
		t.Fatalf("rotation did not create a new generation: %q, %v", next, err)
	}
	if _, err := client.CoreV1().ConfigMaps("tenant").Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Fatal("old generation must remain mountable", err)
	}
	t.Setenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME", "")
	assertUpdate(false)
}

func TestDestinationCADisabled(t *testing.T) {
	t.Setenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME", "")
	client := fake.NewSimpleClientset()
	name, err := reconcileDestinationCABundle(context.Background(), client, "cp", "tenant")
	if err != nil || name != "" || len(client.Actions()) != 0 {
		t.Fatalf("disabled trust touched cluster: %q %v", name, err)
	}
}

func TestDestinationCACapabilitySelection(t *testing.T) {
	t.Setenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME", "corporate-ca")
	t.Setenv("GATEWAY_ADDITIONAL_CA_REQUIRED_CAPABILITY", "network/vpn")
	for _, tc := range []struct {
		name, capabilities string
		enabled            bool
	}{
		{"vpn", "provider/aws,network/vpn", true},
		{"trimmed", "provider/aws, network/vpn ,", true},
		{"public", "provider/aws,network/public", false},
		{"missing", "", false},
		{"exact match", "network/vpn-other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES", tc.capabilities)
			client := fake.NewSimpleClientset()
			// An eligible instance must read the configured source and fail if missing.
			// An ineligible instance must not access Kubernetes or propagate extra trust.
			_, err := reconcileDestinationCABundle(context.Background(), client, "cp", "tenant")
			if tc.enabled != (err != nil) {
				t.Fatalf("enabled=%v, error=%v", tc.enabled, err)
			}
			if !tc.enabled && len(client.Actions()) != 0 {
				t.Fatal("ineligible cluster accessed CA resources")
			}
		})
	}
}
