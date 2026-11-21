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

func (p *controlPlaneProvisioner) ListWorkspaceMemberships(ctx context.Context, input Input) ([]WorkspaceMembership, error) {
	visibility, err := p.ListWorkspaceVisibility(ctx, input)
	if err != nil {
		return nil, err
	}
	return visibility.Memberships, nil
}

func (p *controlPlaneProvisioner) ListWorkspaceVisibility(ctx context.Context, input Input) (WorkspaceVisibility, error) {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	response, err := p.client.ListWorkspaceMemberships(callCtx, &pb.ListWorkspaceMembershipsRequest{
		GatewayId: input.GatewayID, GatewayNamespace: input.GatewayNamespace,
		GatewayEndpoint: input.GatewayEndpoint, Issuer: input.Issuer,
		GatewayClientId: input.GatewayClientID,
	})
	if status.Code(err) == codes.Unimplemented {
		return WorkspaceVisibility{}, errUnsupportedControlPlane
	}
	if err != nil {
		return WorkspaceVisibility{}, err
	}
	visibility := WorkspaceVisibility{
		Workspaces:  append([]string(nil), response.GetWorkspaces()...),
		Memberships: make([]WorkspaceMembership, 0, len(response.GetMemberships())),
	}
	for _, membership := range response.GetMemberships() {
		visibility.Memberships = append(visibility.Memberships, WorkspaceMembership{
			Workspace: membership.GetWorkspace(), Subject: membership.GetSubject(),
		})
	}
	return visibility, nil
}

func provisionerCallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, defaultProvisionerCallTimeout)
}
