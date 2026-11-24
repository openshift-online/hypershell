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
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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
	// A workload without the trust volume cannot be retargeted in place: adding
	// trust needs a chart render, which waits for a spec reconciliation.
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "openshell-gateway", Namespace: "tenant"}}
	if _, err := client.AppsV1().Deployments("tenant").Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	assertRefresh := func(want DestinationCATrustResult) {
		t.Helper()
		got, err := RefreshDestinationCATrust(ctx, client, "cp", "tenant")
		if err != nil || got != want {
			t.Fatalf("refresh = %v, %v; want %v", got, err, want)
		}
	}
	assertRefresh(DestinationCATrustDeferred)
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "additional-ca-source", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}}}}}
	if _, err := client.AppsV1().Deployments("tenant").Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	assertRefresh(DestinationCATrustCurrent)
	// Rotation must change the volume reference, even with no Gateway spec change.
	source.Data["ca-bundle.crt"] = testDestinationCA(t)
	if _, err := client.CoreV1().ConfigMaps("cp").Update(ctx, source, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	assertRefresh(DestinationCATrustRotated)
	next, err := reconcileDestinationCABundle(ctx, client, "cp", "tenant")
	if err != nil || next == name {
		t.Fatalf("rotation did not create a new generation: %q, %v", next, err)
	}
	if got := destinationCAVolume(t, client); got != next {
		t.Fatalf("trust volume = %q, want %q", got, next)
	}
	if _, err := client.CoreV1().ConfigMaps("tenant").Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Fatal("old generation must remain mountable", err)
	}
	assertRefresh(DestinationCATrustCurrent)
	t.Setenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME", "")
	assertRefresh(DestinationCATrustCurrent)
}

func destinationCAVolume(t *testing.T, client *fake.Clientset) string {
	t.Helper()
	deployment, err := client.AppsV1().Deployments("tenant").Get(context.Background(), "openshell-gateway", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "additional-ca-source" && volume.ConfigMap != nil {
			return volume.ConfigMap.Name
		}
	}
	return ""
}

// Verifies: OSG-TLS-07.03, OSG-HLTH-03.06
// A trust rotation on a converged gateway retargets only the trust volume: the
// workload keeps its images, chart label and every other volume.
func TestRefreshDestinationCATrustKeepsImagesAndChart(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME", "corporate-ca")
	source := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "corporate-ca", Namespace: "cp"}, Data: map[string]string{"ca.crt": testDestinationCA(t)}}
	deployment := convergedGatewayDeployment("openshell-destination-ca-0000000000000000")
	client := fake.NewSimpleClientset(source, deployment)
	before := deployment.DeepCopy()

	got, err := RefreshDestinationCATrust(ctx, client, "cp", "tenant")
	if err != nil || got != DestinationCATrustRotated {
		t.Fatalf("refresh = %v, %v; want rotated", got, err)
	}
	after, err := client.AppsV1().Deployments("tenant").Get(ctx, "openshell-gateway", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want, err := reconcileDestinationCABundle(ctx, client, "cp", "tenant")
	if err != nil {
		t.Fatal(err)
	}
	// The only permitted change is the trust volume's ConfigMap reference.
	before.Spec.Template.Spec.Volumes[1].ConfigMap.Name = want
	if !equality.Semantic.DeepEqual(before.Spec, after.Spec) || !equality.Semantic.DeepEqual(before.Labels, after.Labels) {
		t.Fatalf("refresh changed more than the trust volume:\nbefore %+v\nafter  %+v", before.Spec.Template.Spec, after.Spec.Template.Spec)
	}
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "secrets" {
			t.Fatalf("refresh touched Helm release storage: %v", action)
		}
	}
}

// A concurrent pod template change (for example a Helm upgrade that moved the
// volume) must fail the guarded patch instead of retargeting another volume.
func TestRefreshDestinationCATrustGuardsConcurrentChange(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME", "corporate-ca")
	source := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "corporate-ca", Namespace: "cp"}, Data: map[string]string{"ca.crt": testDestinationCA(t)}}
	client := fake.NewSimpleClientset(source, convergedGatewayDeployment("openshell-destination-ca-0000000000000000"))
	client.PrependReactor("get", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		// Serve a stale read: the trust volume at index 0, not where it now is.
		stale := convergedGatewayDeployment("openshell-destination-ca-0000000000000000")
		volumes := stale.Spec.Template.Spec.Volumes
		stale.Spec.Template.Spec.Volumes = []corev1.Volume{volumes[1], volumes[0]}
		return true, stale, nil
	})
	if _, err := RefreshDestinationCATrust(ctx, client, "cp", "tenant"); err == nil {
		t.Fatal("stale pod template must fail the guarded patch")
	}
	if got := destinationCAVolume(t, client); got != "openshell-destination-ca-0000000000000000" {
		t.Fatalf("trust volume = %q after a failed patch", got)
	}
}

func convergedGatewayDeployment(trustConfigMap string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "openshell-gateway", Namespace: "tenant", Labels: map[string]string{
			"helm.sh/chart":                "openshell-0.1.8",
			"app.kubernetes.io/managed-by": "Helm",
		}},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "openshell-gateway", Image: "registry.example/openshell-gateway:v0.1.7"}},
			Volumes: []corev1.Volume{
				{Name: "gateway-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "openshell-gateway-config"}}}},
				{Name: "additional-ca-source", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: trustConfigMap}}}},
			},
		}}},
	}
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
