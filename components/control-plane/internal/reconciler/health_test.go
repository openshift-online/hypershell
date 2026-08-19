package reconciler

import (
	"context"
	"strings"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
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
