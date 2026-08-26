package reconciler

import (
	"context"
	"strings"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"google.golang.org/grpc"
)

// fakeExposure is a stub Gateway Exposure port for driving the health
// reconciler's route-readiness decision logic.
type fakeExposure struct {
	readiness exposure.Readiness
	err       error
}

func (f fakeExposure) ResolveAddress(context.Context, exposure.Request) (string, error) {
	return "", nil
}

func (f fakeExposure) ObserveReadiness(context.Context, exposure.Request) (exposure.Readiness, error) {
	return f.readiness, f.err
}

func newHealthRec(exp exposure.Port, now func() time.Time, timeout time.Duration) *GatewayHealthReconciler {
	return &GatewayHealthReconciler{
		exposure:           exp,
		routeReadyTimeout:  timeout,
		now:                now,
		routeNotReadySince: make(map[string]time.Time),
	}
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func routedGateway(id, namespace string) *pb.Gateway {
	route := `{"host":"gw.example.com"}`
	return &pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: id},
		Namespace: namespace,
		Route:     &route,
	}
}

// selfHealConsole must be bounded: it re-reconciles the console only for routed
// gateways when Keycloak is configured. Outside those conditions it must be a
// pure no-op that never touches the (here nil) Kubernetes clients -- if it did,
// it would panic. The nil-client no-panic is the assertion.
func TestSelfHealConsole_NoOpWhenUnconfigured(t *testing.T) {
	ctx := context.Background()

	t.Run("no keycloak config", func(t *testing.T) {
		h := &GatewayHealthReconciler{exposure: fakeExposure{}} // keycloakConfig nil
		h.selfHealConsole(ctx, "gw-1", routedGateway("gw-1", "openshell-abc"))
	})

	t.Run("no exposure port", func(t *testing.T) {
		h := &GatewayHealthReconciler{keycloakConfig: &gateway.KeycloakConfig{}} // exposure nil
		h.selfHealConsole(ctx, "gw-1", routedGateway("gw-1", "openshell-abc"))
	})

	t.Run("not a routed gateway", func(t *testing.T) {
		h := &GatewayHealthReconciler{exposure: fakeExposure{}, keycloakConfig: &gateway.KeycloakConfig{}}
		h.selfHealConsole(ctx, "gw-1", &pb.Gateway{Metadata: &pb.ObjectReference{Id: "gw-1"}, Namespace: "openshell-abc"})
	})
}

// teardownRoute reconciles the desired absence of the route and console for a
// non-routed gateway. It must return before touching the (here nil) Kubernetes
// clients when the gateway carries no namespace, so a malformed gateway can never
// panic the health loop. The nil-client no-panic is the assertion.
func TestTeardownRoute_NoOpWithoutNamespace(t *testing.T) {
	h := &GatewayHealthReconciler{
		keycloakConfig: &gateway.KeycloakConfig{},
		routeTornDown:  make(map[string]bool),
	}
	// No Namespace set: gatewayNamespace errors, so teardownRoute must return
	// before dereferencing the nil clientset/dynamicClient.
	h.teardownRoute(context.Background(), &fakeGatewayClient{}, "gw-1",
		&pb.Gateway{Metadata: &pb.ObjectReference{Id: "gw-1"}})
}

// Observing a routed gateway must clear any recorded teardown so a later
// un-routing triggers a fresh teardown rather than being skipped forever.
func TestClearRouteTornDown_AllowsFreshTeardown(t *testing.T) {
	h := &GatewayHealthReconciler{routeTornDown: map[string]bool{"gw-1": true}}
	if !h.routeTornDownAlready("gw-1") {
		t.Fatal("precondition: gw-1 should start marked as torn down")
	}
	h.clearRouteTornDown("gw-1")
	if h.routeTornDownAlready("gw-1") {
		t.Fatal("clearRouteTornDown must forget the gateway so teardown can re-run")
	}
}

// teardownSettled must trust the completion marker only while the Gateway record
// carries no route or console address. A console-address publisher started during
// provisioning runs on the uncancelled watch context and can write console_address
// after teardown marked itself complete; if that happens the marker must be
// ignored so the next tick re-runs teardown and clears the stale address.
func TestTeardownSettled(t *testing.T) {
	addr := func(s string) *string { return &s }
	cases := []struct {
		name        string
		marked      bool
		routeAddr   *string
		consoleAddr *string
		wantSettled bool
	}{
		{"not marked", false, nil, nil, false},
		{"marked, no addresses", true, nil, nil, true},
		{"marked, empty addresses", true, addr(""), addr(""), true},
		{"marked, stale console address", true, nil, addr("https://console.example"), false},
		{"marked, stale route address", true, addr("gw.example"), nil, false},
		{"not marked, stale console address", false, nil, addr("https://console.example"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := &GatewayHealthReconciler{routeTornDown: map[string]bool{}}
			if c.marked {
				h.routeTornDown["gw-1"] = true
			}
			gw := &pb.Gateway{
				Metadata:       &pb.ObjectReference{Id: "gw-1"},
				RouteAddress:   c.routeAddr,
				ConsoleAddress: c.consoleAddr,
			}
			if got := h.teardownSettled("gw-1", gw); got != c.wantSettled {
				t.Fatalf("teardownSettled = %v, want %v", got, c.wantSettled)
			}
		})
	}
}

