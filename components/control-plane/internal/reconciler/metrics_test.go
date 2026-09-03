package reconciler

import (
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
