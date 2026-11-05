package inferenceRoutes

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

type ServiceLocator func() InferenceRouteService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewInferenceRouteDao(&env.Database.SessionFactory)
	return func() InferenceRouteService {
		return NewInferenceRouteService(dao, events.Service(&env.Services))
	}
}

func Service(s *environments.Services) InferenceRouteService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("InferenceRoutes"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("InferenceRoutes", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("inferenceRoutes", "ext", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		h := NewInferenceRouteHandler(Service(envServices), extsearch.Service(envServices))

		router := routes.Resource(apiRouter, "inference_routes").PathPrefix("/inference_routes").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
	})

	pkgserver.RegisterGRPCService("inferenceRoutes", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterInferenceRouteServiceServer(grpcServer, NewInferenceRouteGRPCHandler(Service(envServices), extsearch.Service(envServices), brokerFunc))
	}, authz.GRPCResources{
		"/" + pb.InferenceRouteService_ServiceDesc.ServiceName + "/GetInferenceRoute":    {Kind: "inference_routes", Action: authz.ActionGet},
		"/" + pb.InferenceRouteService_ServiceDesc.ServiceName + "/CreateInferenceRoute": {Kind: "inference_routes", Action: authz.ActionCreate},
		"/" + pb.InferenceRouteService_ServiceDesc.ServiceName + "/DeleteInferenceRoute": {Kind: "inference_routes", Action: authz.ActionDelete},
		"/" + pb.InferenceRouteService_ServiceDesc.ServiceName + "/ListInferenceRoutes":  {Kind: "inference_routes", Action: authz.ActionList},
		"/" + pb.InferenceRouteService_ServiceDesc.ServiceName + "/WatchInferenceRoutes": {Kind: "inference_routes", Action: authz.ActionWatch},
	})

	presenters.RegisterPath(InferenceRoute{}, "inference_routes")
	presenters.RegisterPath(&InferenceRoute{}, "inference_routes")
	presenters.RegisterKind(InferenceRoute{}, "InferenceRoute")
	presenters.RegisterKind(&InferenceRoute{}, "InferenceRoute")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddRepositoryOwnership())
	db.RegisterMigration(migrationAddName())
}
