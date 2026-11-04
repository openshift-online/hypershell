package agentRuntimes

import (
	"net/http"

	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/extsearch"
	"github.com/openshift-online/hypershell/components/api-server/pkg/routes"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/authz"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
)

type ServiceLocator func() AgentRuntimeService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() AgentRuntimeService {
		return NewAgentRuntimeService(
			NewAgentRuntimeDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
		)
	}
}

func Service(s *environments.Services) AgentRuntimeService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("AgentRuntimes"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

// gatewayClusterLookup resolves gateway -> cluster assignments through the
// gateways plugin's service locator, looked up per call so it follows the
// environment's service registry. It returns nil when the gateways plugin is
// absent, which the gRPC handler treats as fail-closed for cluster-scoped
// requests. It reuses the same seam the roleBindings plugin exposes.
func gatewayClusterLookup(s *environments.Services) GatewayClusterLookup {
	if s == nil || s.GetService("Gateways") == nil {
		return nil
	}
	source, ok := s.GetService("Gateways").(roleBindings.GatewayClusterLookupSource)
	if !ok {
		return nil
	}
	return source.GatewayClusterLookup()
}

// secretSourceLookup resolves the SecretSources owned by an AgentRuntime through
// the secretSources plugin's service locator, looked up per call so it follows
// the environment's service registry. It returns nil when the secretSources
// plugin is absent, which the gRPC handler treats as "no sources" rather than
// breaking the watch. It is resolved through a local interface so this package
// does not import secretSources (which imports this one to implement the seam).
func secretSourceLookup(s *environments.Services) SecretSourceLookup {
	if s == nil || s.GetService("SecretSources") == nil {
		return nil
	}
	source, ok := s.GetService("SecretSources").(SecretSourceLookupSource)
	if !ok {
		return nil
	}
	return source.SecretSourceLookup()
}

// workspaceLookup resolves the gateway-side workspace owned by an AgentRuntime
// (name, provider bindings, memberships, inference route) through the agentWorkspaces
// plugin's service locator, looked up per call so it follows the environment's
// service registry. It returns nil when the agentWorkspaces plugin is absent, which
// the gRPC handler treats as "no workspace" rather than breaking the watch. It is
// resolved through a local interface so this package does not import agentWorkspaces
// (which imports this one to implement the seam).
func workspaceLookup(s *environments.Services) WorkspaceLookup {
	if s == nil || s.GetService("AgentWorkspaces") == nil {
		return nil
	}
	source, ok := s.GetService("AgentWorkspaces").(WorkspaceLookupSource)
	if !ok {
		return nil
	}
	return source.WorkspaceLookup()
}

func init() {
	registry.RegisterService("AgentRuntimes", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("agentRuntimes", "ext", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		h := NewAgentRuntimeHandler(Service(envServices), extsearch.Service(envServices), newManageAuthorizer(envServices))

		router := routes.Resource(apiRouter, "agent_runtimes").PathPrefix("/agent_runtimes").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
	})

	pkgserver.RegisterGRPCService("agentRuntimes", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		svc := Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterAgentRuntimeServiceServer(grpcServer, NewAgentRuntimeGRPCHandler(svc, gatewayClusterLookup(envServices), secretSourceLookup(envServices), workspaceLookup(envServices), brokerFunc))
	}, authz.GRPCResources{
		"/" + pb.AgentRuntimeService_ServiceDesc.ServiceName + "/ListAgentRuntimes":        {Kind: "agent_runtimes", Action: authz.ActionList},
		"/" + pb.AgentRuntimeService_ServiceDesc.ServiceName + "/WatchAgentRuntimes":       {Kind: "agent_runtimes", Action: authz.ActionWatch},
		"/" + pb.AgentRuntimeService_ServiceDesc.ServiceName + "/CreateAgentRuntime":       {Kind: "agent_runtimes", Action: authz.ActionCreate},
		"/" + pb.AgentRuntimeService_ServiceDesc.ServiceName + "/UpdateAgentRuntime":       {Kind: "agent_runtimes", Action: authz.ActionUpdate},
		"/" + pb.AgentRuntimeService_ServiceDesc.ServiceName + "/DeleteAgentRuntime":       {Kind: "agent_runtimes", Action: authz.ActionDelete},
		"/" + pb.AgentRuntimeService_ServiceDesc.ServiceName + "/UpdateAgentRuntimeStatus": {Kind: "agent_runtimes", Action: authz.ActionUpdate},
	})

	presenters.RegisterPath(AgentRuntime{}, "agent_runtimes")
	presenters.RegisterPath(&AgentRuntime{}, "agent_runtimes")
	presenters.RegisterKind(AgentRuntime{}, "AgentRuntime")
	presenters.RegisterKind(&AgentRuntime{}, "AgentRuntime")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddRepositorySelector())
	db.RegisterMigration(migrationAddStatusMessage())
}