// The residual-absence probe must be indefinite but low-frequency: a settled
// gateway is re-verified at most once per routeVerifyInterval, and -- unlike a
// wall-clock window -- it never stops being verified, because elapsed time is not
// proof a stale provision cannot still resurrect resources.
func TestDueForVerify(t *testing.T) {
	base := time.Unix(1000, 0)
	cur := base
	h := &GatewayHealthReconciler{
		now:             func() time.Time { return cur },
		routeTornDown:   make(map[string]bool),
		routeVerifiedAt: make(map[string]time.Time),
	}

	// No recorded verification: verify defensively rather than trust an unstamped
	// marker.
	if !h.dueForVerify("gw-1") {
		t.Fatal("want due=true when no verification time is recorded")
	}

	// markRouteTornDown stamps the baseline; immediately after, we are within the
	// interval, so no re-probe is due yet.
	h.markRouteTornDown("gw-1")
	if h.dueForVerify("gw-1") {
		t.Fatal("want due=false immediately after teardown")
	}

	// Just inside the interval: still not due.
	cur = base.Add(routeVerifyInterval - time.Second)
	if h.dueForVerify("gw-1") {
		t.Fatal("want due=false just inside the interval")
	}

	// At/after the interval: due again -- verification is indefinite, not a cutoff.
	cur = base.Add(routeVerifyInterval)
	if !h.dueForVerify("gw-1") {
		t.Fatal("want due=true once the interval has elapsed")
	}

	// A successful verification re-stamps the timestamp, deferring the next probe by
	// another interval (proves it keeps probing forever, just not every tick).
	h.markVerified("gw-1")
	if h.dueForVerify("gw-1") {
		t.Fatal("want due=false immediately after a verification")
	}
	cur = base.Add(routeVerifyInterval + routeVerifyInterval)
	if !h.dueForVerify("gw-1") {
		t.Fatal("want due=true an interval after the last verification")
	}

	// Clearing the marker (gateway routed again) forgets the recorded time.
	h.clearRouteTornDown("gw-1")
	if _, ok := h.routeVerifiedAt["gw-1"]; ok {
		t.Fatal("clearRouteTornDown must forget the recorded verification time")
	}
}

func TestEvaluateRouteReadiness_ReadyBecomesRunning(t *testing.T) {
	h := newHealthRec(fakeExposure{readiness: exposure.Readiness{Ready: true}}, fixedClock(time.Unix(0, 0)), 10*time.Minute)
	for _, phase := range []string{"Provisioning", "Degraded"} {
		gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", phase)
		if gotPhase != "Running" || gotStatus != "Healthy" {
			t.Fatalf("from %s: got (%q,%q), want (Running,Healthy)", phase, gotPhase, gotStatus)
		}
	}
}

func TestEvaluateRouteReadiness_ProvisioningWithinGraceStaysProvisioning(t *testing.T) {
	h := newHealthRec(
		fakeExposure{readiness: exposure.Readiness{Ready: false, Reason: "gateway not programmed: Pending"}},
		fixedClock(time.Unix(1000, 0)),
		10*time.Minute,
	)
	gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Provisioning")
	if gotPhase != "Provisioning" {
		t.Fatalf("got phase %q, want Provisioning", gotPhase)
	}
	if gotStatus != "gateway not programmed: Pending" {
		t.Fatalf("got status %q, want the exposure reason", gotStatus)
	}
}

func TestEvaluateRouteReadiness_ProvisioningBeyondGraceBecomesDegraded(t *testing.T) {
	base := time.Unix(1000, 0)
	cur := base
	h := newHealthRec(
		fakeExposure{readiness: exposure.Readiness{Ready: false, Reason: "gateway not programmed: Pending"}},
		func() time.Time { return cur },
		10*time.Minute,
	)

	// First observation starts the grace window.
	if gotPhase, _ := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Provisioning"); gotPhase != "Provisioning" {
		t.Fatalf("first tick: got %q, want Provisioning", gotPhase)
	}

	// Advance past the grace window; the gateway must move to Degraded.
	cur = base.Add(11 * time.Minute)
	gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Provisioning")
	if gotPhase != "Degraded" {
		t.Fatalf("after grace: got %q, want Degraded", gotPhase)
	}
	if !strings.Contains(gotStatus, "route not ready after") {
		t.Fatalf("status %q should record the grace-window expiry", gotStatus)
	}
}

func TestEvaluateRouteReadiness_RunningLosesReadinessBecomesDegraded(t *testing.T) {
	h := newHealthRec(
		fakeExposure{readiness: exposure.Readiness{Ready: false, Reason: "gateway has no assigned address"}},
		fixedClock(time.Unix(0, 0)),
		10*time.Minute,
	)
	// A Running gateway that loses readiness is Degraded immediately, with no
	// grace window.
	gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Running")
	if gotPhase != "Degraded" {
		t.Fatalf("got %q, want Degraded", gotPhase)
	}
	if gotStatus != "gateway has no assigned address" {
		t.Fatalf("got status %q, want the exposure reason", gotStatus)
	}
}

