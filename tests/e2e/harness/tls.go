package harness

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"time"
)

// BuildCAPool returns a cert pool seeded from the system roots (when available)
// plus the supplied PEM CA, so the suite trusts both public CAs and the cluster's
// self-signed CA (the Kind CA extracted from hypershell-ca-secret).
func BuildCAPool(caPEM []byte) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no certificates parsed from CA PEM (%d bytes)", len(caPEM))
	}
	return pool, nil
}

// HostRewriter maps a connection hostname to an alternate dial address. It
// returns the rewritten address ("127.0.0.1:443") and true when the host should
// be redirected, or ("", false) to dial the original address. The Kind ingress
// and gateway hostnames (*.hypershell.localhost, *.gw.localhost) are not in DNS
// on the host running the suite, so the dialer rewrites them to the forwarded
// cluster ingress, mirroring the Bash driver's `curl --connect-to`. TLS
// SNI/verification still uses the original hostname; only the dialed address
// changes.
type HostRewriter func(host string) (addr string, ok bool)

// InstallDefaultCA replaces http.DefaultTransport with a clone that trusts pool
// and applies rewrite. The generated HyperShell Go SDK constructs its http.Client
// without a custom Transport, so it uses http.DefaultTransport; installing the
// cluster CA here is what lets the SDK reach the API server over the cluster's
// self-signed TLS without an insecure bypass, and without modifying the generated
// SDK. This is a process-global change, appropriate for a single test binary.
func InstallDefaultCA(pool *x509.CertPool, rewrite HostRewriter) error {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return fmt.Errorf("http.DefaultTransport is not *http.Transport")
	}
	tr := base.Clone()
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{}
	}
	tr.TLSClientConfig.RootCAs = pool
	applyHostRewrite(tr, rewrite)
	http.DefaultTransport = tr
	return nil
}

// HTTPClientWithCA returns an http.Client that trusts pool and applies rewrite,
// for the driver's raw HTTPS calls (Keycloak token + admin API, the BFF OIDC
// endpoints, the unauthenticated 401 probe) that do not go through the SDK.
func HTTPClientWithCA(pool *x509.CertPool, rewrite HostRewriter, timeout time.Duration) *http.Client {
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
	applyHostRewrite(tr, rewrite)
	return &http.Client{Transport: tr, Timeout: timeout}
}

func applyHostRewrite(tr *http.Transport, rewrite HostRewriter) {
	if rewrite == nil {
		return
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if host, _, err := net.SplitHostPort(addr); err == nil {
			if target, ok := rewrite(host); ok {
				addr = target
			}
		}
		return dialer.DialContext(ctx, network, addr)
	}
}
