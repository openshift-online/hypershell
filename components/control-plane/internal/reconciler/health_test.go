package reconciler

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"google.golang.org/grpc"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
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
		h := &GatewayHealthReconciler{ingressMode: gateway.IngressModeRoute} // keycloakConfig nil
		h.selfHealConsole(ctx, "gw-1", routedGateway("gw-1", "openshell-abc"))
	})

	t.Run("no selected ingress", func(t *testing.T) {
		h := &GatewayHealthReconciler{keycloakConfig: &gateway.KeycloakConfig{}}
		h.selfHealConsole(ctx, "gw-1", routedGateway("gw-1", "openshell-abc"))
	})

	t.Run("not a routed gateway", func(t *testing.T) {
		h := &GatewayHealthReconciler{ingressMode: gateway.IngressModeRoute, keycloakConfig: &gateway.KeycloakConfig{}}
		h.selfHealConsole(ctx, "gw-1", &pb.Gateway{Metadata: &pb.ObjectReference{Id: "gw-1"}, Namespace: "openshell-abc"})
	})

	t.Run("route explicitly disabled", func(t *testing.T) {
		route := `{"enabled":false}`
		h := &GatewayHealthReconciler{ingressMode: gateway.IngressModeRoute, keycloakConfig: &gateway.KeycloakConfig{}}
		h.selfHealConsole(ctx, "gw-1", &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: "gw-1"},
			Namespace: "openshell-abc",
			Route:     &route,
		})
	})
}

func TestSyncConsoleAddressUsesSelectedRouteExposure(t *testing.T) {
	t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
	const namespace = "openshell-abc"

	for _, tc := range []struct {
		name        string
		admitted    bool
		current     string
		wantReady   bool
		wantAddress string
	}{
		{
			name:        "admitted Route publishes the address",
			admitted:    true,
			wantReady:   true,
			wantAddress: "https://console-openshell-abc.apps.example.com",
		},
		{
			name:        "rejected Route clears the address",
			current:     "https://console-openshell-abc.apps.example.com",
			wantAddress: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replicas := int32(1)
			clientset := k8sfake.NewSimpleClientset(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: gateway.ConsoleDeploymentName, Namespace: namespace},
				Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
				Status:     appsv1.DeploymentStatus{ReadyReplicas: replicas},
			})

			status := "False"
			reason := "HostAlreadyClaimed"
			if tc.admitted {
				status = "True"
				reason = ""
			}
			route := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "route.openshift.io/v1",
				"kind":       "Route",
				"metadata": map[string]interface{}{
					"name":      gateway.ConsoleDeploymentName,
					"namespace": namespace,
				},
				"status": map[string]interface{}{
					"ingress": []interface{}{
						map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{"type": "Admitted", "status": status, "reason": reason},
							},
						},
					},
				},
			}}
			routeGVR := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
				runtime.NewScheme(),
				map[schema.GroupVersionResource]string{routeGVR: "RouteList"},
				route,
			)

			var gotAddress string
			updates := 0
			client := &fakeGatewayClient{updateFn: func(_ context.Context, request *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
				updates++
				if request.ConsoleAddress == nil {
					t.Fatal("console address update is nil")
				}
				gotAddress = *request.ConsoleAddress
				return &pb.UpdateGatewayResponse{}, nil
			}}
			gw := routedGateway("gw-1", namespace)
			gw.ConsoleAddress = &tc.current

			ready := syncConsoleAddress(context.Background(), clientset, dynamicClient, client, "gw-1", gw, gateway.IngressModeRoute)
			if ready != tc.wantReady {
				t.Fatalf("ready = %v, want %v", ready, tc.wantReady)
			}
			if gotAddress != tc.wantAddress {
				t.Fatalf("console address = %q, want %q", gotAddress, tc.wantAddress)
			}
			if updates != 1 {
				t.Fatalf("address updates = %d, want 1", updates)
			}
		})
	}
}