func TestEvaluateRouteReadiness_DegradedStaysDegraded(t *testing.T) {
	h := newHealthRec(
		fakeExposure{readiness: exposure.Readiness{Ready: false, Reason: "gateway not programmed: Pending"}},
		fixedClock(time.Unix(0, 0)),
		10*time.Minute,
	)
	if gotPhase, _ := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Degraded"); gotPhase != "Degraded" {
		t.Fatalf("got %q, want Degraded", gotPhase)
	}
}

func TestEvaluateRouteReadiness_ObserveErrorLeavesPhaseUntouched(t *testing.T) {
	h := newHealthRec(
		fakeExposure{err: context.DeadlineExceeded},
		fixedClock(time.Unix(0, 0)),
		10*time.Minute,
	)
	gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Provisioning")
	if gotPhase != "" || gotStatus != "" {
		t.Fatalf("on observe error got (%q,%q), want empty (leave untouched)", gotPhase, gotStatus)
	}
}

func TestIsRoutedGateway(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name  string
		route *string
		want  bool
	}{
		{"nil route", nil, false},
		{"empty route", str(""), false},
		{"whitespace route", str("  "), false},
		{"null route", str("null"), false},
		{"empty object", str("{}"), true},
		{"enabled route", str(`{"enabled":true}`), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isRoutedGateway(&pb.Gateway{Route: c.route}); got != c.want {
				t.Fatalf("isRoutedGateway(%v) = %v, want %v", c.route, got, c.want)
			}
		})
	}
}

type fakeGatewayClient struct {
	pb.GatewayServiceClient
	listFn func(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error)
}

func (f *fakeGatewayClient) ListGateways(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
	if f.listFn != nil {
		return f.listFn(ctx, in, opts...)
	}
	return &pb.ListGatewaysResponse{}, nil
}

func TestListAllGateways_Pagination(t *testing.T) {
	// 250 gateways distributed across 3 pages (100, 100, 50).
	total := 250
	allGWs := make([]*pb.Gateway, total)
	for i := 0; i < total; i++ {
		allGWs[i] = &pb.Gateway{
			Metadata: &pb.ObjectReference{Id: strings.Repeat("a", 10) + string(rune(i))},
			Name:     "gw-" + strings.Repeat("x", 5),
		}
	}

	var requestedPages []int32
	client := &fakeGatewayClient{
		listFn: func(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
			requestedPages = append(requestedPages, in.Page)
			pageSize := int(in.Size)
			start := (int(in.Page) - 1) * pageSize
			if start >= total {
				return &pb.ListGatewaysResponse{
					Metadata: &pb.ListMeta{Page: in.Page, Size: in.Size, Total: int32(total)},
				}, nil
			}
			end := start + pageSize
			if end > total {
				end = total
			}
			return &pb.ListGatewaysResponse{
				Items:    allGWs[start:end],
				Metadata: &pb.ListMeta{Page: in.Page, Size: in.Size, Total: int32(total)},
			}, nil
		},
	}

	h := &GatewayHealthReconciler{}
	got, err := h.listAllGateways(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != total {
		t.Fatalf("got %d gateways, want %d", len(got), total)
	}
	for i, gw := range got {
		if gw.GetMetadata().GetId() != allGWs[i].GetMetadata().GetId() {
			t.Fatalf("gateway[%d] = %q, want %q",
				i, gw.GetMetadata().GetId(), allGWs[i].GetMetadata().GetId())
		}
	}

	expectedPages := []int32{1, 2, 3}
	if len(requestedPages) != len(expectedPages) {
		t.Fatalf("got requested pages %v, want %v", requestedPages, expectedPages)
	}
	for i, p := range expectedPages {
		if requestedPages[i] != p {
			t.Errorf("page[%d] = %d, want %d", i, requestedPages[i], p)
		}
	}
}

func TestListAllGateways_SinglePage(t *testing.T) {
	total := 5
	items := make([]*pb.Gateway, total)
	for i := 0; i < total; i++ {
		items[i] = &pb.Gateway{
			Metadata: &pb.ObjectReference{Id: "id"},
		}
	}

	var callCount int
	client := &fakeGatewayClient{
		listFn: func(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
			callCount++
			return &pb.ListGatewaysResponse{
				Items:    items,
				Metadata: &pb.ListMeta{Page: 1, Size: in.Size, Total: int32(total)},
			}, nil
		},
	}

	h := &GatewayHealthReconciler{}
	got, err := h.listAllGateways(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != total {
		t.Fatalf("got %d gateways, want %d", len(got), total)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}
}

func TestListAllGateways_Empty(t *testing.T) {
	var callCount int
	client := &fakeGatewayClient{
		listFn: func(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
			callCount++
			return &pb.ListGatewaysResponse{
				Items:    nil,
				Metadata: &pb.ListMeta{Page: 1, Size: in.Size, Total: 0},
			}, nil
		},
	}

	h := &GatewayHealthReconciler{}
	got, err := h.listAllGateways(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d gateways, want 0", len(got))
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}
}
