// Package grpctransport selects the gRPC transport credentials the control plane
// dials the API server with, from HYPERSHELL_GRPC_SERVER_ADDR alone.
//
// See specs/platform/control-plane.spec.md ("Requirement: gRPC Transport
// Security"): an in-cluster address dials plaintext, anything else dials TLS
// with the system trust store and standard hostname verification. There is no
// separate TLS flag, so the transport can never drift from the address.
package grpctransport

import (
	"crypto/tls"
	"net"
	"strings"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Mode is the transport a gRPC target address is dialed with.
type Mode string

const (
	// Plaintext is used for in-cluster addresses, which never leave the cluster.
	Plaintext Mode = "plaintext"
	// TLS is used for every other address (for example a hub's passthrough Route).
	TLS Mode = "tls"
)

// Classify returns the transport mode for a gRPC target address.
//
// An address is in-cluster (Plaintext) when its host is:
//   - "localhost" or a loopback IP;
//   - a name with no dot at all (a bare Kubernetes Service short name such as
//     "hypershell-api-server");
//   - a name ending in ".svc" or containing ".svc." (a namespace-qualified
//     Service name under any cluster domain).
//
// Every other address is external (TLS). IP literals are classified by
// loopback-ness only: a non-loopback IP is never a Service name, so it dials TLS
// even when (as with IPv6) it contains no dot.
func Classify(target string) Mode {
	host := strings.ToLower(hostOf(target))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		// An empty host is dialed as localhost by grpc-go ("":9000).
		return Plaintext
	}
	if host == "localhost" {
		return Plaintext
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return Plaintext
		}
		return TLS
	}
	if !strings.Contains(host, ".") {
		return Plaintext
	}
	if strings.HasSuffix(host, ".svc") || strings.Contains(host, ".svc.") {
		return Plaintext
	}
	return TLS
}

// Credentials returns the transport credentials for a gRPC target address:
// insecure credentials for in-cluster addresses, and TLS credentials using the
// system trust store with standard certificate and hostname verification
// otherwise. A handshake failure surfaces as an ordinary connection error, which
// the watchers retry with backoff like any other disconnect.
func Credentials(target string) credentials.TransportCredentials {
	if Classify(target) == Plaintext {
		return insecure.NewCredentials()
	}
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
}

// hostOf extracts the host part of a gRPC dial target. It accepts "host:port",
// "[ipv6]:port", a bare host, and resolver-scheme targets such as
// "dns:///host:port" or "dns://authority/host:port".
func hostOf(target string) string {
	t := strings.TrimSpace(target)
	if i := strings.Index(t, "://"); i >= 0 {
		t = t[i+3:]
		// "dns://authority/host:port" or "dns:///host:port": the endpoint is
		// everything after the authority's slash.
		if j := strings.Index(t, "/"); j >= 0 {
			t = t[j+1:]
		}
	}
	if host, _, err := net.SplitHostPort(t); err == nil {
		return host
	}
	return strings.TrimSuffix(strings.TrimPrefix(t, "["), "]")
}