func TestSyncConsoleAddressClearsAddressWithoutBaseDomain(t *testing.T) {
	t.Setenv("GATEWAY_API_BASE_DOMAIN", "")
	current := "https://console-openshell-abc.apps.example.com"
	gw := routedGateway("gw-1", "openshell-abc")
	gw.ConsoleAddress = &current

	var gotAddress string
	client := &fakeGatewayClient{updateFn: func(_ context.Context, request *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
		if request.ConsoleAddress == nil {
			t.Fatal("console address update is nil")
		}
		gotAddress = *request.ConsoleAddress
		return &pb.UpdateGatewayResponse{}, nil
	}}

	ready := syncConsoleAddress(
		context.Background(),
		k8sfake.NewSimpleClientset(),
		nil,
		client,
		"gw-1",
		gw,
		gateway.IngressModeRoute,
	)
	if ready {
		t.Fatal("ready = true, want false without a base domain")
	}
	if gotAddress != "" {
		t.Fatalf("console address = %q, want empty", gotAddress)
	}
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
		{"disabled route", str(`{"enabled":false}`), false},
		{"host-only route", str(`{"host":"gateway.example.com"}`), true},
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
	listFn   func(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error)
	updateFn func(ctx context.Context, in *pb.UpdateGatewayRequest, opts ...grpc.CallOption) (*pb.UpdateGatewayResponse, error)
}

func (f *fakeGatewayClient) ListGateways(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
	if f.listFn != nil {
		return f.listFn(ctx, in, opts...)
	}
	return &pb.ListGatewaysResponse{}, nil
}

func (f *fakeGatewayClient) UpdateGateway(ctx context.Context, in *pb.UpdateGatewayRequest, opts ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
	if f.updateFn != nil {
		return f.updateFn(ctx, in, opts...)
	}
	return &pb.UpdateGatewayResponse{}, nil
}

func TestRunGatewayWorkersDoesNotBlockOtherGateways(t *testing.T) {
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	processed := make(chan string, 3)
	finished := make(chan struct{})

	gateways := []*pb.Gateway{
		{Metadata: &pb.ObjectReference{Id: "slow"}},
		{Metadata: &pb.ObjectReference{Id: "fast"}},
		{Metadata: &pb.ObjectReference{Id: "fast"}},
	}
	go func() {
		runGatewayWorkers(context.Background(), gateways, func(gatewayRecord *pb.Gateway) {
			gatewayID := gatewayRecord.GetMetadata().GetId()
			if gatewayID == "slow" {
				close(slowStarted)
				<-releaseSlow
			}
			processed <- gatewayID
		})
		close(finished)
	}()

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("slow gateway did not start")
	}
	select {
	case gatewayID := <-processed:
		if gatewayID != "fast" {
			t.Fatalf("first completed gateway = %q, want fast", gatewayID)
		}
	case <-time.After(time.Second):
		t.Fatal("fast gateway was blocked by slow gateway")
	}

	close(releaseSlow)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("gateway workers did not finish")
	}

	if gatewayID := <-processed; gatewayID != "slow" {
		t.Fatalf("second completed gateway = %q, want slow", gatewayID)
	}
	select {
	case gatewayID := <-processed:
		t.Fatalf("duplicate gateway was processed: %q", gatewayID)
	default:
	}
}

func TestRunGatewayWorkersBoundsConcurrency(t *testing.T) {
	gateways := make([]*pb.Gateway, gatewayHealthWorkerCount+2)
	for i := range gateways {
		gateways[i] = &pb.Gateway{Metadata: &pb.ObjectReference{Id: string(rune('a' + i))}}
	}

	started := make(chan struct{}, len(gateways))
	release := make(chan struct{})
	finished := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	go func() {
		runGatewayWorkers(context.Background(), gateways, func(*pb.Gateway) {
			current := active.Add(1)
			for {
				previous := maximum.Load()
				if current <= previous || maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
		})
		close(finished)
	}()

	for range gatewayHealthWorkerCount {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("expected worker did not start")
		}
	}
	select {
	case <-started:
		t.Fatalf("more than %d workers ran at the same time", gatewayHealthWorkerCount)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("gateway workers did not finish")
	}
	if got := maximum.Load(); got != gatewayHealthWorkerCount {
		t.Fatalf("maximum workers = %d, want %d", got, gatewayHealthWorkerCount)
	}
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
