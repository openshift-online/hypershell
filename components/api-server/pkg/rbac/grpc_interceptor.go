package rbac

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

func RBACUnaryInterceptor(lookup RoleBindingLookup, provisioner UserProvisioner, syncer JWTRoleSyncer, config AuthzConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = provisionUserForGRPC(ctx, provisioner, syncer)

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

func RBACStreamInterceptor(lookup RoleBindingLookup, provisioner UserProvisioner, syncer JWTRoleSyncer, config AuthzConfig) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := provisionUserForGRPC(ss.Context(), provisioner, syncer)
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}

		if !config.EnforceRBAC {
			return handler(srv, wrapped)
		}

		username := auth.GetUsernameFromContext(ctx)
		if isManagedDatabaseTombstoneReplay(ctx, info.FullMethod) {
			// Historical tombstones are control-plane recovery data.
			// Unlike the ordinary live watch, replay is never available through role
			// bindings or the no-allowlist fallback.
			if len(config.ServiceAccounts) == 0 || !isServiceAccount(username, config.ServiceAccounts) {
				return status.Errorf(codes.PermissionDenied, "forbidden")
			}
			return handler(srv, wrapped)
		}
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

func isManagedDatabaseTombstoneReplay(ctx context.Context, fullMethod string) bool {
	if fullMethod != "/hypershell.v1.ManagedDatabaseService/WatchManagedDatabases" {
		return false
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	values := md.Get("hypershell-managed-database-replay")
	return len(values) == 1 && values[0] == "deleted-v1"
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
		if b.RoleName == "platform:admin" {
			if isGRPCReadMethod(fullMethod) || isGRPCDeleteMethod(fullMethod) {
				return true
			}
		}
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

func provisionUserForGRPC(ctx context.Context, provisioner UserProvisioner, syncer JWTRoleSyncer) context.Context {
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

	ctx = context.WithValue(ctx, ContextUserIDKey, userID)

	if syncer != nil {
		jwtRoles := extractJWTRolesFromContext(ctx)
		if len(jwtRoles) > 0 {
			ctx = context.WithValue(ctx, ContextJWTRolesKey, jwtRoles)
			if syncErr := syncer.SyncJWTRoles(ctx, userID, jwtRoles); syncErr != nil {
				glog.Warningf("gRPC JWT role sync failed for %q: %v", username, syncErr)
			}
		}
	}

	return ctx
}

func extractJWTRolesFromContext(ctx context.Context) []string {
	token, err := auth.TokenFromContext(ctx)
	if err != nil {
		return nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil
	}

	realmAccess, ok := claims["realm_access"]
	if !ok {
		return nil
	}

	raMap, ok := realmAccess.(map[string]interface{})
	if !ok {
		return nil
	}

	rolesRaw, ok := raMap["roles"]
	if !ok {
		return nil
	}

	rolesSlice, ok := rolesRaw.([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, 0, len(rolesSlice))
	for _, r := range rolesSlice {
		if s, ok := r.(string); ok {
			result = append(result, s)
		}
	}
	return result
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
