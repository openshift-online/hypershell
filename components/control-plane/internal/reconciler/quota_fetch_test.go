package reconciler

import (
	"context"
	"errors"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeGatewayProfileClient is a hand-written test double for
// pb.GatewayProfileServiceClient. Only GetGatewayProfile is exercised by
// resolveGatewayProfileFromClient; the rest satisfy the interface.
type fakeGatewayProfileClient struct {
	resp *pb.GetGatewayProfileResponse
	err  error

	calledWithID string
}

func (f *fakeGatewayProfileClient) GetGatewayProfile(ctx context.Context, in *pb.GetGatewayProfileRequest, opts ...grpc.CallOption) (*pb.GetGatewayProfileResponse, error) {
	f.calledWithID = in.GetId()
	return f.resp, f.err
}

func (f *fakeGatewayProfileClient) CreateGatewayProfile(ctx context.Context, in *pb.CreateGatewayProfileRequest, opts ...grpc.CallOption) (*pb.CreateGatewayProfileResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeGatewayProfileClient) UpdateGatewayProfile(ctx context.Context, in *pb.UpdateGatewayProfileRequest, opts ...grpc.CallOption) (*pb.UpdateGatewayProfileResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeGatewayProfileClient) DeleteGatewayProfile(ctx context.Context, in *pb.DeleteGatewayProfileRequest, opts ...grpc.CallOption) (*pb.DeleteGatewayProfileResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeGatewayProfileClient) ListGatewayProfiles(ctx context.Context, in *pb.ListGatewayProfilesRequest, opts ...grpc.CallOption) (*pb.ListGatewayProfilesResponse, error) {
	return nil, errors.New("not implemented")
}

func strptr(s string) *string { return &s }
func i32ptr(i int32) *int32   { return &i }

func TestResolveGatewayProfileFromClient(t *testing.T) {
	t.Run("empty profile_id returns nil config and no error", func(t *testing.T) {
		client := &fakeGatewayProfileClient{err: errors.New("must not be called")}
		cfg, err := resolveGatewayProfileFromClient(context.Background(), "", client)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg != nil {
			t.Fatalf("expected nil config, got %+v", cfg)
		}
		if client.calledWithID != "" {
			t.Fatalf("expected client not to be called, got id %q", client.calledWithID)
		}
	})

	t.Run("NotFound is terminal and blocks provisioning", func(t *testing.T) {
		client := &fakeGatewayProfileClient{err: status.Error(codes.NotFound, "profile not found")}
		cfg, err := resolveGatewayProfileFromClient(context.Background(), "p-1", client)
		if err == nil {
			t.Fatal("expected an error when the profile is not found")
		}
		if cfg != nil {
			t.Fatalf("expected nil config on error, got %+v", cfg)
		}
		if !errors.Is(err, errTerminalProfile) {
			t.Fatalf("expected NotFound to be terminal, got %v", err)
		}
	})

	t.Run("transient error blocks provisioning but is not terminal", func(t *testing.T) {
		client := &fakeGatewayProfileClient{err: status.Error(codes.Unavailable, "api server down")}
		cfg, err := resolveGatewayProfileFromClient(context.Background(), "p-1", client)
		if err == nil {
			t.Fatal("expected an error when the fetch fails transiently")
		}
		if cfg != nil {
			t.Fatalf("expected nil config on error, got %+v", cfg)
		}
		if errors.Is(err, errTerminalProfile) {
			t.Fatalf("expected Unavailable to be transient, got terminal: %v", err)
		}
	})

	t.Run("non-status error is treated as transient", func(t *testing.T) {
		client := &fakeGatewayProfileClient{err: errors.New("connection reset")}
		_, err := resolveGatewayProfileFromClient(context.Background(), "p-1", client)
		if err == nil {
			t.Fatal("expected an error when the fetch fails")
		}
		if errors.Is(err, errTerminalProfile) {
			t.Fatalf("expected a non-status error to be transient, got terminal: %v", err)
		}
	})

	t.Run("nil profile in response is a terminal error", func(t *testing.T) {
		client := &fakeGatewayProfileClient{resp: &pb.GetGatewayProfileResponse{}}
		_, err := resolveGatewayProfileFromClient(context.Background(), "p-1", client)
		if err == nil {
			t.Fatal("expected an error when the response profile is nil")
		}
		if !errors.Is(err, errTerminalProfile) {
			t.Fatalf("expected empty payload to be terminal, got %v", err)
		}
	})

	t.Run("success maps all fields", func(t *testing.T) {
		client := &fakeGatewayProfileClient{resp: &pb.GetGatewayProfileResponse{
			GatewayProfile: &pb.GatewayProfile{
				CpuRequestTotal:               strptr("2"),
				CpuLimitTotal:                 strptr("4"),
				MemoryRequestTotal:            strptr("4Gi"),
				MemoryLimitTotal:              strptr("8Gi"),
				EphemeralStorageTotal:         strptr("10Gi"),
				PodCount:                      i32ptr(10),
				PvcCount:                      i32ptr(5),
				ContainerCpuRequestDefault:    strptr("100m"),
				ContainerCpuLimitMax:          strptr("2"),
				ContainerMemoryRequestDefault: strptr("128Mi"),
				ContainerMemoryLimitMax:       strptr("4Gi"),
			},
		}}
		cfg, err := resolveGatewayProfileFromClient(context.Background(), "p-1", client)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg == nil {
			t.Fatal("expected a config, got nil")
		}
		if client.calledWithID != "p-1" {
			t.Fatalf("expected client called with p-1, got %q", client.calledWithID)
		}
		if cfg.CPURequestTotal != "2" || cfg.CPULimitTotal != "4" {
			t.Fatalf("cpu fields not mapped: %+v", cfg)
		}
		if cfg.MemoryRequestTotal != "4Gi" || cfg.MemoryLimitTotal != "8Gi" {
			t.Fatalf("memory fields not mapped: %+v", cfg)
		}
		if cfg.EphemeralStorageTotal != "10Gi" {
			t.Fatalf("ephemeral storage not mapped: %+v", cfg)
		}
		if cfg.PodCount != 10 || cfg.PVCCount != 5 {
			t.Fatalf("count fields not mapped: %+v", cfg)
		}
		if cfg.ContainerCPURequestDefault != "100m" || cfg.ContainerCPULimitMax != "2" {
			t.Fatalf("container cpu fields not mapped: %+v", cfg)
		}
		if cfg.ContainerMemoryRequestDefault != "128Mi" || cfg.ContainerMemoryLimitMax != "4Gi" {
			t.Fatalf("container memory fields not mapped: %+v", cfg)
		}
	})
}
