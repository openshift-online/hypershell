package reconciler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEvaluateCertificateHealth(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	health := evaluateCertificateHealth([][]byte{
		testCertificatePEM(t, now.Add(5*24*time.Hour)),
		testCertificatePEM(t, now.Add(20*24*time.Hour)),
		testCertificatePEM(t, now.Add(-time.Hour)),
		[]byte("not a certificate"),
	}, now)
	if health.Valid != 2 || health.Expired != 1 || health.Unavailable != 1 {
		t.Fatalf("health = %+v", health)
	}
	if health.DaysToExpiryMin == nil || *health.DaysToExpiryMin != 5 {
		t.Fatalf("minimum = %v, want 5", health.DaysToExpiryMin)
	}
}

func TestSelectCertificateMaterialsUsesConfiguredSecretIdentities(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	managed := testCertificatePEM(t, now.Add(5*24*time.Hour))
	unrelated := testCertificatePEM(t, now.Add(30*24*time.Hour))
	secrets := []corev1.Secret{
		{ObjectMeta: metav1.ObjectMeta{Name: "openshell-server-tls", Namespace: "openshell-one"}, Type: corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: managed}},
		{ObjectMeta: metav1.ObjectMeta{Name: "openshell-server-external-tls", Namespace: "openshell-one"}, Type: corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: unrelated}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unrelated-workload-tls", Namespace: "openshell-one"}, Type: corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: unrelated}},
		{ObjectMeta: metav1.ObjectMeta{Name: "openshell-server-tls", Namespace: "other"}, Type: corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: unrelated}},
	}
	dedicated := true
	gateways := []*pb.Gateway{
		{Namespace: "openshell-one", DedicatedRoute: &dedicated},
		{Namespace: "openshell-one"}, // Duplicate namespaces must not duplicate inventory.
		{Namespace: "other", DedicatedRoute: &dedicated},
	}

	expected := expectedCertificateSecretIdentities(gateways, map[string]struct{}{"openshell-one": {}})
	materials := selectCertificateMaterials(secrets, expected)
	if len(materials) != 3 {
		t.Fatalf("materials = %d, want three configured Secret identities", len(materials))
	}
	health := evaluateCertificateHealth(materials, now)
	if health.Valid != 1 || health.Expired != 0 || health.Unavailable != 2 {
		t.Fatalf("health = %+v, want one valid and two unavailable configured certificates", health)
	}
}

func TestExpectedCertificateSecretIdentitiesFollowGatewayConfiguration(t *testing.T) {
	dedicated := true
	tests := []struct {
		name    string
		gateway *pb.Gateway
		want    int
	}{
		{name: "default", gateway: &pb.Gateway{Namespace: "openshell-one"}, want: 2},
		{name: "dedicated", gateway: &pb.Gateway{Namespace: "openshell-one", DedicatedRoute: &dedicated}, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expectedCertificateSecretIdentities(
				[]*pb.Gateway{tt.gateway, tt.gateway},
				map[string]struct{}{"openshell-one": {}},
			)
			if len(got) != tt.want {
				t.Fatalf("identities = %v, want %d deduplicated identities", got, tt.want)
			}
		})
	}
}

func TestReadySandboxCountUsesCurrentSandboxReadyCondition(t *testing.T) {
	eligible := map[string]struct{}{"openshell-one": {}}
	sandbox := func(name, namespace, status string, generation, observedGeneration int64) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": name, "namespace": namespace, "generation": generation},
			"status": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{
				"type": "Ready", "status": status, "observedGeneration": observedGeneration,
			}}},
		}}
	}
	sandboxes := []*unstructured.Unstructured{
		sandbox("ready", "openshell-one", "True", 2, 2),
		sandbox("stale", "openshell-one", "True", 2, 1),
		sandbox("false", "openshell-one", "False", 2, 2),
		sandbox("outside", "other", "True", 2, 2),
		{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "missing", "namespace": "openshell-one", "generation": int64(2)}}},
	}
	if got := readySandboxCount(sandboxes, eligible); got != 1 {
		t.Fatalf("ready count = %d, want 1", got)
	}
}

func TestDesiredOIDCClientIDs(t *testing.T) {
	config, _ := json.Marshal(gateway.OIDCConfig{ClientID: "gateway-client"})
	value := string(config)
	got := desiredOIDCClientIDs([]*pb.Gateway{{Oidc: &value}, {Oidc: &value}, {}})
	if len(got) != 1 || got[0] != "gateway-client" {
		t.Fatalf("client IDs = %v", got)
	}
}

func testCertificatePEM(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(notAfter.UnixNano()), Subject: pkix.Name{CommonName: "test"}, NotBefore: notAfter.Add(-30 * 24 * time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
