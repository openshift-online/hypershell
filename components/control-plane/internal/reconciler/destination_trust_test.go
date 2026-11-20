package reconciler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func testTrustCA(t *testing.T) string {
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

// recordingHelm returns a Helm client whose binary only records that it ran.
func recordingHelm(t *testing.T) (*helm.ShellClient, string) {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := filepath.Join(dir, "helm")
	body := "#!/bin/sh\necho \"$@\" >> '" + calls + "'\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return &helm.ShellClient{HelmBinary: script, ChartPath: filepath.Join(dir, "chart")}, calls
}

// Verifies: OSG-TLS-07.03, OSG-TLS-07.05, OSG-HLTH-03.06
// A destination-CA change on a converged gateway must not upgrade it: the
// reconciler retargets the trust volume in place and never runs Helm, so the
// gateway keeps its images and chart version. Before the fix this event fell
// through the convergence gate into a full Helm upgrade with the controller's
// current images and chart.
func TestGatewayReconciler_Handle_ConvergedTrustChangeKeepsImagesAndChart(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME", "corporate-ca")
	t.Setenv("GATEWAY_ADDITIONAL_CA_REQUIRED_CAPABILITY", "")

	for _, tc := range []struct {
		name        string
		trustVolume bool
		wantVolume  bool
	}{
		// Rotation: the existing trust volume is retargeted to the new generation.
		{name: "rotation", trustVolume: true, wantVolume: true},
		// First enablement needs a chart render; it waits for a spec reconcile.
		{name: "first enablement", trustVolume: false, wantVolume: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const namespace = "openshell-0011223344556677"
			const oldImage = "registry.example/openshell-gateway:v0.1.7"
			const oldChart = "openshell-0.1.7"
			const oldTrust = "openshell-destination-ca-0000000000000000"

			volumes := []corev1.Volume{{Name: "gateway-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "openshell-gateway-config"}}}}}
			if tc.trustVolume {
				volumes = append(volumes, corev1.Volume{Name: "additional-ca-source", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: oldTrust}}}})
			}
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "openshell-gateway", Namespace: namespace, Labels: map[string]string{"helm.sh/chart": oldChart}},
				Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"helm.sh/chart": oldChart}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "openshell-gateway", Image: oldImage}},
						Volumes:    volumes,
					},
				}},
			}
			source := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "corporate-ca", Namespace: "hypershell-system"}, Data: map[string]string{"ca.crt": testTrustCA(t)}}
			clientset := fake.NewSimpleClientset(source, deployment)

			gatewayID := "gw-trust"
			mockKC := newMockKeycloakAdminServer(t, gatewayID, "uuid-trust", map[string]interface{}{
				"id":         "uuid-trust",
				"clientId":   gatewayID,
				"attributes": map[string]interface{}{"oauth2.device.authorization.grant.enabled": "true"},
			})
			helmClient, helmCalls := recordingHelm(t)
			r := &GatewayReconciler{
				active:                make(map[string]struct{}),
				dynamicClient:         fakedynamic.NewSimpleDynamicClient(runtime.NewScheme()),
				helmClient:            helmClient,
				controlPlaneNamespace: "hypershell-system",
				keycloakClient:        keycloak.NewClient(mockKC.server.URL, mockKC.realm, mockKC.adminClient, mockKC.adminSecret),
				trustClientOverride:   clientset,
			}

			phase := "Running"
			obsGen := int64(3)
			event := watcher.Event[*pb.Gateway]{
				Type:       watcher.EventUpdated,
				ResourceID: gatewayID,
				Resource: &pb.Gateway{
					Metadata:           &pb.ObjectReference{Id: gatewayID},
					Name:               "my-gateway",
					Namespace:          namespace,
					Phase:              &phase,
					Generation:         3,
					ObservedGeneration: &obsGen,
				},
			}
			if err := r.Handle(ctx, event); err != nil {
				t.Fatalf("Handle() = %v", err)
			}

			if _, err := os.Stat(helmCalls); err == nil {
				calls, _ := os.ReadFile(helmCalls)
				t.Fatalf("trust change ran helm: %s", calls)
			}
			after, err := clientset.AppsV1().Deployments(namespace).Get(ctx, "openshell-gateway", metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := after.Spec.Template.Spec.Containers[0].Image; got != oldImage {
				t.Errorf("gateway image = %q, want %q", got, oldImage)
			}
			if after.Labels["helm.sh/chart"] != oldChart || after.Spec.Template.Labels["helm.sh/chart"] != oldChart {
				t.Errorf("chart labels = %q/%q, want %q", after.Labels["helm.sh/chart"], after.Spec.Template.Labels["helm.sh/chart"], oldChart)
			}
			trust := ""
			for _, volume := range after.Spec.Template.Spec.Volumes {
				if volume.Name == "additional-ca-source" {
					trust = volume.ConfigMap.Name
				}
			}
			switch {
			case tc.wantVolume && (trust == "" || trust == oldTrust):
				t.Errorf("trust volume = %q, want the new generation", trust)
			case !tc.wantVolume && trust != "":
				t.Errorf("trust volume added in place: %q", trust)
			}
		})
	}
}
