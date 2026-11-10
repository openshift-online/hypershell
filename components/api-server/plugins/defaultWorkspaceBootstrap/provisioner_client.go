package defaultWorkspaceBootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const defaultProvisionerCallTimeout = 30 * time.Second

var errUnsupportedControlPlane = errors.New("control plane does not support default-workspace bootstrap")

type controlPlaneProvisioner struct {
	client pb.OpenShellGatewayServiceAccountProvisionerServiceClient
}

func newControlPlaneProvisionerFromEnvironment() (Provisioner, error) {
	address := os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR")
	if address == "" {
		return nil, nil
	}
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("create default-workspace provisioner client: %w", err)
	}
	return &controlPlaneProvisioner{client: pb.NewOpenShellGatewayServiceAccountProvisionerServiceClient(connection)}, nil
}

func (p *controlPlaneProvisioner) Configured() bool {
	return p != nil && p.client != nil
}

func (p *controlPlaneProvisioner) BootstrapDefaultWorkspace(ctx context.Context, input Input) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.BootstrapDefaultWorkspace(callCtx, &pb.BootstrapDefaultWorkspaceRequest{
		GatewayId: input.GatewayID, GatewayNamespace: input.GatewayNamespace,
		GatewayEndpoint: input.GatewayEndpoint, Issuer: input.Issuer,
		GatewayClientId: input.GatewayClientID, Subject: input.Subject,
		SubjectType: input.SubjectType, Role: input.Role,
	})
	if status.Code(err) == codes.Unimplemented {
		return errUnsupportedControlPlane
	}
	return err
}

func provisionerCallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, defaultProvisionerCallTimeout)
}
