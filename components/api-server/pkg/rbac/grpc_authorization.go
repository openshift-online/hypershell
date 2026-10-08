package rbac

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// gRPC authorization (rbac-enforcement.spec.md, "gRPC Authorization"): every
// hypershell.v1 method is decided per caller kind, default-deny.
//
//   - A registered control plane (its JWT sub is a ManagedCluster's
//     oidc_subject) may read fleet-wide records it reconciles from, write only
//     gateways assigned to its own cluster, and delete only its own ManagedCluster record.
//   - An RBAC_SERVICE_ACCOUNTS account that has not registered keeps the
//     bootstrap exemption, except that it may not create, change, or delete
//     ManagedCluster records.
//   - Every other caller gets the HTTP rules: per-gateway bindings for one
//     gateway, platform:admin for the unfiltered gateway list and watch, and no
//     access to the control-plane-only methods or the RoleBinding service.
//
// The hub exposes gRPC through a public passthrough Route (hub-grpc-tls.spec.md),
// so these rules, not network placement, are what keep a tenant, or a
// compromised spoke, inside its own gateways and cluster.

// GatewayClusterResolver resolves the managed cluster a gateway is assigned to.
type GatewayClusterResolver interface {
	// GatewayClusterID returns the cluster of the gateway with this id, live or
	// soft-deleted.
	GatewayClusterID(ctx context.Context, gatewayID string) (clusterID string, found bool, err error)
	// NamespaceClusterID returns the cluster of the live gateway in namespace.
	NamespaceClusterID(ctx context.Context, namespace string) (clusterID string, found bool, err error)
}

const hypershellGRPCPrefix = "/hypershell.v1."

