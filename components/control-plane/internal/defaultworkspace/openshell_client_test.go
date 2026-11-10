package defaultworkspace

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGRPCOpenShellClientTLSUsesGatewaySecret(t *testing.T) {
	secret := clientTLSSecret(t)
	client := NewGRPCOpenShellClient(fake.NewSimpleClientset(secret))

	certificate, rootCAs, err := client.clientTLS(t.Context(), "gateway-namespace")
	if err != nil {
		t.Fatalf("clientTLS() error = %v", err)
	}
	if len(certificate.Certificate) == 0 {
		t.Fatal("clientTLS() returned no client certificate")
	}
	if len(rootCAs.Subjects()) != 1 {
		t.Fatalf("clientTLS() trusted subjects = %v, want one", rootCAs.Subjects())
	}
}

func TestBootstrapRequestIDIsStableForTheSameMembership(t *testing.T) {
	first := bootstrapRequestID("gateway-namespace", "subject-id", "user")
	second := bootstrapRequestID("gateway-namespace", "subject-id", "user")
	if first != second {
		t.Fatalf("bootstrapRequestID() = %q, want stable %q", first, second)
	}
	if first == bootstrapRequestID("gateway-namespace", "other-subject", "user") {
		t.Fatal("bootstrapRequestID() did not distinguish subjects")
	}
}

func TestGRPCOpenShellClientTLSFailsWithoutGatewayCA(t *testing.T) {
	secret := clientTLSSecret(t)
	delete(secret.Data, corev1.ServiceAccountRootCAKey)
	client := NewGRPCOpenShellClient(fake.NewSimpleClientset(secret))

	_, _, err := client.clientTLS(t.Context(), "gateway-namespace")
	if err == nil {
		t.Fatal("clientTLS() error = nil, want missing CA failure")
	}
}

func clientTLSSecret(t *testing.T) *corev1.Secret {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "openshell-client"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, clientKey.Public(), caKey)
	if err != nil {
		t.Fatalf("create client certificate: %v", err)
	}
	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: clientTLSSecretName, Namespace: "gateway-namespace"},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:              pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER}),
			corev1.TLSPrivateKeyKey:        pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: clientKeyDER}),
			corev1.ServiceAccountRootCAKey: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		},
	}
}
