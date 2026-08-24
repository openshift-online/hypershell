package serviceAccounts

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

func TestControlPlaneProvisionerMapsDesiredStateAndOneTimeSecret(t *testing.T) {
	client := &fakeProvisionerClient{provisionResponse: &pb.ProvisionResponse{
		ClientUuid: "client-uuid", ClientId: "client-id", ClientSecret: "one-time-secret", Subject: "subject-id",
	}}
	provisioner := &controlPlaneProvisioner{client: client}
	result, err := provisioner.ProvisionServiceAccount(t.Context(), ProvisioningSpec{
		ClientID: "client-id", DisplayName: "build bot", GatewayClientID: "gateway-client",
		GatewayID: "gateway-id", ServiceAccountID: "account-id", CreatorUserID: "user-id",
		Role: "openshell-user", ExpectedIssuer: "https://issuer.example/realms/hypershell",
		AccessTokenLifetimeSeconds: 300,
	})
	if err != nil {
		t.Fatalf("ProvisionServiceAccount() error = %v", err)
	}
	if result.ClientSecret != "one-time-secret" || result.ClientUUID != "client-uuid" || result.Subject != "subject-id" {
		t.Fatalf("ProvisionServiceAccount() = %#v", result)
	}
	if client.provisionRequest.GetSpec().GetGatewayId() != "gateway-id" ||
		client.provisionRequest.GetSpec().GetRole() != "openshell-user" ||
		client.provisionRequest.GetSpec().GetAccessTokenLifetimeSeconds() != 300 {
		t.Fatalf("provision request = %#v", client.provisionRequest)
	}
}

func TestControlPlaneProvisionerMapsNotFound(t *testing.T) {
	provisioner := &controlPlaneProvisioner{client: &fakeProvisionerClient{
		reconcileErr: status.Error(codes.NotFound, "managed client missing"),
	}}
	err := provisioner.ReconcileServiceAccount(t.Context(), ProvisioningSpec{}, "client-uuid", "subject-id", true)
	if !errors.Is(err, ErrProvisionerNotFound) {
		t.Fatalf("ReconcileServiceAccount() error = %v, want ErrProvisionerNotFound", err)
	}
}

func TestProvisionerTLSConfigurationIsRequired(t *testing.T) {
	if _, err := loadProvisionerClientCredentials("", "", "", ""); err == nil {
		t.Fatal("loadProvisionerClientCredentials() error = nil")
	}
}

func TestProvisionerClientCredentialsReloadRotatedCertificate(t *testing.T) {
	ca := newTestCA(t)
	directory := t.TempDir()
	certFile := filepath.Join(directory, "tls.crt")
	keyFile := filepath.Join(directory, "tls.key")
	caFile := filepath.Join(directory, "ca.crt")
	writeClientTestFile(t, caFile, ca.pemBytes)

	certPEM, keyPEM := ca.sign(t, "hypershell-api-server", nil, x509.ExtKeyUsageClientAuth, 7)
	writeClientTestFile(t, certFile, certPEM)
	writeClientTestFile(t, keyFile, keyPEM)

	clientCredentials, err := loadProvisionerClientCredentials(certFile, keyFile, caFile, "provisioner.test")
	if err != nil {
		t.Fatalf("loadProvisionerClientCredentials() error = %v", err)
	}

	serverConfig := ca.serverTLSConfig(t)
	firstSerial := clientLeafSerialSeenByServer(t, clientCredentials, serverConfig)
	if firstSerial != "7" {
		t.Fatalf("initial client certificate serial = %s, want 7", firstSerial)
	}

	// Rotate the mounted key pair in place, as cert-manager does on renewal.
	rotatedCertPEM, rotatedKeyPEM := ca.sign(t, "hypershell-api-server", nil, x509.ExtKeyUsageClientAuth, 99)
	writeClientTestFile(t, certFile, rotatedCertPEM)
	writeClientTestFile(t, keyFile, rotatedKeyPEM)
	bumpClientModTime(t, certFile)
	bumpClientModTime(t, keyFile)

	// Reuse the SAME credentials object; the reloader must present the new pair.
	secondSerial := clientLeafSerialSeenByServer(t, clientCredentials, serverConfig)
	if secondSerial != "99" {
		t.Fatalf("rotated client certificate serial = %s, want 99", secondSerial)
	}
	if firstSerial == secondSerial {
		t.Fatal("client credentials did not present the rotated certificate")
	}
}

