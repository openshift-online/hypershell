package e2e

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
)

func completeConditions() []map[string]any {
	return []map[string]any{
		{"type": "EnvironmentReady", "condition_status": "Complete"},
		{"type": "DatabaseReady", "condition_status": "Complete"},
		{"type": "GatewayDeployed", "condition_status": "Complete"},
		{"type": "GatewayHealthy", "condition_status": "Complete"},
	}
}

func incompleteConditions() []map[string]any {
	c := completeConditions()
	c[3] = map[string]any{"type": "GatewayHealthy", "condition_status": "InProgress"}
	return c
}

// readyGateway returns a response that passes every readiness check; tests
// mutate one field to make it fail exactly one check.
func readyGateway() *sdktypes.Gateway {
	return &sdktypes.Gateway{
		Phase:                  "Running",
		Generation:             2,
		ObservedGeneration:     2,
		ProvisioningConditions: completeConditions(),
		RouteAddress:           "grpcs://gw-1.example.test:443",
	}
}

func withGateway(mut func(*sdktypes.Gateway)) *sdktypes.Gateway {
	gw := readyGateway()
	mut(gw)
	return gw
}

// scriptedGet replays responses in order and then repeats the last one.
type scriptedGet struct {
	mu        sync.Mutex
	responses []*sdktypes.Gateway
	calls     int
}

func (g *scriptedGet) get(context.Context) (*sdktypes.Gateway, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	i := min(g.calls, len(g.responses)-1)
	g.calls++
	if g.responses[i] == nil {
		return nil, errors.New("transient API error")
	}
	return g.responses[i], nil
}

// Verifies: E2E-CORE-13.02
func TestPrimaryGatewayReadiness(t *testing.T) {
	tests := []struct {
		name    string
		gw      *sdktypes.Gateway
		wantErr string
	}{
		{name: "fully ready", gw: readyGateway()},
		{name: "nil response", gw: nil, wantErr: "no gateway response"},
		{name: "provisioning phase", gw: withGateway(func(g *sdktypes.Gateway) { g.Phase = "Provisioning" }), wantErr: `phase is "Provisioning"`},
		{name: "zero generation", gw: withGateway(func(g *sdktypes.Gateway) { g.Generation, g.ObservedGeneration = 0, 0 }), wantErr: "generation is 0"},
		{name: "generation not observed", gw: withGateway(func(g *sdktypes.Gateway) { g.ObservedGeneration = 1 }), wantErr: "observed_generation 1 does not match generation 2"},
		{name: "conditions incomplete", gw: withGateway(func(g *sdktypes.Gateway) { g.ProvisioningConditions = incompleteConditions() }), wantErr: "provisioning conditions not complete"},
		{name: "conditions empty", gw: withGateway(func(g *sdktypes.Gateway) { g.ProvisioningConditions = nil }), wantErr: "provisioning_conditions is empty"},
		{name: "route missing", gw: withGateway(func(g *sdktypes.Gateway) { g.RouteAddress = "" }), wantErr: "route_address is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := primaryGatewayReadiness(tt.gw)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want ready, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// Verifies: E2E-CORE-13.02
func TestWaitPrimaryGatewayReady(t *testing.T) {
	const interval = time.Millisecond

	runningIncomplete := withGateway(func(g *sdktypes.Gateway) { g.ProvisioningConditions = incompleteConditions() })
	provisioningComplete := withGateway(func(g *sdktypes.Gateway) { g.Phase = "Provisioning" })
	generationBehind := withGateway(func(g *sdktypes.Gateway) { g.ObservedGeneration = 1 })
	noRoute := withGateway(func(g *sdktypes.Gateway) { g.RouteAddress = "" })
	final := readyGateway()

	tests := []struct {
		name        string
		responses   []*sdktypes.Gateway
		timeout     time.Duration
		wantReady   *sdktypes.Gateway
		wantCalls   int
		wantErrPart []string
	}{
		{
			// The old two-wait check could accept the second response: Running was
			// seen first, then conditions completed while the phase was Provisioning.
			name:      "accepts only the response that meets every check",
			responses: []*sdktypes.Gateway{runningIncomplete, provisioningComplete, final},
			timeout:   5 * time.Second,
			wantReady: final,
			wantCalls: 3,
		},
		{
			name:      "keeps waiting while observed_generation lags generation",
			responses: []*sdktypes.Gateway{generationBehind, generationBehind, final},
			timeout:   5 * time.Second,
			wantReady: final,
			wantCalls: 3,
		},
		{
			name:      "keeps waiting until the route address is published",
			responses: []*sdktypes.Gateway{noRoute, final},
			timeout:   5 * time.Second,
			wantReady: final,
			wantCalls: 2,
		},
		{
			name:      "transient get errors are retried",
			responses: []*sdktypes.Gateway{nil, final},
			timeout:   5 * time.Second,
			wantReady: final,
			wantCalls: 2,
		},
		{
			name:      "succeeds on the first read",
			responses: []*sdktypes.Gateway{final},
			timeout:   5 * time.Second,
			wantReady: final,
			wantCalls: 1,
		},
		{
			name:      "times out reporting the last state when phase stays Provisioning",
			responses: []*sdktypes.Gateway{provisioningComplete},
			timeout:   20 * time.Millisecond,
			wantErrPart: []string{
				"timed out",
				`last phase="Provisioning"`,
				"generation=2",
				"observed_generation=2",
				"route_address=set",
				`phase is "Provisioning", want Running`,
			},
		},
		{
			name:        "times out reporting the conditions error",
			responses:   []*sdktypes.Gateway{runningIncomplete},
			timeout:     20 * time.Millisecond,
			wantErrPart: []string{`last phase="Running"`, `provisioning condition "GatewayHealthy" is "InProgress"`},
		},
		{
			name:        "times out with no response observed",
			responses:   []*sdktypes.Gateway{nil},
			timeout:     20 * time.Millisecond,
			wantErrPart: []string{"no gateway response observed", "transient API error"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := &scriptedGet{responses: tt.responses}
			ready, last, err := waitPrimaryGatewayReady(t.Context(), interval, tt.timeout, script.get)
			if tt.wantReady != nil {
				if err != nil {
					t.Fatalf("want ready, got %v", err)
				}
				if ready != tt.wantReady || last != tt.wantReady {
					t.Fatalf("want the exact accepted response, got ready=%p last=%p want=%p", ready, last, tt.wantReady)
				}
				if script.calls != tt.wantCalls {
					t.Fatalf("want %d reads, got %d", tt.wantCalls, script.calls)
				}
				return
			}
			if err == nil {
				t.Fatalf("want timeout, got ready response %+v", ready)
			}
			if ready != nil {
				t.Fatalf("want no ready response on timeout, got %+v", ready)
			}
			for _, part := range tt.wantErrPart {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q does not contain %q", err.Error(), part)
				}
			}
			if script.responses[len(script.responses)-1] != nil && last != script.responses[len(script.responses)-1] {
				t.Errorf("want last observed response returned on timeout")
			}
		})
	}
}
