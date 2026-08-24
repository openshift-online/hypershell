package serviceAccounts

import (
	"context"
	"fmt"
	"os"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const defaultProvisionerCallTimeout = 60 * time.Second

type controlPlaneProvisioner struct {
	client pb.OpenShellGatewayServiceAccountProvisionerServiceClient
}

// newControlPlaneProvisionerFromEnvironment dials the control-plane provisioner
// over plaintext in-cluster gRPC. The channel is not exposed outside the cluster
// and a NetworkPolicy restricts its port to the API server pod, so it mirrors the
// sibling control-plane -> api-server watch channel rather than adding mTLS. See
// specs/platform/openshell-gateway-service-accounts.spec.md (Internal Provisioner
// Network Isolation).
func newControlPlaneProvisionerFromEnvironment() (ServiceAccountProvisioner, error) {
	address := os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR")
	if address == "" {
		return nil, nil
	}
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("create control-plane provisioner client: %w", err)
	}
	return &controlPlaneProvisioner{client: pb.NewOpenShellGatewayServiceAccountProvisionerServiceClient(connection)}, nil
}

func (p *controlPlaneProvisioner) Configured() bool {
	return p != nil && p.client != nil
}

func (p *controlPlaneProvisioner) ProvisionServiceAccount(ctx context.Context, spec ProvisioningSpec) (*ProvisionedServiceAccount, error) {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	response, err := p.client.Provision(callCtx, &pb.ProvisionRequest{Spec: provisioningSpecToProto(spec)})
	if err != nil {
		return nil, mapProvisionerError(err)
	}
	return &ProvisionedServiceAccount{
		ClientUUID: response.GetClientUuid(), ClientID: response.GetClientId(),
		ClientSecret: response.GetClientSecret(), Subject: response.GetSubject(),
	}, nil
}

func (p *controlPlaneProvisioner) ReconcileServiceAccount(ctx context.Context, spec ProvisioningSpec, clientUUID, expectedSubject string, enabled bool) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.Reconcile(callCtx, &pb.ReconcileRequest{
		Spec: provisioningSpecToProto(spec), ClientUuid: clientUUID,
		ExpectedSubject: expectedSubject, Enabled: enabled,
	})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) DisableServiceAccount(ctx context.Context, clientUUID, gatewayID, serviceAccountID string) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.Disable(callCtx, &pb.DisableRequest{
		ClientUuid: clientUUID, GatewayId: gatewayID, ServiceAccountId: serviceAccountID,
	})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) DeleteServiceAccount(ctx context.Context, clientUUID, gatewayID, serviceAccountID string) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.Delete(callCtx, &pb.DeleteRequest{
		ClientUuid: clientUUID, GatewayId: gatewayID, ServiceAccountId: serviceAccountID,
	})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) DeleteManagedServiceAccount(ctx context.Context, gatewayID, serviceAccountID string) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.DeleteManaged(callCtx, &pb.DeleteManagedRequest{
		GatewayId: gatewayID, ServiceAccountId: serviceAccountID,
	})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) DeleteGatewayServiceAccounts(ctx context.Context, gatewayID string) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.DeleteGateway(callCtx, &pb.DeleteGatewayRequest{GatewayId: gatewayID})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) ListManagedClients(ctx context.Context, gatewayID string) ([]ManagedClient, error) {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	response, err := p.client.ListManaged(callCtx, &pb.ListManagedRequest{GatewayId: gatewayID})
	if err != nil {
		return nil, mapProvisionerError(err)
	}
	clients := make([]ManagedClient, 0, len(response.GetClients()))
	for _, client := range response.GetClients() {
		clients = append(clients, ManagedClient{
			UUID: client.GetUuid(), ClientID: client.GetClientId(), GatewayID: client.GetGatewayId(),
			ServiceAccountID: client.GetServiceAccountId(),
		})
	}
	return clients, nil
}

func provisioningSpecToProto(spec ProvisioningSpec) *pb.ServiceAccountSpec {
	return &pb.ServiceAccountSpec{
		ClientId: spec.ClientID, DisplayName: spec.DisplayName,
		GatewayClientId: spec.GatewayClientID, GatewayId: spec.GatewayID,
		ServiceAccountId: spec.ServiceAccountID, CreatorUserId: spec.CreatorUserID,
		Role: spec.Role, ExpectedIssuer: spec.ExpectedIssuer,
		AccessTokenLifetimeSeconds: int32(spec.AccessTokenLifetimeSeconds),
	}
}

func provisionerCallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, defaultProvisionerCallTimeout)
}

func mapProvisionerError(err error) error {
	if err == nil {
		return nil
	}
	if status.Code(err) == codes.NotFound {
		return fmt.Errorf("%w: %v", ErrProvisionerNotFound, err)
	}
	return fmt.Errorf("control-plane provisioner request failed: %w", err)
}
