// Package platformGatewayPolicies is the PlatformGatewayPolicy extension kind
// (PLAT-DATA-28), the PlatformPolicySettings global toggle (PLAT-DATA-29), the
// policy audit findings store (PLAT-DATA-30), and the gateway effective-policies
// read surface (PLAT-PGP-19). Behavior is specified in
// specs/platform/platform-gateway-policy.spec.md.
package platformGatewayPolicies

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/extsearch"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/pkg/routes"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/authz"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
)

const serviceName = "PlatformGatewayPolicies"

// EnvPruneInterval overrides how often expired findings are pruned.
const EnvPruneInterval = "HYPERSHELL_POLICY_FINDINGS_PRUNE_INTERVAL"

type ServiceLocator func() PolicyService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	var once sync.Once
	var svc PolicyService
	return func() PolicyService {
		once.Do(func() {
			cfg, cfgErr := LoadConfig()
			if cfgErr != nil {
				slog.Error("PlatformGatewayPolicy configuration is invalid; policy writes and reads fail until it is fixed", "error", cfgErr)
			}
			svc = NewService(NewDao(&env.Database.SessionFactory), events.Service(&env.Services), cfg, cfgErr)
			go runPruner(svc, pruneInterval())
			if cfgErr == nil {
				go reconcileDefaultsAtStartup(svc, cfg.Defaults)
			}
		})
		return svc
	}
}

func Service(s *environments.Services) PolicyService {
	if s == nil {
		return nil
	}
	if obj := s.GetService(serviceName); obj != nil {
		return obj.(ServiceLocator)()
	}
	return nil
}

func pruneInterval() time.Duration {
	if v := os.Getenv(EnvPruneInterval); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= time.Minute {
			return d
		}
		slog.Warn("ignoring invalid findings prune interval; using 1h", "value", v)
	}
	return time.Hour
}

// runPruner deletes findings past the retention on a fixed interval
// (PLAT-DATA-30.02). A failed prune is logged and retried on the next tick.
func runPruner(svc PolicyService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		if n, svcErr := svc.PruneFindings(ctx, time.Now()); svcErr != nil {
			slog.WarnContext(ctx, "policy audit findings prune failed", "error", svcErr)
		} else if n > 0 {
			slog.InfoContext(ctx, "pruned expired policy audit findings", "rows", n)
		}
		cancel()
		<-ticker.C
	}
}

// reconcileDefaultsAtStartup accepts or refuses the configured fleet defaults
// once per api-server start (PLAT-PGP-08.02), retrying until the database
// answers. Until it succeeds, reads serve the stored accepted defaults.
func reconcileDefaultsAtStartup(svc PolicyService, configured FleetDefaults) {
	backoff := time.Second
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		_, svcErr := svc.ReconcileDefaults(ctx, configured)
		cancel()
		if svcErr == nil {
			return
		}
		slog.Warn("fleet defaults reconcile failed; retrying", "error", svcErr, "backoff", backoff)
		time.Sleep(backoff)
		backoff = min(backoff*2, time.Minute)
	}
}

func gatewayLookup(envServices *environments.Services) GatewayLookup {
	return func(ctx context.Context, gatewayID string) (bool, *errors.ServiceError) {
		return Service(envServices).GatewayExists(ctx, gatewayID)
	}
}

func newPluginHandler(envServices *environments.Services) *handler {
	var lookup rbac.RoleBindingLookup
	if rb := roleBindings.Service(envServices); rb != nil {
		lookup = rb
	}
	return newHandler(Service(envServices), extsearch.Service(envServices), gatewayLookup(envServices),
		targetVisibility{enforced: rbac.Enforced(), bindings: lookup})
}

func brokerFor(envServices *environments.Services) func() *pkgserver.EventBroker {
	return func() *pkgserver.EventBroker {
		if obj := envServices.GetService("EventBroker"); obj != nil {
			return obj.(*pkgserver.EventBroker)
		}
		return nil
	}
}

func init() {
	registry.RegisterService(serviceName, func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("platform_gateway_policies", "ext", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
		h := newPluginHandler(services.(*environments.Services))

		router := routes.Resource(apiRouter, "platform_gateway_policies").PathPrefix("/platform_gateway_policies").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/evaluate", h.Evaluate).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.HandleFunc("/{id}/findings", h.Findings).Methods(http.MethodGet)

		settings := routes.Resource(apiRouter, "platform_policy_settings").PathPrefix("/platform_policy_settings").Subrouter()
		settings.HandleFunc("", h.GetSettings).Methods(http.MethodGet)
		settings.HandleFunc("", h.PatchSettings).Methods(http.MethodPatch)
	})

	pkgserver.RegisterRoutes("platform_gateway_policies_effective", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
		h := newPluginHandler(services.(*environments.Services))
		router := routes.Resource(apiRouter, "effective_policies").PathPrefix("/gateways/{gateway_id}/effective_policies").Subrouter()
		router.HandleFunc("", h.EffectivePolicies).Methods(http.MethodGet)
	})

	pkgserver.RegisterGRPCService("platform_gateway_policies", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		svc := Service(envServices)
		broker := brokerFor(envServices)
		pb.RegisterPlatformGatewayPolicyServiceServer(grpcServer, &policyGRPCHandler{service: svc, brokerFunc: broker})
		pb.RegisterPlatformPolicySettingsServiceServer(grpcServer, &settingsGRPCHandler{service: svc, brokerFunc: broker})
		pb.RegisterPolicyAuditFindingServiceServer(grpcServer, &findingGRPCHandler{service: svc})
	}, grpcResources())

	presenters.RegisterPath(PlatformGatewayPolicy{}, "platform_gateway_policies")
	presenters.RegisterPath(&PlatformGatewayPolicy{}, "platform_gateway_policies")
	presenters.RegisterKind(PlatformGatewayPolicy{}, "PlatformGatewayPolicy")
	presenters.RegisterKind(&PlatformGatewayPolicy{}, "PlatformGatewayPolicy")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationSeedCanary())
	db.RegisterMigration(migrationSettingsDefaults())
}

func grpcResources() authz.GRPCResources {
	policy := "/" + pb.PlatformGatewayPolicyService_ServiceDesc.ServiceName + "/"
	settings := "/" + pb.PlatformPolicySettingsService_ServiceDesc.ServiceName + "/"
	findings := "/" + pb.PolicyAuditFindingService_ServiceDesc.ServiceName + "/"
	return authz.GRPCResources{
		policy + "GetPlatformGatewayPolicy":          {Kind: "platform_gateway_policies", Action: authz.ActionGet},
		policy + "ListPlatformGatewayPolicies":       {Kind: "platform_gateway_policies", Action: authz.ActionList},
		policy + "WatchPlatformGatewayPolicies":      {Kind: "platform_gateway_policies", Action: authz.ActionWatch},
		policy + "UpdatePlatformGatewayPolicyStatus": {Kind: "platform_gateway_policies", Action: authz.ActionUpdate},
		settings + "GetPlatformPolicySettings":       {Kind: "platform_policy_settings", Action: authz.ActionGet},
		settings + "WatchPlatformPolicySettings":     {Kind: "platform_policy_settings", Action: authz.ActionWatch},
		findings + "ReportPolicyAuditFindings":       {Kind: "policy_audit_findings", Action: authz.ActionCreate},
	}
}
