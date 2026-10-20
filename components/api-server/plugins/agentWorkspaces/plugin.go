package agentWorkspaces

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/generic"
)

type ServiceLocator func() AgentWorkspaceService

// WorkspaceLookup satisfies agentRuntimes.WorkspaceLookupSource so the agentRuntimes
// gRPC handler can embed a runtime's workspace (name, provider bindings, memberships,
// inference route) in its payload without importing this plugin. The dependency
// points agentWorkspaces -> agentRuntimes only, so no import cycle forms.
var _ agentRuntimes.WorkspaceLookupSource = ServiceLocator(nil)

func (l ServiceLocator) WorkspaceLookup() agentRuntimes.WorkspaceLookup {
	return func(ctx context.Context, agentRuntimeID string) (*agentRuntimes.WorkspaceView, *errors.ServiceError) {
		data, svcErr := l().ResolveView(ctx, agentRuntimeID)
		if svcErr != nil {
			return nil, svcErr
		}
		if data == nil {
			return nil, nil
		}
		view := &agentRuntimes.WorkspaceView{
			ID:        data.ID,
			Name:      data.Name,
			GatewayID: data.GatewayId,
		}
		for _, b := range data.ProviderBindings {
			view.ProviderBindings = append(view.ProviderBindings, agentRuntimes.ProviderBindingView{
				ID:              b.ID,
				Name:            b.Name,
				WorkspaceID:     b.WorkspaceId,
				ProviderSpecID:  b.ProviderSpecId,
				SecretSourceID:  b.SecretSourceId,
				RefreshStrategy: b.RefreshStrategy,
				Status:          b.Status,
			})
		}
		for _, m := range data.Memberships {
			view.Memberships = append(view.Memberships, agentRuntimes.WorkspaceMembershipView{
				ID:          m.ID,
				WorkspaceID: m.WorkspaceId,
				Subject:     m.Subject,
				Role:        m.Role,
			})
		}
		if data.Route != nil {
			view.Route = &agentRuntimes.InferenceRouteView{
				ID:                data.Route.ID,
				WorkspaceID:       data.Route.WorkspaceId,
				ProviderBindingID: data.Route.ProviderBindingId,
				Model:             data.Route.Model,
			}
		}
		return view, nil
	}
}

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewAgentWorkspaceDao(&env.Database.SessionFactory)
	return func() AgentWorkspaceService {
		return NewAgentWorkspaceService(dao, events.Service(&env.Services))
	}
}

func Service(s *environments.Services) AgentWorkspaceService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("AgentWorkspaces"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("AgentWorkspaces", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("agentWorkspaces", "ext", func(extRouter *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		h := NewAgentWorkspaceHandler(Service(envServices), generic.Service(envServices))

		router := extRouter.PathPrefix("/agent_workspaces").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterGRPCService("agentWorkspaces", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterAgentWorkspaceServiceServer(grpcServer, NewAgentWorkspaceGRPCHandler(Service(envServices), generic.Service(envServices), brokerFunc))
	})

	presenters.RegisterPath(AgentWorkspace{}, "agent_workspaces")
	presenters.RegisterPath(&AgentWorkspace{}, "agent_workspaces")
	presenters.RegisterKind(AgentWorkspace{}, "AgentWorkspace")
	presenters.RegisterKind(&AgentWorkspace{}, "AgentWorkspace")

	db.RegisterMigration(migration())
}
