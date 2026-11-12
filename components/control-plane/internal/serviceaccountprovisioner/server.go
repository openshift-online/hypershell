// Package serviceaccountprovisioner exposes the control plane's Keycloak
// service-account operations to the API server over a private in-cluster gRPC
// API. A NetworkPolicy restricts the port to the API server pod; the channel is
// not exposed outside the cluster and does not use mTLS.
package serviceaccountprovisioner

import (
	"context"
	"errors"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/defaultworkspace"
	"github.com/openshift-online/hypershell/components/control-plane/internal/serviceaccountkeycloak"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedOpenShellGatewayServiceAccountProvisionerServiceServer
	provider     Provider
	bootstrapper DefaultWorkspaceBootstrapper
}

type DefaultWorkspaceBootstrapper interface {
	Configured() bool
	AddMember(context.Context, defaultworkspace.MemberRequest) error
}

type Provider interface {
	Configured() bool
	ProvisionServiceAccount(context.Context, serviceaccountkeycloak.ServiceAccountSpec) (*serviceaccountkeycloak.ProvisionedServiceAccount, error)
	ReconcileServiceAccount(context.Context, serviceaccountkeycloak.ServiceAccountSpec, string, string, bool) error
	DisableServiceAccount(context.Context, string, string, string) error
	DeleteServiceAccount(context.Context, string, string, string) error
	DeleteManagedServiceAccount(context.Context, string, string) error
	DeleteGatewayServiceAccounts(context.Context, string) error
	ListManagedClients(context.Context, string) ([]serviceaccountkeycloak.ManagedClient, error)
}

func NewServer(provider Provider) *Server {
	return &Server{provider: provider}
}

func NewServerWithDefaultWorkspace(provider Provider, bootstrapper DefaultWorkspaceBootstrapper) *Server {
	return &Server{provider: provider, bootstrapper: bootstrapper}
}

func (s *Server) Provision(ctx context.Context, request *pb.ProvisionRequest) (*pb.ProvisionResponse, error) {
	if problem := s.available(); problem != nil {
		return nil, problem
	}
	if request.GetSpec() == nil {
		return nil, status.Error(codes.InvalidArgument, "service-account specification is required")
	}
	provisioned, err := s.provider.ProvisionServiceAccount(ctx, specFromProto(request.GetSpec()))
	if err != nil {
		return nil, providerError(err)
	}
	// ClientSecret is deliberately returned only by this synchronous method. Do
	// not log, persist, trace, or attach this response to a retryable queue.
	return &pb.ProvisionResponse{
		ClientUuid: provisioned.ClientUUID, ClientId: provisioned.ClientID,
		ClientSecret: provisioned.ClientSecret, Subject: provisioned.Subject,
	}, nil
}

func (s *Server) Reconcile(ctx context.Context, request *pb.ReconcileRequest) (*pb.ReconcileResponse, error) {
	if problem := s.available(); problem != nil {
		return nil, problem
	}
	if request.GetSpec() == nil || request.GetClientUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "service-account specification and client UUID are required")
	}
	err := s.provider.ReconcileServiceAccount(
		ctx, specFromProto(request.GetSpec()), request.GetClientUuid(),
		request.GetExpectedSubject(), request.GetEnabled(),
	)
	if err != nil {
		return nil, providerError(err)
	}
	return &pb.ReconcileResponse{}, nil
}

func (s *Server) Disable(ctx context.Context, request *pb.DisableRequest) (*pb.DisableResponse, error) {
	if problem := s.available(); problem != nil {
		return nil, problem
	}
	if request.GetClientUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "client UUID is required")
	}
	if err := s.provider.DisableServiceAccount(ctx, request.GetClientUuid(), request.GetGatewayId(), request.GetServiceAccountId()); err != nil {
		return nil, providerError(err)
	}
	return &pb.DisableResponse{}, nil
}

func (s *Server) Delete(ctx context.Context, request *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	if problem := s.available(); problem != nil {
		return nil, problem
	}
	if request.GetClientUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "client UUID is required")
	}
	if err := s.provider.DeleteServiceAccount(ctx, request.GetClientUuid(), request.GetGatewayId(), request.GetServiceAccountId()); err != nil {
		return nil, providerError(err)
	}
	return &pb.DeleteResponse{}, nil
}

