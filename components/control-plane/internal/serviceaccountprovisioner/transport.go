package serviceaccountprovisioner

import (
	"context"
	"fmt"
	"net"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type TransportConfig struct {
	Address string
}

// ListenAndServe runs the internal provisioner until ctx is canceled. The channel
// is plaintext in-cluster gRPC; a NetworkPolicy restricts the port to the API
// server pod, so it mirrors the sibling control-plane -> api-server watch channel
// rather than adding mTLS. See
// specs/platform/openshell-gateway-service-accounts.spec.md (Internal Provisioner
// Network Isolation).
func ListenAndServe(ctx context.Context, config TransportConfig, handler *Server) error {
	if config.Address == "" {
		return fmt.Errorf("service-account provisioner bind address is required")
	}
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		return fmt.Errorf("listen for service-account provisioner: %w", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterOpenShellGatewayServiceAccountProvisionerServiceServer(grpcServer, handler)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		<-ctx.Done()
		healthServer.Shutdown()
		grpcServer.GracefulStop()
	}()
	if err := grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("serve service-account provisioner: %w", err)
	}
	return nil
}
