package reconciler

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGatewayProvisionDuration(t *testing.T) {
	runningAt := time.Date(2026, time.September, 3, 20, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		gateway   *pb.Gateway
		want      time.Duration
		wantValid bool
	}{
		{
			name: "valid creation time",
			gateway: &pb.Gateway{Metadata: &pb.ObjectReference{
				CreatedAt: timestamppb.New(runningAt.Add(-2 * time.Minute)),
				UpdatedAt: timestamppb.New(runningAt),
			}},
			want:      2 * time.Minute,
			wantValid: true,
		},
		{name: "missing gateway", gateway: nil},
		{name: "missing metadata", gateway: &pb.Gateway{}},
		{
			name: "missing creation time",
			gateway: &pb.Gateway{Metadata: &pb.ObjectReference{
				UpdatedAt: timestamppb.New(runningAt),
			}},
		},
		{
			name: "missing update time",
			gateway: &pb.Gateway{Metadata: &pb.ObjectReference{
				CreatedAt: timestamppb.New(runningAt.Add(-2 * time.Minute)),
			}},
		},
		{
			name: "invalid creation time",
			gateway: &pb.Gateway{Metadata: &pb.ObjectReference{
				CreatedAt: &timestamppb.Timestamp{Seconds: 253402300800},
				UpdatedAt: timestamppb.New(runningAt),
			}},
		},
		{
			name: "invalid update time",
			gateway: &pb.Gateway{Metadata: &pb.ObjectReference{
				CreatedAt: timestamppb.New(runningAt.Add(-2 * time.Minute)),
				UpdatedAt: &timestamppb.Timestamp{Seconds: 253402300800},
			}},
		},
		{
			name: "update before creation",
			gateway: &pb.Gateway{Metadata: &pb.ObjectReference{
				CreatedAt: timestamppb.New(runningAt.Add(time.Minute)),
				UpdatedAt: timestamppb.New(runningAt),
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := gatewayProvisionDuration(test.gateway)
			if valid != test.wantValid || got != test.want {
				t.Fatalf("gatewayProvisionDuration() = (%s, %v), want (%s, %v)", got, valid, test.want, test.wantValid)
			}
		})
	}
}

func TestIsGatewayProvisionCompletion(t *testing.T) {
	tests := []struct {
		current string
		desired string
		want    bool
	}{
		{current: "Provisioning", desired: "Running", want: true},
		{current: "Degraded", desired: "Running", want: false},
		{current: "Running", desired: "Running", want: false},
		{current: "Provisioning", desired: "Degraded", want: false},
	}

	for _, test := range tests {
		if got := isGatewayProvisionCompletion(test.current, test.desired); got != test.want {
			t.Errorf("isGatewayProvisionCompletion(%q, %q) = %v, want %v", test.current, test.desired, got, test.want)
		}
	}
}

func TestClaimGatewayProvisionObservation(t *testing.T) {
	const gatewayID = "gateway-provision-observation-test"
	forgetGatewayProvisionObservation(gatewayID)
	t.Cleanup(func() { forgetGatewayProvisionObservation(gatewayID) })

	var claims atomic.Int32
	var group sync.WaitGroup
	for range 100 {
		group.Add(1)
		go func() {
			defer group.Done()
			if claimGatewayProvisionObservation(gatewayID) {
				claims.Add(1)
			}
		}()
	}
	group.Wait()

	if got := claims.Load(); got != 1 {
		t.Fatalf("successful claims = %d, want 1", got)
	}
	if claimGatewayProvisionObservation("") {
		t.Fatal("empty Gateway identifier was accepted")
	}

	forgetGatewayProvisionObservation(gatewayID)
	if !claimGatewayProvisionObservation(gatewayID) {
		t.Fatal("claim after Gateway deletion was rejected")
	}
}

func TestSuppressGatewayProvisionObservation(t *testing.T) {
	tests := []struct {
		phase      string
		suppressed bool
	}{
		{phase: "Running", suppressed: true},
		{phase: "Degraded", suppressed: true},
		{phase: "Provisioning", suppressed: false},
		{phase: "Failed", suppressed: false},
		{phase: "", suppressed: false},
	}

	for _, test := range tests {
		t.Run(test.phase, func(t *testing.T) {
			gatewayID := "gateway-recovery-" + test.phase
			forgetGatewayProvisionObservation(gatewayID)
			t.Cleanup(func() { forgetGatewayProvisionObservation(gatewayID) })

			suppressGatewayProvisionObservation(gatewayID, test.phase)
			claimed := claimGatewayProvisionObservation(gatewayID)
			if got := !claimed; got != test.suppressed {
				t.Fatalf("suppressed = %v, want %v", got, test.suppressed)
			}
		})
	}
}
