package secretSources

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/secretstore"
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

type ServiceLocator func() SecretSourceService

// SecretSourceLookup satisfies agentRuntimes.SecretSourceLookupSource so the
// agentRuntimes gRPC handler can embed a runtime's SecretSources in its payload
// without importing this plugin. The dependency points secretSources ->
// agentRuntimes only, so no import cycle forms.
var _ agentRuntimes.SecretSourceLookupSource = ServiceLocator(nil)

func (l ServiceLocator) SecretSourceLookup() agentRuntimes.SecretSourceLookup {
	return func(ctx context.Context, agentRuntimeID string) ([]agentRuntimes.SecretSourceView, *errors.ServiceError) {
		sources, svcErr := l().FindByAgentRuntimeID(ctx, agentRuntimeID)
		if svcErr != nil {
			return nil, svcErr
		}
		views := make([]agentRuntimes.SecretSourceView, 0, len(sources))
		for _, s := range sources {
			if s == nil {
				continue
			}
			views = append(views, agentRuntimes.SecretSourceView{
				Name:        s.Name,
				Purpose:     s.Purpose,
				Backend:     s.Backend,
				Path:        s.Path,
				KeyMappings: s.KeyMappings,
			})
		}
		return views, nil
	}
}

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewSecretSourceDao(&env.Database.SessionFactory)
	return func() SecretSourceService {
		return NewSecretSourceService(dao, events.Service(&env.Services))
	}
}

func Service(s *environments.Services) SecretSourceService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("SecretSources"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("SecretSources", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("secretSources", "ext", func(extRouter *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		// The secret-submission endpoint writes a submitted value to the configured
		// secret manager before persisting the reference. A misconfigured store does
		// not block the server from starting or reference-only creates from working;
		// it fails any value submission loudly (errorStore).
		store, storeErr := secretstore.NewFromEnv()
		if storeErr != nil {
			slog.Warn("secret store configuration error; secret submissions will be rejected", "error", storeErr)
			store = secretstore.NewErrorStore(storeErr)
		}
		h := NewSecretSourceHandler(Service(envServices), generic.Service(envServices), store)

		router := extRouter.PathPrefix("/secret_sources").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterGRPCService("secretSources", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterSecretSourceServiceServer(grpcServer, NewSecretSourceGRPCHandler(Service(envServices), generic.Service(envServices), brokerFunc))
	})

	presenters.RegisterPath(SecretSource{}, "secret_sources")
	presenters.RegisterPath(&SecretSource{}, "secret_sources")
	presenters.RegisterKind(SecretSource{}, "SecretSource")
	presenters.RegisterKind(&SecretSource{}, "SecretSource")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddRepositoryOwnership())
}