func TestProvisionerClientCredentialsReloadRotatedServerCA(t *testing.T) {
	ca := newTestCA(t)
	directory := t.TempDir()
	certFile := filepath.Join(directory, "tls.crt")
	keyFile := filepath.Join(directory, "tls.key")
	caFile := filepath.Join(directory, "ca.crt")
	writeClientTestFile(t, caFile, ca.pemBytes)

	certPEM, keyPEM := ca.sign(t, "hypershell-api-server", nil, x509.ExtKeyUsageClientAuth, 7)
	writeClientTestFile(t, certFile, certPEM)
	writeClientTestFile(t, keyFile, keyPEM)

	clientCredentials, err := loadProvisionerClientCredentials(certFile, keyFile, caFile, "provisioner.test")
	if err != nil {
		t.Fatalf("loadProvisionerClientCredentials() error = %v", err)
	}

	// A server whose leaf is signed by a rotated CA the client does not yet trust.
	// The server still trusts the client via the original CA pool.
	rotatedCA := newTestCA(t)
	rotatedServer := rotatedCA.serverTLSConfigTrusting(t, ca.pool)
	if handshakeErr := attemptClientHandshake(t, clientCredentials, rotatedServer); handshakeErr == nil {
		t.Fatal("client trusted a server signed by an unrotated CA")
	}

	// Rotate the mounted trust bundle in place, as cert-manager does when the
	// server-issuing CA rolls over.
	rotatedBundle := append(append([]byte{}, ca.pemBytes...), rotatedCA.pemBytes...)
	writeClientTestFile(t, caFile, rotatedBundle)
	bumpClientModTime(t, caFile)

	// Reuse the SAME credentials object; the reloaded bundle now trusts the CA.
	if handshakeErr := attemptClientHandshake(t, clientCredentials, rotatedServer); handshakeErr != nil {
		t.Fatalf("client did not trust rotated server CA: %v", handshakeErr)
	}
}

// attemptClientHandshake dials a TLS server and returns the client-side result
// of the handshake so a rejected server certificate can be asserted.
func attemptClientHandshake(t *testing.T, clientCredentials credentials.TransportCredentials, serverConfig *tls.Config) error {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test handshake: %v", err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		serverConnection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = serverConnection.Close() }()
		tlsConnection := tls.Server(serverConnection, serverConfig)
		_ = tlsConnection.Handshake()
	}()

	clientConnection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial test handshake: %v", err)
	}
	defer func() { _ = clientConnection.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, _, handshakeErr := clientCredentials.ClientHandshake(ctx, "provisioner.test", clientConnection)
	return handshakeErr
}

// clientLeafSerialSeenByServer completes a handshake and returns the serial
// number of the client leaf certificate the server observed.
func clientLeafSerialSeenByServer(t *testing.T, clientCredentials credentials.TransportCredentials, serverConfig *tls.Config) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test handshake: %v", err)
	}
	defer func() { _ = listener.Close() }()

	serialResult := make(chan string, 1)
	serverError := make(chan error, 1)
	go func() {
		serverConnection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverError <- acceptErr
			return
		}
		defer func() { _ = serverConnection.Close() }()
		tlsConnection := tls.Server(serverConnection, serverConfig)
		if handshakeErr := tlsConnection.Handshake(); handshakeErr != nil {
			serverError <- handshakeErr
			return
		}
		state := tlsConnection.ConnectionState()
		if len(state.PeerCertificates) == 0 {
			serverError <- errors.New("client presented no certificate")
			return
		}
		serialResult <- state.PeerCertificates[0].SerialNumber.String()
	}()

	clientConnection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial test handshake: %v", err)
	}
	defer func() { _ = clientConnection.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, _, err := clientCredentials.ClientHandshake(ctx, "provisioner.test", clientConnection); err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	select {
	case serial := <-serialResult:
		return serial
	case err := <-serverError:
		t.Fatalf("server handshake: %v", err)
	case <-ctx.Done():
		t.Fatalf("handshake timed out: %v", ctx.Err())
	}
	return ""
}

