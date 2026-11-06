package ciTrusts

import (
	"net/http"

	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/extsearch"
	"github.com/openshift-online/hypershell/components/api-server/pkg/routes"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/authz"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
)

type ServiceLocator func() CITrustService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewCITrustDao(&env.Database.SessionFactory)
	return func() CITrustService {
		return NewCITrustService(dao, events.Service(&env.Services))
	}
}

func Service(s *environments.Services) CITrustService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("CITrusts"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("CITrusts", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("ci_trusts", "ext", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		h := NewCITrustHandler(Service(envServices), extsearch.Service(envServices), newTenancyAuthorizer(envServices))

		router := routes.Resource(apiRouter, "ci_trusts").PathPrefix("/ci_trusts").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
	})

	pkgserver.RegisterGRPCService("ci_trusts", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterCITrustServiceServer(grpcServer, NewCITrustGRPCHandler(Service(envServices), extsearch.Service(envServices), brokerFunc))
	}, authz.CRUDResources(pb.CITrustService_ServiceDesc.ServiceName, "ci_trusts", "CITrust", "CITrusts"))

	presenters.RegisterPath(CITrust{}, "ci_trusts")
	presenters.RegisterPath(&CITrust{}, "ci_trusts")
	presenters.RegisterKind(CITrust{}, "CITrust")
	presenters.RegisterKind(&CITrust{}, "CITrust")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationConverge())
	db.RegisterMigration(migrationAddName())
	db.RegisterMigration(migrationAddRepositoryOwnership())
}
