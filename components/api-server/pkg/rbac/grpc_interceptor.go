package rbac

import (
	"context"
	"strings"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

func RBACUnaryInterceptor(lookup RoleBindingLookup, provisioner UserProvisioner, config AuthzConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = provisionUserForGRPC(ctx, provisioner)

		if !config.EnforceRBAC {
			return handler(ctx, req)
		}

		username := auth.GetUsernameFromContext(ctx)
		if isServiceAccount(username, config.ServiceAccounts) {
			return handler(ctx, req)
		}

		// Control-plane-only mutations (the sandbox-count writes) are restricted to
		// the service-account allowlist when one is configured. Any principal that
		// reaches here is not an allowlisted SA, so deny outright rather than fall
		// through to the coarse role check, which grants gateway:creator/owner every
		// non-read method in any namespace. With no allowlist configured, fall
		// through as a documented fallback to the standard role check.
		if len(config.ServiceAccounts) > 0 && isServiceAccountOnlyMethod(info.FullMethod) {
			return nil, status.Errorf(codes.PermissionDenied, "forbidden")
		}

		userID := GetUserIDFromContext(ctx)
		if userID == "" {
			if username != "" {
				return nil, status.Errorf(codes.PermissionDenied, "forbidden")
			}
			return handler(ctx, req)
		}

		bindings, err := lookup.FindBindingsByUserID(ctx, userID)
		if err != nil {
			return nil, status.Errorf(codes.PermissionDenied, "forbidden")
		}

		if !isGRPCAuthorized(info.FullMethod, bindings) {
			return nil, status.Errorf(codes.PermissionDenied, "forbidden")
		}

		return handler(ctx, req)
	}
}

func RBACStreamInterceptor(lookup RoleBindingLookup, provisioner UserProvisioner, config AuthzConfig) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := provisionUserForGRPC(ss.Context(), provisioner)
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}

		if !config.EnforceRBAC {
			return handler(srv, wrapped)
		}

		username := auth.GetUsernameFromContext(ctx)
		if isServiceAccount(username, config.ServiceAccounts) {
			return handler(srv, wrapped)
		}

		// See the unary interceptor: control-plane-only mutations are SA-only when an
		// allowlist is configured. These methods are unary today; guarding the stream
		// path too keeps the two interceptors symmetric if that ever changes.
		if len(config.ServiceAccounts) > 0 && isServiceAccountOnlyMethod(info.FullMethod) {
			return status.Errorf(codes.PermissionDenied, "forbidden")
		}

		userID := GetUserIDFromContext(ctx)
		if userID == "" {
			if username != "" {
				return status.Errorf(codes.PermissionDenied, "forbidden")
			}
			return handler(srv, wrapped)
		}

		bindings, err := lookup.FindBindingsByUserID(ctx, userID)
		if err != nil {
			return status.Errorf(codes.PermissionDenied, "forbidden")
		}

		if !isGRPCAuthorized(info.FullMethod, bindings) {
			return status.Errorf(codes.PermissionDenied, "forbidden")
		}

		return handler(srv, wrapped)
	}
}

func isGRPCReadMethod(fullMethod string) bool {
	parts := strings.Split(fullMethod, "/")
	if len(parts) < 3 {
		return false
	}
	method := parts[len(parts)-1]
	return strings.HasPrefix(method, "Get") ||
		strings.HasPrefix(method, "List") ||
		strings.HasPrefix(method, "Watch")
}

func isGRPCAuthorized(fullMethod string, bindings []BindingSummary) bool {
	if len(bindings) == 0 {
		return false
	}

	for _, b := range bindings {
		if b.RoleName == "gateway:creator" {
			return true
		}
		if b.RoleName == "gateway:owner" {
			return true
		}
		if b.RoleName == "gateway:viewer" && isGRPCReadMethod(fullMethod) {
			return true
		}
	}
	return false
}

// isServiceAccountOnlyMethod reports whether a method is a control-plane-only
// mutation that ordinary role bindings must never reach. AdjustActiveSandboxCount
// and SetActiveSandboxCount write the control-plane-owned active_sandbox_count and
// are issued solely by the control plane's service account; without this guard
// isGRPCAuthorized would grant them to any gateway:creator / gateway:owner in any
// namespace. The restriction applies only when a service-account allowlist is
// configured (see the interceptors).
func isServiceAccountOnlyMethod(fullMethod string) bool {
	parts := strings.Split(fullMethod, "/")
	if len(parts) < 3 {
		return false
	}
	method := parts[len(parts)-1]
	return method == "AdjustActiveSandboxCount" || method == "SetActiveSandboxCount"
}

func isGRPCDeleteMethod(fullMethod string) bool {
	parts := strings.Split(fullMethod, "/")
	if len(parts) < 3 {
		return false
	}
	return strings.HasPrefix(parts[len(parts)-1], "Delete")
}

func provisionUserForGRPC(ctx context.Context, provisioner UserProvisioner) context.Context {
	if provisioner == nil {
		return ctx
	}

	username := auth.GetUsernameFromContext(ctx)
	if username == "" {
		return ctx
	}

	payload := &auth.Payload{Username: username}
	userID, err := provisioner.UpsertFromJWT(ctx, payload)
	if err != nil {
		glog.Warningf("gRPC user provisioning failed for %q: %v", username, err)
		return ctx
	}

	return context.WithValue(ctx, ContextUserIDKey, userID)
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

func isServiceAccount(username string, serviceAccounts []string) bool {
	if username == "" || len(serviceAccounts) == 0 {
		return false
	}
	for _, sa := range serviceAccounts {
		if sa == username {
			return true
		}
	}
	return false
}
