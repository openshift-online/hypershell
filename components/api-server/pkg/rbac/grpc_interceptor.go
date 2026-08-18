package rbac

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
		if isServiceAccount(username, config.ServiceAccounts) {
			return handler(srv, wrapped)
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