func (s *Server) DeleteManaged(ctx context.Context, request *pb.DeleteManagedRequest) (*pb.DeleteManagedResponse, error) {
	if problem := s.available(); problem != nil {
		return nil, problem
	}
	if request.GetGatewayId() == "" || request.GetServiceAccountId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway ID and service-account ID are required")
	}
	if err := s.provider.DeleteManagedServiceAccount(ctx, request.GetGatewayId(), request.GetServiceAccountId()); err != nil {
		return nil, providerError(err)
	}
	return &pb.DeleteManagedResponse{}, nil
}

func (s *Server) DeleteGateway(ctx context.Context, request *pb.DeleteGatewayRequest) (*pb.DeleteGatewayResponse, error) {
	if problem := s.available(); problem != nil {
		return nil, problem
	}
	if request.GetGatewayId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway ID is required")
	}
	if err := s.provider.DeleteGatewayServiceAccounts(ctx, request.GetGatewayId()); err != nil {
		return nil, providerError(err)
	}
	return &pb.DeleteGatewayResponse{}, nil
}

func (s *Server) ListManaged(ctx context.Context, request *pb.ListManagedRequest) (*pb.ListManagedResponse, error) {
	if problem := s.available(); problem != nil {
		return nil, problem
	}
	clients, err := s.provider.ListManagedClients(ctx, request.GetGatewayId())
	if err != nil {
		return nil, providerError(err)
	}
	response := &pb.ListManagedResponse{Clients: make([]*pb.ManagedClient, 0, len(clients))}
	for _, client := range clients {
		response.Clients = append(response.Clients, &pb.ManagedClient{
			Uuid: client.UUID, ClientId: client.ClientID, GatewayId: client.GatewayID,
			ServiceAccountId: client.ServiceAccountID,
		})
	}
	return response, nil
}

func (s *Server) BootstrapDefaultWorkspace(ctx context.Context, request *pb.BootstrapDefaultWorkspaceRequest) (*pb.BootstrapDefaultWorkspaceResponse, error) {
	if s == nil || s.bootstrapper == nil || !s.bootstrapper.Configured() {
		return nil, status.Error(codes.Unavailable, "default-workspace bootstrap is unavailable")
	}
	member := defaultworkspace.MemberRequest{
		AdminSpec: defaultworkspace.AdminSpec{
			GatewayID: request.GetGatewayId(), GatewayNamespace: request.GetGatewayNamespace(),
			Issuer: request.GetIssuer(), GatewayClientID: request.GetGatewayClientId(),
		},
		Endpoint: request.GetGatewayEndpoint(), Subject: request.GetSubject(),
		SubjectType: request.GetSubjectType(), Role: request.GetRole(),
	}
	if err := s.bootstrapper.AddMember(ctx, member); err != nil {
		return nil, defaultWorkspaceError(err)
	}
	return &pb.BootstrapDefaultWorkspaceResponse{}, nil
}

func (s *Server) available() error {
	if s == nil || s.provider == nil || !s.provider.Configured() {
		return status.Error(codes.Unavailable, "Keycloak service-account provisioning is unavailable")
	}
	return nil
}

func specFromProto(spec *pb.ServiceAccountSpec) serviceaccountkeycloak.ServiceAccountSpec {
	return serviceaccountkeycloak.ServiceAccountSpec{
		ClientID: spec.GetClientId(), DisplayName: spec.GetDisplayName(),
		GatewayClientID: spec.GetGatewayClientId(), GatewayID: spec.GetGatewayId(),
		ServiceAccountID: spec.GetServiceAccountId(), CreatorUserID: spec.GetCreatorUserId(),
		Role: spec.GetRole(), ExpectedIssuer: spec.GetExpectedIssuer(),
		AccessTokenLifetimeSeconds: int(spec.GetAccessTokenLifetimeSeconds()),
	}
}

func providerError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "service-account operation canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "service-account operation timed out")
	case errors.Is(err, serviceaccountkeycloak.ErrNotFound):
		return status.Error(codes.NotFound, "managed service-account client was not found")
	case errors.Is(err, serviceaccountkeycloak.ErrNotManaged):
		return status.Error(codes.PermissionDenied, "target client is not a HyperShell-managed service account")
	default:
		// Provider response bodies can contain sensitive administration details.
		// Keep the public status useful but deliberately generic.
		return status.Error(codes.Internal, "Keycloak service-account operation failed")
	}
}

func defaultWorkspaceError(err error) error {
	switch {
	case errors.Is(err, defaultworkspace.ErrInvalidRequest):
		return status.Error(codes.InvalidArgument, "default-workspace bootstrap request is invalid")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "default-workspace bootstrap canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "default-workspace bootstrap timed out")
	default:
		return status.Error(codes.Internal, "default-workspace bootstrap failed")
	}
}
