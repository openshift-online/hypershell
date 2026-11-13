package rbac

import (
	"context"
	"fmt"
	"log/slog"

	pkgrbac "github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/ciTrusts"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/plugins/repositories"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

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

// gatewayClusterResolver adapts the gateways service to the lookups that hold a
// control plane to its own cluster's gateways.
type gatewayClusterResolver struct {
	envServices *environments.Services
}

func newGatewayClusterResolver(envServices *environments.Services) pkgrbac.GatewayClusterResolver {
	if gateways.Service(envServices) == nil {
		return nil
	}
	return gatewayClusterResolver{envServices: envServices}
}

func (r gatewayClusterResolver) GatewayClusterID(ctx context.Context, gatewayID string) (string, bool, error) {
	svc := gateways.Service(r.envServices)
	if svc == nil {
		return "", false, fmt.Errorf("gateway service is not available")
	}
	gateway, svcErr := svc.GetUnscoped(ctx, gatewayID)
	if svcErr != nil {
		if svcErr.Is404() {
			return "", false, nil
		}
		return "", false, svcErr
	}
	return gateway.ClusterId, true, nil
}

func (r gatewayClusterResolver) NamespaceClusterID(ctx context.Context, namespace string) (string, bool, error) {
	svc := gateways.Service(r.envServices)
	if svc == nil {
		return "", false, fmt.Errorf("gateway service is not available")
	}
	clusterID, found, svcErr := svc.ClusterIDByNamespace(ctx, namespace)
	if svcErr != nil {
		return "", false, svcErr
	}
	return clusterID, found, nil
}

// CITrustGatewayClusterID resolves the CITrust's referenced gateway and returns
// that gateway's cluster, backing the own-cluster gate on the control plane's
// CITrust status write-back (SEC-RBAC-27). A missing trust or gateway reports
// found=false (the write is then denied, not errored).
func (r gatewayClusterResolver) CITrustGatewayClusterID(ctx context.Context, ciTrustID string) (string, bool, error) {
	ctSvc := ciTrusts.Service(r.envServices)
	if ctSvc == nil {
		return "", false, fmt.Errorf("ci trust service is not available")
	}
	trust, svcErr := ctSvc.Get(ctx, ciTrustID)
	if svcErr != nil {
		if svcErr.Is404() {
			return "", false, nil
		}
		return "", false, svcErr
	}
	gwSvc := gateways.Service(r.envServices)
	if gwSvc == nil {
		return "", false, fmt.Errorf("gateway service is not available")
	}
	gateway, svcErr := gwSvc.GetUnscoped(ctx, trust.GatewayID)
	if svcErr != nil {
		if svcErr.Is404() {
			return "", false, nil
		}
		return "", false, svcErr
	}
	return gateway.ClusterId, true, nil
}

// repositoryImportResolver adapts the repositories service to the import write
// binding's "does this reconciled Repository exist" check.
type repositoryImportResolver struct {
	envServices *environments.Services
}

func newRepositoryImportResolver(envServices *environments.Services) pkgrbac.ImportRepositoryResolver {
	if repositories.Service(envServices) == nil {
		return nil
	}
	return repositoryImportResolver{envServices: envServices}
}

func (r repositoryImportResolver) RepositoryExists(ctx context.Context, repositoryID string) (bool, error) {
	svc := repositories.Service(r.envServices)
	if svc == nil {
		return false, fmt.Errorf("repository service is not available")
	}
	_, svcErr := svc.Get(ctx, repositoryID)
	if svcErr != nil {
		if svcErr.Is404() {
			return false, nil
		}
		return false, svcErr
	}
	return true, nil
}

// dependencies resolves the authorizer's services from the environment. A
// service whose plugin is not registered stays nil (see rbac.Dependencies).
func dependencies(env *environments.Env) pkgrbac.Dependencies {
	envServices := &env.Services
	deps := pkgrbac.Dependencies{
		ActivityRecorder: users.ActivityRecorder(envServices),
		Clusters:         managedClusterResolver{envServices: envServices},
		ControlPlanes:    newControlPlaneResolver(envServices),
		Gateways:         newGatewayClusterResolver(envServices),
		Imports:          newRepositoryImportResolver(envServices),
	}
	if rbService := roleBindings.Service(envServices); rbService != nil {
		deps.Lookup = rbService
		deps.Syncer = rbService
	}
	if userService := users.Service(envServices); userService != nil {
		deps.Provisioner = pkgrbac.NewUserProvisioner(userService)
	}
	if deps.ControlPlanes == nil {
		slog.Warn("managedClusters service not registered: registered-cluster control-plane identity is disabled; only RBAC_SERVICE_ACCOUNTS is exempt from gRPC role bindings")
	}
	return deps
}
