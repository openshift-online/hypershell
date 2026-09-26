package grpctls

import (
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
)

type testPKI struct {
	pool     *x509.CertPool
	certFile string
	keyFile  string
}

// issue writes a fresh CA-signed leaf for serverName into dir, returning the
// pool that trusts it. Each call uses a new CA so two issues are distinguishable.
func issue(t *testing.T, dir, serverName string, serial int64) testPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		NotAfter:              time.Now().Add(time.Hour),
		NotBefore:             time.Now().Add(-time.Minute),
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: "test ca"},
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		DNSNames:     []string{serverName},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		NotAfter:     time.Now().Add(time.Hour),
		NotBefore:    time.Now().Add(-time.Minute),
		SerialNumber: big.NewInt(serial + 1),
		Subject:      pkix.Name{CommonName: serverName},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return testPKI{pool: pool, certFile: certFile, keyFile: keyFile}
}

// serve accepts connections and completes the TLS handshake on each so the
// client side observes the negotiated state.
func serve(t *testing.T, listener net.Listener) {
	t.Helper()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				if tlsConn, ok := conn.(*tls.Conn); ok {
					_ = tlsConn.Handshake()
				}
				_ = conn.Close()
			}()
		}
	}()
}

func dial(t *testing.T, addr string, pool *x509.CertPool) (tls.ConnectionState, error) {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2"},
		RootCAs:    pool,
		ServerName: "grpc.example.test",
	})
	if err != nil {
		return tls.ConnectionState{}, err
	}
	defer func() { _ = conn.Close() }()
	return conn.ConnectionState(), nil
}

func TestWrapServesTrustedCertificateWithHTTP2(t *testing.T) {
	dir := t.TempDir()
	pki := issue(t, dir, "grpc.example.test", 1)
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := Wrap(inner, Config{CertFile: pki.certFile, KeyFile: pki.keyFile})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	serve(t, listener)

	state, err := dial(t, listener.Addr().String(), pki.pool)
	if err != nil {
		t.Fatalf("TLS dial against the wrapped listener failed: %v", err)
	}
	if state.NegotiatedProtocol != "h2" {
		t.Fatalf("negotiated ALPN %q, want h2", state.NegotiatedProtocol)
	}
	if state.Version < tls.VersionTLS12 {
		t.Fatalf("negotiated TLS version %x, want at least 1.2", state.Version)
	}
}

func TestWrapRejectsUntrustedClientTrust(t *testing.T) {
	dir := t.TempDir()
	pki := issue(t, dir, "grpc.example.test", 10)
	other := issue(t, t.TempDir(), "grpc.example.test", 20)
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := Wrap(inner, Config{CertFile: pki.certFile, KeyFile: pki.keyFile})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	serve(t, listener)

	if _, err := dial(t, listener.Addr().String(), other.pool); err == nil {
		t.Fatal("dial with a pool that does not trust the served certificate succeeded")
	}
}

func TestWrapReloadsRenewedCertificate(t *testing.T) {
	dir := t.TempDir()
	first := issue(t, dir, "grpc.example.test", 100)
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := Wrap(inner, Config{CertFile: first.certFile, KeyFile: first.keyFile})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	serve(t, listener)
	if _, err := dial(t, listener.Addr().String(), first.pool); err != nil {
		t.Fatalf("dial before renewal failed: %v", err)
	}

	// Renew in place (same paths, new CA and leaf) and move the mtimes forward
	// so the change is visible even on filesystems with coarse timestamps.
	renewed := issue(t, dir, "grpc.example.test", 200)
	future := time.Now().Add(2 * time.Second)
	for _, path := range []string{renewed.certFile, renewed.keyFile} {
		if err := os.Chtimes(path, future, future); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := dial(t, listener.Addr().String(), renewed.pool); err != nil {
		t.Fatalf("dial trusting the renewed certificate failed, so it was not reloaded: %v", err)
	}
	if _, err := dial(t, listener.Addr().String(), first.pool); err == nil {
		t.Fatal("dial trusting only the old certificate succeeded after renewal")
	}
}

func TestWrapKeepsServingWhenRenewalIsUnreadable(t *testing.T) {
	dir := t.TempDir()
	pki := issue(t, dir, "grpc.example.test", 300)
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := Wrap(inner, Config{CertFile: pki.certFile, KeyFile: pki.keyFile})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	serve(t, listener)

	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(pki.certFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(pki.certFile, future, future); err != nil {
		t.Fatal(err)
	}

	if _, err := dial(t, listener.Addr().String(), pki.pool); err != nil {
		t.Fatalf("listener stopped serving the previous certificate after a bad renewal: %v", err)
	}
}

func TestWrapFailsFastOnMissingFiles(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = inner.Close() }()
	if _, err := Wrap(inner, Config{CertFile: filepath.Join(t.TempDir(), "missing.crt"), KeyFile: "x"}); err == nil {
		t.Fatal("Wrap with a missing certificate file succeeded")
	}
	if _, err := Wrap(inner, Config{}); err == nil {
		t.Fatal("Wrap with no files configured succeeded")
	}
}
