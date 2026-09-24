package grpctransport

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc/credentials/insecure"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		addr string
		want Mode
	}{
		// Spec scenarios (control-plane.spec.md, gRPC Transport Security).
		{"hypershell-api-server.hypershell-system.svc.cluster.local:9000", Plaintext},
		{"hypershell-api-server.hypershell-system.svc:9000", Plaintext},
		{"hypershell-api-server:9000", Plaintext},
		{"grpc.hyp0.infra.hypershell.app:443", TLS},

		// Loopback and localhost.
		{"localhost:9000", Plaintext},
		{"LOCALHOST:9000", Plaintext},
		{"127.0.0.1:9000", Plaintext},
		{"127.0.0.53:9000", Plaintext},
		{"[::1]:9000", Plaintext},
		{":9000", Plaintext},

		// Any cluster domain, trailing-dot FQDN, bare host without port.
		{"api.ns.svc.example.internal:9000", Plaintext},
		{"hypershell-api-server.hypershell-system.svc.cluster.local.:9000", Plaintext},
		{"hypershell-api-server", Plaintext},
		{"dns:///hypershell-api-server.hypershell-system.svc:9000", Plaintext},

		// External: public names, names merely containing "svc", and
		// non-loopback IP literals (never a Service short name).
		{"grpc.hyp0.infra.hypershell.app", TLS},
		{"dns:///grpc.hyp0.infra.hypershell.app:443", TLS},
		{"svc.example.com:443", TLS},
		{"mysvc.example.com:443", TLS},
		{"api.example.svcx:443", TLS},
		{"10.0.0.5:9000", TLS},
		{"[fd00::1]:9000", TLS},
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			if got := Classify(tc.addr); got != tc.want {
				t.Fatalf("Classify(%q) = %s, want %s", tc.addr, got, tc.want)
			}
		})
	}
}

func TestCredentials(t *testing.T) {
	plain := Credentials("hypershell-api-server:9000")
	if plain.Info().SecurityProtocol != insecure.NewCredentials().Info().SecurityProtocol {
		t.Fatalf("in-cluster address: security protocol = %q, want insecure", plain.Info().SecurityProtocol)
	}
	tlsCreds := Credentials("grpc.hyp0.infra.hypershell.app:443")
	if got := tlsCreds.Info().SecurityProtocol; got != "tls" {
		t.Fatalf("external address: security protocol = %q, want tls", got)
	}
}

// TestCredentialsTLSVerifiesServerCertificate proves the external transport uses
// standard verification: a handshake against a server whose certificate does not
// chain to the system trust store fails (it is never InsecureSkipVerify). The
// watchers treat that failure as an ordinary disconnect and retry with backoff.
func TestCredentialsTLSVerifiesServerCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	creds := Credentials("grpc.hyp0.infra.hypershell.app:443")
	if _, _, err := creds.ClientHandshake(ctx, "grpc.hyp0.infra.hypershell.app:443", conn); err == nil {
		t.Fatal("TLS handshake against an untrusted certificate succeeded, want verification failure")
	}
}
