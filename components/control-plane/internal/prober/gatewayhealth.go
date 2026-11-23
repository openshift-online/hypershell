package prober

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	openshellv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/openshellv1"
)

// healthClient is the subset of the generated OpenShell client the gateway health
// probe calls (OBS-PRB-03). Narrowing to this interface lets tests inject a fake
// without a real gRPC server.
type healthClient interface {
	Health(ctx context.Context, in *openshellv1.HealthRequest, opts ...grpc.CallOption) (*openshellv1.HealthResponse, error)
}

// healthDialer opens a connection to a gateway's public OpenShell endpoint and
// returns a health client and a close function. It is a seam for tests.
type healthDialer func(ctx context.Context, routeAddress string) (healthClient, func() error, error)

// newTLSHealthDialer returns a healthDialer that dials the gateway's public route
// with normal TLS verification (OBS-PRB-03): the prober reaches the gateway
// exactly as an external user would, exercising the same ingress, routing, and
// serving path (OBS-PRB-01). The call is unauthenticated because the OpenShell
// grpc.health.v1 service is exempt from JWT validation on the public listener.
//
// tlsConfig is nil in production (the system trust store verifies the public
// route's certificate); tests supply a config trusting their test CA.
func newTLSHealthDialer(tlsConfig *tls.Config) healthDialer {
	return func(_ context.Context, routeAddress string) (healthClient, func() error, error) {
		authority := normalizeGatewayEndpoint(routeAddress)
		if authority == "" {
			return nil, nil, fmt.Errorf("empty gateway route address")
		}
		cfg := tlsConfig.Clone()
		if cfg == nil {
			cfg = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		if cfg.MinVersion == 0 {
			cfg.MinVersion = tls.VersionTLS12
		}
		conn, err := grpc.NewClient(authority, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
		if err != nil {
			return nil, nil, fmt.Errorf("dial gateway %q: %w", authority, err)
		}
		return openshellv1.NewOpenShellClient(conn), conn.Close, nil
	}
}

// probeGatewayHealth runs one gateway health probe (OBS-PRB-03). It dials the
// gateway's public route, calls the unauthenticated OpenShell Health RPC under
// ctx (which carries the per-probe timeout), and classifies the result. A probe
// is good only when the gateway answers the Health call successfully within the
// timeout; a transport failure, a timeout, or an unsuccessful Health response is
// a failed probe.
func probeGatewayHealth(ctx context.Context, dial healthDialer, routeAddress string) Result {
	client, closeConn, err := dial(ctx, routeAddress)
	if err != nil {
		return ResultFailure
	}
	defer func() {
		if closeConn != nil {
			_ = closeConn()
		}
	}()

	resp, err := client.Health(ctx, &openshellv1.HealthRequest{})
	return classifyGatewayHealth(resp, err)
}

// classifyGatewayHealth classifies a gateway Health response (OBS-PRB-03). The
// Gateway availability objective is black-box answering: whether the gateway
// answers the Health call through its public route, exactly as a user would see
// it (OBS-PRB-01, spec Purpose). A successful RPC means the gateway answered, so
// a HEALTHY or DEGRADED status is a good probe: a DEGRADED gateway is still
// serving its users through the public route, and self-reported degradation is
// the in-pod health reconciler's concern (OSG-HLTH-04), which this prober must
// not duplicate (OBS-PRB-01). A failed probe is reserved for an outcome where the
// gateway did not answer or reported itself not serving: a transport failure, a
// timeout, a non-OK gRPC status (err != nil), an UNHEALTHY status, or an
// unspecified/empty status.
func classifyGatewayHealth(resp *openshellv1.HealthResponse, err error) Result {
	if err != nil || resp == nil {
		return ResultFailure
	}
	switch resp.GetStatus() {
	case openshellv1.ServiceStatus_SERVICE_STATUS_HEALTHY,
		openshellv1.ServiceStatus_SERVICE_STATUS_DEGRADED:
		return ResultSuccess // the gateway answered through its public route
	default:
		return ResultFailure // UNHEALTHY or UNSPECIFIED: not serving
	}
}

// normalizeGatewayEndpoint reduces a gateway route_address (a grpcs://host:443
// address) to the host:port authority gRPC dials: it strips a scheme and any
// path and defaults the port to 443.
func normalizeGatewayEndpoint(endpoint string) string {
	e := strings.TrimSpace(endpoint)
	for _, scheme := range []string{"grpcs://", "grpc://", "https://", "http://"} {
		e = strings.TrimPrefix(e, scheme)
	}
	if i := strings.IndexByte(e, '/'); i >= 0 {
		e = e[:i]
	}
	if e == "" {
		return ""
	}
	if !strings.Contains(e, ":") {
		e += ":443"
	}
	return e
}