// splitHypershellMethod splits "/hypershell.v1.GatewayService/GetGateway" into
// ("GatewayService", "GetGateway"). ok is false for any other package (health,
// reflection).
func splitHypershellMethod(fullMethod string) (service, method string, ok bool) {
	if !strings.HasPrefix(fullMethod, hypershellGRPCPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(fullMethod, hypershellGRPCPrefix)
	service, method, ok = strings.Cut(rest, "/")
	return service, method, ok && service != "" && method != ""
}

func isReadMethodName(method string) bool {
	return strings.HasPrefix(method, "Get") || strings.HasPrefix(method, "List") || strings.HasPrefix(method, "Watch")
}

func denied() error { return status.Error(codes.PermissionDenied, "forbidden") }

// notFound hides whether a resource the caller may not read exists, like the
// HTTP API's 404 for a GET by id.
func notFound() error { return status.Error(codes.NotFound, "not found") }

func unexpectedRequest() error {
	return status.Error(codes.Internal, "unexpected request type for authorization")
}

// isManagedClusterRecordMutation reports the ManagedCluster record writes the
// bootstrap allowlist no longer covers.
func isManagedClusterRecordMutation(fullMethod string) bool {
	service, method, ok := splitHypershellMethod(fullMethod)
	return ok && service == "ManagedClusterService" && !isReadMethodName(method)
}

// authorizeControlPlaneGRPC decides a call from the control plane registered as
// clusterID. req is nil on a streaming call, where only Watch methods exist.
func authorizeControlPlaneGRPC(ctx context.Context, clusterID, fullMethod string, req interface{}, gateways GatewayClusterResolver) error {
	service, method, ok := splitHypershellMethod(fullMethod)
	if !ok {
		return nil
	}
	switch service {
	case "GatewayService":
		switch method {
		case "ListGateways", "WatchGateways":
			// The watch-stream caller binding already requires cluster_id to be
			// this control plane's own (cluster_binding.go).
			return nil
		case "GetGateway":
			r, ok := req.(*pb.GetGatewayRequest)
			if !ok {
				return unexpectedRequest()
			}
			return requireGatewayOnCluster(ctx, gateways, r.GetId(), clusterID, notFound)
		case "UpdateGateway":
			r, ok := req.(*pb.UpdateGatewayRequest)
			if !ok {
				return unexpectedRequest()
			}
			if r.ClusterId != nil && r.GetClusterId() != clusterID {
				return denied()
			}
			return requireGatewayOnCluster(ctx, gateways, r.GetId(), clusterID, denied)
		case "SetGatewayVersion":
			r, ok := req.(*pb.SetGatewayVersionRequest)
			if !ok {
				return unexpectedRequest()
			}
			return requireGatewayOnCluster(ctx, gateways, r.GetId(), clusterID, denied)
		case "AdjustActiveSandboxCount":
			r, ok := req.(*pb.AdjustActiveSandboxCountRequest)
			if !ok {
				return unexpectedRequest()
			}
			return requireNamespaceOnCluster(ctx, gateways, r.GetNamespace(), clusterID)
		case "SetActiveSandboxCount":
			r, ok := req.(*pb.SetActiveSandboxCountRequest)
			if !ok {
				return unexpectedRequest()
			}
			return requireNamespaceOnCluster(ctx, gateways, r.GetNamespace(), clusterID)
		}
		return denied()
	case "ManagedClusterService":
		if isReadMethodName(method) {
			return nil
		}
		if method == "DeleteManagedCluster" {
			r, ok := req.(*pb.DeleteManagedClusterRequest)
			if !ok {
				return unexpectedRequest()
			}
			if r.GetId() == clusterID {
				return nil
			}
		}
		return denied()
	case "RoleBindingService":
		if method == "ListRoleBindings" || method == "WatchRoleBindings" {
			// Scoped to the caller's cluster by the caller binding.
			return nil
		}
		return denied()
	}
	return denied()
}

func requireGatewayOnCluster(ctx context.Context, gateways GatewayClusterResolver, gatewayID, clusterID string, deny func() error) error {
	if gateways == nil {
		return status.Error(codes.Unavailable, "gateway registry unavailable; cannot verify the gateway's cluster")
	}
	if gatewayID == "" {
		return status.Error(codes.InvalidArgument, "id is required")
	}
	owner, found, err := gateways.GatewayClusterID(ctx, gatewayID)
	if err != nil {
		return status.Error(codes.Unavailable, "gateway lookup failed; cannot verify the gateway's cluster")
	}
	if !found || owner != clusterID {
		return deny()
	}
	return nil
}

// requireNamespaceOnCluster lets a sandbox-count write through when the live
// gateway backing namespace is on the caller's cluster. With no live gateway
// the write is a no-op in the service, so it is let through as well.
func requireNamespaceOnCluster(ctx context.Context, gateways GatewayClusterResolver, namespace, clusterID string) error {
	if gateways == nil {
		return status.Error(codes.Unavailable, "gateway registry unavailable; cannot verify the gateway's cluster")
	}
	owner, found, err := gateways.NamespaceClusterID(ctx, namespace)
	if err != nil {
		return status.Error(codes.Unavailable, "gateway lookup failed; cannot verify the gateway's cluster")
	}
	if found && owner != clusterID {
		return denied()
	}
	return nil
}

// authorizeUserGRPC applies the HTTP authorization rules (isAuthorized) to a
// caller that is not a control-plane identity. req is nil on a streaming call.
func authorizeUserGRPC(fullMethod string, req interface{}, bindings []BindingSummary) error {
	service, method, ok := splitHypershellMethod(fullMethod)
	if !ok {
		if len(bindings) == 0 {
			return denied()
		}
		return nil
	}
	switch service {
	case "GatewayService":
		switch method {
		case "CreateGateway":
			return allowIf(hasGatewayCreator(bindings))
		case "GetGateway":
			r, ok := req.(*pb.GetGatewayRequest)
			if !ok {
				return unexpectedRequest()
			}
			if !isGatewayAuthorized(http.MethodGet, r.GetId(), bindings) {
				return notFound()
			}
			return nil
		case "UpdateGateway":
			r, ok := req.(*pb.UpdateGatewayRequest)
			if !ok {
				return unexpectedRequest()
			}
			return allowIf(isGatewayAuthorized(http.MethodPatch, r.GetId(), bindings))
		case "DeleteGateway":
			r, ok := req.(*pb.DeleteGatewayRequest)
			if !ok {
				return unexpectedRequest()
			}
			return allowIf(isGatewayAuthorized(http.MethodDelete, r.GetId(), bindings))
		case "ListGateways", "WatchGateways":
			// The HTTP list is filtered to the caller's gateways; the gRPC list
			// and watch are not, so only platform:admin (who sees every gateway
			// over HTTP too) may use them.
			return allowIf(hasPlatformAdmin(bindings))
		}
		// AdjustActiveSandboxCount, SetActiveSandboxCount, SetGatewayVersion:
		// control-plane-owned fields.
		return denied()
	case "ManagedClusterService":
		if isReadMethodName(method) {
			return allowIf(hasDashboardInventoryAccess(bindings, nil))
		}
		return allowIf(hasPlatformAdmin(bindings))
	case "RoleBindingService":
		// Unfiltered by user or gateway; users manage bindings over HTTP.
		return denied()
	}
	return denied()
}

func allowIf(ok bool) error {
	if ok {
		return nil
	}
	return denied()
}
