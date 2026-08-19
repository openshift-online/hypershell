package rbac

import (
	"context"
	"os"
	"strings"
	"sync"

	"google.golang.org/grpc"

	pkgrbac "github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
)

type lazyRBACInterceptor struct {
	once        sync.Once
	lookup      pkgrbac.RoleBindingLookup
	provisioner pkgrbac.UserProvisioner
	syncer      pkgrbac.JWTRoleSyncer
	config      pkgrbac.AuthzConfig
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

		var serviceAccounts []string
		if sa := os.Getenv("RBAC_SERVICE_ACCOUNTS"); sa != "" {
			for _, s := range strings.Split(sa, ",") {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					serviceAccounts = append(serviceAccounts, trimmed)
				}
			}
		}

		l.config = pkgrbac.AuthzConfig{
			EnforceRBAC:     os.Getenv("RBAC_ENFORCE") == "true",
			ServiceAccounts: serviceAccounts,
		}
	})
}

func init() {
	lazy := &lazyRBACInterceptor{}

	pkgserver.RegisterPostAuthGRPCUnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		lazy.init(ctx)
		if lazy.lookup == nil {
			return handler(ctx, req)
		}
		interceptor := pkgrbac.RBACUnaryInterceptor(lazy.lookup, lazy.provisioner, lazy.syncer, lazy.config)
		return interceptor(ctx, req, info, handler)
	})

	pkgserver.RegisterPostAuthGRPCStreamInterceptor(func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		lazy.init(ss.Context())
		if lazy.lookup == nil {
			return handler(srv, ss)
		}
		interceptor := pkgrbac.RBACStreamInterceptor(lazy.lookup, lazy.provisioner, lazy.syncer, lazy.config)
		return interceptor(srv, ss, info, handler)
	})
}
