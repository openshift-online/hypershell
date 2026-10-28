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

func TestReadySandboxCountUsesProvisionedReadyPopulation(t *testing.T) {
	eligible := map[string]struct{}{"openshell-one": {}}
	sandboxes := []*unstructured.Unstructured{{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "ready", "namespace": "openshell-one"}}}, {Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "pending", "namespace": "openshell-one"}}}}
	pod := func(name string, ready bool) *corev1.Pod {
		status := corev1.ConditionFalse
		if ready {
			status = corev1.ConditionTrue
		}
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name + "-pod", Namespace: "openshell-one", OwnerReferences: []metav1.OwnerReference{{Kind: "Sandbox", Name: name}}}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}}}}
	}
	if got := readySandboxCount([]*corev1.Pod{pod("ready", true), pod("pending", false)}, sandboxes, eligible); got != 1 {
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
