package agentRuntimes

import (
	"net/http"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/generic"
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

func init() {
	registry.RegisterService("AgentRuntimes", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("agentRuntimes", "ext", func(extRouter *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		h := NewAgentRuntimeHandler(Service(envServices), generic.Service(envServices))

		router := extRouter.PathPrefix("/agent_runtimes").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
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
		pb.RegisterAgentRuntimeServiceServer(grpcServer, NewAgentRuntimeGRPCHandler(svc, gatewayClusterLookup(envServices), brokerFunc))
	})

	presenters.RegisterPath(AgentRuntime{}, "agent_runtimes")
	presenters.RegisterPath(&AgentRuntime{}, "agent_runtimes")
	presenters.RegisterKind(AgentRuntime{}, "AgentRuntime")
	presenters.RegisterKind(&AgentRuntime{}, "AgentRuntime")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddRepositorySelector())
}
