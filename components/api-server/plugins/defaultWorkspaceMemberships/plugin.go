package defaultWorkspaceMemberships

import (
	"context"

	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/authz"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
)

// DefaultWorkspaceMembership is internal-only (PLAT-DATA-27, "No public CRUD"):
// this plugin registers NO REST route, NO OpenAPI schema, NO hsctl command, and
// NO presenters.RegisterPath/RegisterKind entry. It registers only a gRPC
// service exposing List/Watch (read, for the control plane's reconciler,
// CTRL-CORE-20) and the reconciliation-status write-back. Create and Delete are
// plain Go calls the gatewayAccess and serviceAccounts plugins make directly
// through ServiceLocator (same process), never over gRPC or HTTP.

type ServiceLocator func() MembershipService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewDao(&env.Database.SessionFactory)
	return func() MembershipService {
		return NewService(dao, events.Service(&env.Services))
	}
}

func Service(s *environments.Services) MembershipService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("DefaultWorkspaceMemberships"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

// gatewayClusterLookupFromGateways resolves gateway -> cluster assignments
// through the gateways plugin's service locator, mirroring
// roleBindings/plugin.go's gatewayClusterLookup helper of the same shape. It
// reuses roleBindings.GatewayClusterLookupSource, the interface the gateways
// plugin's locator already implements, rather than declaring a second
// identical interface.
func gatewayClusterLookupFromGateways(s *environments.Services) roleBindings.GatewayClusterLookup {
	if s == nil || s.GetService("Gateways") == nil {
		return nil
	}
	return func(ctx context.Context, gatewayID string) (string, bool, *errors.ServiceError) {
		source, ok := s.GetService("Gateways").(roleBindings.GatewayClusterLookupSource)
		if !ok {
			return "", false, errors.GeneralError("gateway service does not provide a cluster lookup")
		}
		return source.GatewayClusterLookup()(ctx, gatewayID)
	}
}

func init() {
	registry.RegisterService("DefaultWorkspaceMemberships", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})
	RegisterMetrics()

	pkgserver.RegisterGRPCService("defaultWorkspaceMemberships", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterDefaultWorkspaceMembershipServiceServer(grpcServer, NewGRPCHandler(Service(envServices), gatewayClusterLookupFromGateways(envServices), brokerFunc))
	}, authz.GRPCResources{
		"/" + pb.DefaultWorkspaceMembershipService_ServiceDesc.ServiceName + "/ListDefaultWorkspaceMemberships":        {Kind: "default_workspace_memberships", Action: authz.ActionList},
		"/" + pb.DefaultWorkspaceMembershipService_ServiceDesc.ServiceName + "/WatchDefaultWorkspaceMemberships":       {Kind: "default_workspace_memberships", Action: authz.ActionWatch},
		"/" + pb.DefaultWorkspaceMembershipService_ServiceDesc.ServiceName + "/UpdateDefaultWorkspaceMembershipStatus": {Kind: "default_workspace_memberships", Action: authz.ActionUpdate},
	})

	db.RegisterMigration(migration())
}
