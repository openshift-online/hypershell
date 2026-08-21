package serviceaccountprovisioner

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"
)

func TestServerCredentialsRequireExpectedAPIClientIdentity(t *testing.T) {
	fixture := newTLSFixture(t)
	serverCredentials, err := loadServerCredentials(TransportConfig{
		Address:                  "127.0.0.1:9443",
		CertificateFile:          fixture.serverCertFile,
		KeyFile:                  fixture.serverKeyFile,
		ClientCAFile:             fixture.caFile,
		ExpectedClientCommonName: "hypershell-api-server",
	})
	if err != nil {
		t.Fatalf("loadServerCredentials() error = %v", err)
	}

	validClient := fixture.clientCredentials(t, "hypershell-api-server")
	if serverErr, clientErr := performTLSHandshake(t, serverCredentials, validClient); serverErr != nil || clientErr != nil {
		t.Fatalf("valid mTLS handshake errors = server %v, client %v", serverErr, clientErr)
	}

	wrongClient := fixture.clientCredentials(t, "another-workload")
	serverErr, _ := performTLSHandshake(t, serverCredentials, wrongClient)
	if serverErr == nil {
		t.Fatal("mTLS handshake accepted an unexpected client identity")
	}
}

func performTLSHandshake(t *testing.T, serverCredentials, clientCredentials credentials.TransportCredentials) (error, error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test handshake: %v", err)
	}
	defer func() { _ = listener.Close() }()
	serverResult := make(chan error, 1)
	go func() {
		serverConnection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverResult <- acceptErr
			return
		}
		defer func() { _ = serverConnection.Close() }()
		_, _, err := serverCredentials.ServerHandshake(serverConnection)
		serverResult <- err
	}()

	clientConnection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial test handshake: %v", err)
	}
	defer func() { _ = clientConnection.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, _, clientErr := clientCredentials.ClientHandshake(ctx, "provisioner.test", clientConnection)
	select {
	case serverErr := <-serverResult:
		return serverErr, clientErr
	case <-ctx.Done():
		return ctx.Err(), clientErr
	}
}

type tlsFixture struct {
	caCertificate  *x509.Certificate
	caKey          *ecdsa.PrivateKey
	caPEM          []byte
	caFile         string
	serverCertFile string
	serverKeyFile  string
}

func newTLSFixture(t *testing.T) tlsFixture {
	t.Helper()
	now := time.Now().UTC()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	serverCertPEM, serverKeyPEM := signedCertificate(t, caCertificate, caKey, "hypershell-controller", []string{"provisioner.test"}, x509.ExtKeyUsageServerAuth, 2)
	directory := t.TempDir()
	fixture := tlsFixture{
		caCertificate: caCertificate, caKey: caKey, caPEM: caPEM,
		caFile:         filepath.Join(directory, "ca.crt"),
		serverCertFile: filepath.Join(directory, "tls.crt"),
		serverKeyFile:  filepath.Join(directory, "tls.key"),
	}
	writeTestFile(t, fixture.caFile, caPEM)
	writeTestFile(t, fixture.serverCertFile, serverCertPEM)
	writeTestFile(t, fixture.serverKeyFile, serverKeyPEM)
	return fixture
}

func (f tlsFixture) clientCredentials(t *testing.T, commonName string) credentials.TransportCredentials {
	t.Helper()
	certPEM, keyPEM := signedCertificate(t, f.caCertificate, f.caKey, commonName, nil, x509.ExtKeyUsageClientAuth, 3)
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("load client key pair: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(f.caPEM) {
		t.Fatal("append test CA")
	}
	return credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, ServerName: "provisioner.test",
		Certificates: []tls.Certificate{certificate}, RootCAs: roots,
	})
}

func signedCertificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, commonName string, dnsNames []string, usage x509.ExtKeyUsage, serial int64) ([]byte, []byte) {
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
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create signed certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal certificate key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func writeTestFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