type testCA struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	pemBytes    []byte
	pool        *x509.CertPool
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		t.Fatal("append test CA")
	}
	return &testCA{certificate: certificate, key: key, pemBytes: pemBytes, pool: pool}
}

func (c *testCA) sign(t *testing.T, commonName string, dnsNames []string, usage x509.ExtKeyUsage, serial int64) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate certificate key: %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: commonName},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), DNSNames: dnsNames,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, c.certificate, &key.PublicKey, c.key)
	if err != nil {
		t.Fatalf("create signed certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal certificate key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func (c *testCA) serverTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	return c.serverTLSConfigTrusting(t, c.pool)
}

// serverTLSConfigTrusting presents a server leaf signed by this CA while
// trusting an explicit client CA pool, so a rotated server-issuing CA can be
// exercised independently of the client's own identity.
func (c *testCA) serverTLSConfigTrusting(t *testing.T, clientPool *x509.CertPool) *tls.Config {
	t.Helper()
	certPEM, keyPEM := c.sign(t, "hypershell-controller", []string{"provisioner.test"}, x509.ExtKeyUsageServerAuth, 500)
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("load server key pair: %v", err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    clientPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		// gRPC transport credentials enforce ALPN; advertise HTTP/2.
		NextProtos: []string{"h2"},
	}
}

func writeClientTestFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func bumpClientModTime(t *testing.T, path string) {
	t.Helper()
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

type fakeProvisionerClient struct {
	provisionRequest  *pb.ProvisionRequest
	provisionResponse *pb.ProvisionResponse
	provisionErr      error
	reconcileErr      error
}

func (f *fakeProvisionerClient) Provision(_ context.Context, request *pb.ProvisionRequest, _ ...grpc.CallOption) (*pb.ProvisionResponse, error) {
	f.provisionRequest = request
	return f.provisionResponse, f.provisionErr
}

func (f *fakeProvisionerClient) Reconcile(context.Context, *pb.ReconcileRequest, ...grpc.CallOption) (*pb.ReconcileResponse, error) {
	return &pb.ReconcileResponse{}, f.reconcileErr
}

func (f *fakeProvisionerClient) Disable(context.Context, *pb.DisableRequest, ...grpc.CallOption) (*pb.DisableResponse, error) {
	return &pb.DisableResponse{}, nil
}

func (f *fakeProvisionerClient) Delete(context.Context, *pb.DeleteRequest, ...grpc.CallOption) (*pb.DeleteResponse, error) {
	return &pb.DeleteResponse{}, nil
}

func (f *fakeProvisionerClient) DeleteManaged(context.Context, *pb.DeleteManagedRequest, ...grpc.CallOption) (*pb.DeleteManagedResponse, error) {
	return &pb.DeleteManagedResponse{}, nil
}

func (f *fakeProvisionerClient) DeleteGateway(context.Context, *pb.DeleteGatewayRequest, ...grpc.CallOption) (*pb.DeleteGatewayResponse, error) {
	return &pb.DeleteGatewayResponse{}, nil
}

func (f *fakeProvisionerClient) ListManaged(context.Context, *pb.ListManagedRequest, ...grpc.CallOption) (*pb.ListManagedResponse, error) {
	return &pb.ListManagedResponse{}, nil
}
