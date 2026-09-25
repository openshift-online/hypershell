package rbac

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/golang/glog"
	"google.golang.org/grpc"

	pkgrbac "github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
)

type lazyRBACInterceptor struct {
	once             sync.Once
	lookup           pkgrbac.RoleBindingLookup
	provisioner      pkgrbac.UserProvisioner
	syncer           pkgrbac.JWTRoleSyncer
	activityRecorder pkgrbac.DailyActivityRecorder
	config           pkgrbac.AuthzConfig
	// clusters backs the watch-stream caller binding, which fails closed when
	// the managed cluster registry is missing.
	clusters pkgrbac.RegisteredClusterResolver
	// controlPlanes backs the control-plane identity exemption. It is nil when
	// the managedClusters plugin is not registered, so that exemption is simply
	// off (RBAC_SERVICE_ACCOUNTS alone applies) instead of every call failing.
	controlPlanes pkgrbac.RegisteredClusterResolver
}

// managedClusterResolver adapts the managedClusters service to the caller
// binding's subject -> registered cluster id lookup.
type managedClusterResolver struct {
	envServices *environments.Services
}

// newControlPlaneResolver returns the resolver for the control-plane identity
// exemption, or nil when the managedClusters plugin is not registered.
func newControlPlaneResolver(envServices *environments.Services) pkgrbac.RegisteredClusterResolver {
	if managedClusters.Service(envServices) == nil {
		return nil
	}
	return managedClusterResolver{envServices: envServices}
}

func (r managedClusterResolver) RegisteredClusterIDForSubject(ctx context.Context, subject string) (string, bool, error) {
	svc := managedClusters.Service(r.envServices)
	if svc == nil {
		return "", false, fmt.Errorf("managed cluster service is not available")
	}
	cluster, svcErr := svc.FindRegisteredBySubject(ctx, subject)
	if svcErr != nil {
		return "", false, svcErr
	}
	if cluster == nil {
		return "", false, nil
	}
	return cluster.ID, true, nil
}

func (l *lazyRBACInterceptor) init(ctx context.Context) {
	l.once.Do(func() {
		env := environments.Environment()
		if env == nil {
			return
		}
		envServices := &env.Services

		rbService := roleBindings.Service(envServices)
		if rbService != nil {
			l.lookup = rbService
			l.syncer = rbService
		}

		userService := users.Service(envServices)
		if userService != nil {
			l.provisioner = pkgrbac.NewUserProvisioner(userService)
		}
		l.activityRecorder = users.ActivityRecorder(envServices)
		l.clusters = managedClusterResolver{envServices: envServices}
		l.controlPlanes = newControlPlaneResolver(envServices)
		if l.controlPlanes == nil {
			glog.Warning("managedClusters service not registered: registered-cluster control-plane identity is disabled; only RBAC_SERVICE_ACCOUNTS is exempt from gRPC role bindings")
		}

		l.config = pkgrbac.AuthzConfig{
			EnforceRBAC:     os.Getenv("RBAC_ENFORCE") == "true",
			ServiceAccounts: pkgrbac.ServiceAccountsFromEnv(),
		}
	})
}

func init() {
	lazy := &lazyRBACInterceptor{}

	// The managed-cluster caller binding runs first, before role-binding
	// authorization and regardless of the RBAC_SERVICE_ACCOUNTS allowlist and
	// RBAC_ENFORCE: a registered control plane may list or watch only its own
	// cluster's gateways (managed-cluster-registration.spec.md, "Watch Stream
	// Caller Binding"). Role-binding authorization then treats a registered
	// cluster as a control-plane identity, like an RBAC_SERVICE_ACCOUNTS entry
	// ("Control-Plane Identity"), so a new spoke needs no hub-side config.
	pkgserver.RegisterPostAuthGRPCUnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		lazy.init(ctx)
		if err := pkgrbac.CheckClusterCallerBindingUnary(ctx, lazy.clusters, info.FullMethod, req); err != nil {
			return nil, err
		}
		if lazy.lookup == nil {
			return handler(ctx, req)
		}
		interceptor := pkgrbac.RBACUnaryInterceptor(lazy.lookup, lazy.provisioner, lazy.syncer, lazy.activityRecorder, lazy.controlPlanes, lazy.config)
		return interceptor(ctx, req, info, handler)
	})

	pkgserver.RegisterPostAuthGRPCStreamInterceptor(func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		lazy.init(ss.Context())
		ss, err := pkgrbac.BindClusterCallerStream(ss, info.FullMethod, lazy.clusters)
		if err != nil {
			return err
		}
		if lazy.lookup == nil {
			return handler(srv, ss)
		}
		interceptor := pkgrbac.RBACStreamInterceptor(lazy.lookup, lazy.provisioner, lazy.syncer, lazy.activityRecorder, lazy.controlPlanes, lazy.config)
		return interceptor(srv, ss, info, handler)
	})
}
