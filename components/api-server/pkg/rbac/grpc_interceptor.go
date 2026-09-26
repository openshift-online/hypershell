package rbac

import (
	"context"
	"strings"
	"time"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

// RBACUnaryInterceptor authorizes a unary call. clusters resolves the caller's
// JWT subject to a registered ManagedCluster; a registered caller is a
// control-plane identity (see control_plane_identity.go). A nil clusters
// disables that exemption, leaving RBAC_SERVICE_ACCOUNTS as the only one.
func RBACUnaryInterceptor(lookup RoleBindingLookup, provisioner UserProvisioner, syncer JWTRoleSyncer, activityRecorder DailyActivityRecorder, clusters RegisteredClusterResolver, config AuthzConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = provisionUserForGRPC(ctx, provisioner, syncer)

		username := auth.GetUsernameFromContext(ctx)
		if !config.EnforceRBAC {
			recordAuthorizedDailyActivityGRPC(ctx, username, config.ServiceAccounts, clusters, activityRecorder)
			return handler(ctx, req)
		}

		if isServiceAccount(username, config.ServiceAccounts) {
			return handler(ctx, req)
		}

		// A registered managed cluster is a control-plane identity, exempt like
		// an allowlisted account (control_plane_identity.go). A failed lookup is
		// Unavailable: never a grant, never a fatal PermissionDenied.
		registered, err := isRegisteredClusterCallerGRPC(ctx, username, clusters)
		if err != nil {
			return nil, err
		}
		if registered {
			return handler(ctx, req)
		}

		// Control-plane-only mutations (the sandbox-count and runtime-version writes) are restricted to
		// control-plane identities when an allowlist is configured. Any principal that
		// reaches here is neither an allowlisted SA nor a registered cluster, so deny outright rather than fall
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

		// nil resolver: this caller is already known not to be a registered cluster.
		recordAuthorizedDailyActivityGRPC(ctx, username, config.ServiceAccounts, nil, activityRecorder)
		return handler(ctx, req)
	}
}

// RBACStreamInterceptor is the streaming counterpart of RBACUnaryInterceptor.
func RBACStreamInterceptor(lookup RoleBindingLookup, provisioner UserProvisioner, syncer JWTRoleSyncer, activityRecorder DailyActivityRecorder, clusters RegisteredClusterResolver, config AuthzConfig) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := provisionUserForGRPC(ss.Context(), provisioner, syncer)
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}

		username := auth.GetUsernameFromContext(ctx)
		if !config.EnforceRBAC {
			recordAuthorizedDailyActivityGRPC(ctx, username, config.ServiceAccounts, clusters, activityRecorder)
			return handler(srv, wrapped)
		}
		if isServiceAccount(username, config.ServiceAccounts) {
			return handler(srv, wrapped)
		}
		registered, err := isRegisteredClusterCallerGRPC(ctx, username, clusters)
		if err != nil {
			return err
		}
		if registered {
			return handler(srv, wrapped)
		}

		// See the unary interceptor: control-plane-only mutations are restricted to
		// control-plane identities when an allowlist is configured. These methods are unary today; guarding the stream
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

		// nil resolver: this caller is already known not to be a registered cluster.
		recordAuthorizedDailyActivityGRPC(ctx, username, config.ServiceAccounts, nil, activityRecorder)
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

// isServiceAccountOnlyMethod reports whether a method is a control-plane-only
// mutation that ordinary role bindings must never reach. AdjustActiveSandboxCount
// and SetActiveSandboxCount write active_sandbox_count. SetGatewayVersion writes
// the observed runtime version. Only the control plane may write these fields.
// Without this guard, isGRPCAuthorized would grant them to any gateway:creator /
// gateway:owner in any namespace. The restriction applies only when a
// service-account allowlist is configured (see the interceptors); an allowlisted
// account or a registered managed cluster passes it (control_plane_identity.go).
func isServiceAccountOnlyMethod(fullMethod string) bool {
	parts := strings.Split(fullMethod, "/")
	if len(parts) < 3 {
		return false
	}
	method := parts[len(parts)-1]
	return method == "AdjustActiveSandboxCount" || method == "SetActiveSandboxCount" || method == "SetGatewayVersion"
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

	jwtRoles := extractJWTRolesFromContext(ctx)
	if len(jwtRoles) > 0 {
		ctx = context.WithValue(ctx, ContextJWTRolesKey, jwtRoles)
	}
	// Always sync even when jwtRoles is empty: SyncJWTRoles applies
	// configured default roles (e.g. gateway:creator) so that users with
	// no Keycloak realm roles still receive their initial bindings.
	if syncer != nil {
		if syncErr := syncer.SyncJWTRoles(ctx, userID, jwtRoles); syncErr != nil {
			glog.Warningf("gRPC JWT role sync failed for %q: %v", username, syncErr)
		}
	}

	return ctx
}

// recordAuthorizedDailyActivityGRPC records the caller as a daily-active user
// unless it is a control-plane identity: an allowlisted account, or a
// registered managed cluster per clusters. A cluster lookup failure skips the
// record rather than counting a possible control plane as a user; it never
// fails the call.
func recordAuthorizedDailyActivityGRPC(ctx context.Context, username string, serviceAccounts []string, clusters RegisteredClusterResolver, activityRecorder DailyActivityRecorder) {
	if activityRecorder == nil || isServiceAccount(username, serviceAccounts) {
		return
	}

	userID := GetUserIDFromContext(ctx)
	if userID == "" {
		return
	}
	if registered, err := isRegisteredClusterCallerGRPC(ctx, username, clusters); err != nil || registered {
		return
	}

	activityRecorder.RecordDailyActivity(ctx, userID, time.Now().UTC())
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
