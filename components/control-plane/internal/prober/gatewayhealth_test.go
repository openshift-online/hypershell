package prober

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"

	openshellv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/openshellv1"
)

// fakeHealthClient is an injected OpenShell health client for tests.
type fakeHealthClient struct {
	status openshellv1.ServiceStatus
	err    error
	delay  time.Duration
}

func (f fakeHealthClient) Health(ctx context.Context, _ *openshellv1.HealthRequest, _ ...grpc.CallOption) (*openshellv1.HealthResponse, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return &openshellv1.HealthResponse{Status: f.status}, nil
}

func dialReturning(c healthClient, err error) healthDialer {
	return func(context.Context, string) (healthClient, func() error, error) {
		if err != nil {
			return nil, nil, err
		}
		return c, func() error { return nil }, nil
	}
}

func TestClassifyGatewayHealth(t *testing.T) {
	cases := []struct {
		name string
		resp *openshellv1.HealthResponse
		err  error
		want Result
	}{
		// A successful RPC means the gateway answered through its public route;
		// HEALTHY and DEGRADED are both good availability probes (OBS-PRB-03).
		{"healthy", &openshellv1.HealthResponse{Status: openshellv1.ServiceStatus_SERVICE_STATUS_HEALTHY}, nil, ResultSuccess},
		{"degraded answers so is good", &openshellv1.HealthResponse{Status: openshellv1.ServiceStatus_SERVICE_STATUS_DEGRADED}, nil, ResultSuccess},
		{"unhealthy is not serving", &openshellv1.HealthResponse{Status: openshellv1.ServiceStatus_SERVICE_STATUS_UNHEALTHY}, nil, ResultFailure},
		{"unspecified is failure", &openshellv1.HealthResponse{Status: openshellv1.ServiceStatus_SERVICE_STATUS_UNSPECIFIED}, nil, ResultFailure},
		{"rpc error", nil, errors.New("transport"), ResultFailure},
		{"nil response", nil, nil, ResultFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyGatewayHealth(tc.resp, tc.err); got != tc.want {
				t.Errorf("classifyGatewayHealth = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAnsweringGatewayIsGood covers OBS-PRB-03.01.
func TestAnsweringGatewayIsGood(t *testing.T) {
	dial := dialReturning(fakeHealthClient{status: openshellv1.ServiceStatus_SERVICE_STATUS_HEALTHY}, nil)
	if got := probeGatewayHealth(context.Background(), dial, "grpcs://g.example:443"); got != ResultSuccess {
		t.Fatalf("probe = %q, want success", got)
	}
}

// TestUnreachableGatewayIsFailed covers OBS-PRB-03.02: a gateway that does not
// answer within the timeout is a failed probe.
func TestUnreachableGatewayIsFailed(t *testing.T) {
	// Transport failure on dial.
	dial := dialReturning(nil, errors.New("connection refused"))
	if got := probeGatewayHealth(context.Background(), dial, "grpcs://g.example:443"); got != ResultFailure {
		t.Fatalf("probe (dial error) = %q, want failure", got)
	}

	// Timeout: the Health call outlives the per-probe deadline.
	slowDial := dialReturning(fakeHealthClient{status: openshellv1.ServiceStatus_SERVICE_STATUS_HEALTHY, delay: 200 * time.Millisecond}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if got := probeGatewayHealth(ctx, slowDial, "grpcs://g.example:443"); got != ResultFailure {
		t.Fatalf("probe (timeout) = %q, want failure", got)
	}
}

func TestNormalizeGatewayEndpoint(t *testing.T) {
	cases := map[string]string{
		"grpcs://g.example:443": "g.example:443",
		"grpcs://g.example":     "g.example:443",
		"https://g.example/x":   "g.example:443",
		"g.example:8443":        "g.example:8443",
		"":                      "",
	}
	for in, want := range cases {
		if got := normalizeGatewayEndpoint(in); got != want {
			t.Errorf("normalizeGatewayEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}
