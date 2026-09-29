package rbac

import (
	"context"
	"time"

	"github.com/golang/glog"
	"google.golang.org/grpc"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
)

// RBACUnaryInterceptor authorizes a unary call. clusters resolves the caller's
// JWT subject to a registered ManagedCluster, which makes it a control-plane
// identity; gateways resolves a gateway to its cluster so a control plane is
// held to its own cluster's gateways (grpc_authorization.go). A nil clusters
// disables the control-plane identity, leaving RBAC_SERVICE_ACCOUNTS as the only
// exemption.
func RBACUnaryInterceptor(lookup RoleBindingLookup, provisioner UserProvisioner, syncer JWTRoleSyncer, activityRecorder DailyActivityRecorder, clusters RegisteredClusterResolver, gateways GatewayClusterResolver, config AuthzConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = provisionUserForGRPC(ctx, provisioner, syncer)
		if err := authorizeGRPC(ctx, info.FullMethod, req, lookup, activityRecorder, clusters, gateways, config); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// RBACStreamInterceptor is the streaming counterpart of RBACUnaryInterceptor.
// The request of a server-streaming call is read by the handler, so streams are
// decided per method; every hypershell stream is a Watch.
func RBACStreamInterceptor(lookup RoleBindingLookup, provisioner UserProvisioner, syncer JWTRoleSyncer, activityRecorder DailyActivityRecorder, clusters RegisteredClusterResolver, gateways GatewayClusterResolver, config AuthzConfig) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := provisionUserForGRPC(ss.Context(), provisioner, syncer)
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}
		if err := authorizeGRPC(ctx, info.FullMethod, nil, lookup, activityRecorder, clusters, gateways, config); err != nil {
			return err
		}
		return handler(srv, wrapped)
	}
}

// authorizeGRPC decides one call. Order matters: a registered control plane is
// scoped to its own cluster even when it is also on the allowlist, and the
// allowlist exemption does not cover ManagedCluster record writes.
func authorizeGRPC(ctx context.Context, fullMethod string, req interface{}, lookup RoleBindingLookup, activityRecorder DailyActivityRecorder, clusters RegisteredClusterResolver, gateways GatewayClusterResolver, config AuthzConfig) error {
	username := auth.GetUsernameFromContext(ctx)
	if !config.EnforceRBAC {
		recordAuthorizedDailyActivityGRPC(ctx, username, config.ServiceAccounts, clusters, activityRecorder)
		return nil
	}

	// A failed lookup is Unavailable: never a grant, never a fatal
	// PermissionDenied (a control plane retries Unavailable).
	clusterID, registered, err := registeredClusterIDGRPC(ctx, username, clusters)
	if err != nil {
		return err
	}
	if registered {
		return authorizeControlPlaneGRPC(ctx, clusterID, fullMethod, req, gateways)
	}

	if isServiceAccount(username, config.ServiceAccounts) {
		if isManagedClusterRecordMutation(fullMethod) {
			return denied()
		}
		return nil
	}

	if username == "" {
		// Unauthenticated: with JWT on, only --auth-bypass-methods (health,
		// reflection) get here. No hypershell.v1 method is served without an
		// identity when RBAC is enforced.
		if _, _, hypershell := splitHypershellMethod(fullMethod); hypershell {
			return denied()
		}
		return nil
	}
	userID := GetUserIDFromContext(ctx)
	if userID == "" {
		return denied()
	}

	bindings, err := lookup.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return denied()
	}
	if err := authorizeUserGRPC(fullMethod, req, bindings); err != nil {
		return err
	}

	// nil resolver: this caller is already known not to be a registered cluster.
	recordAuthorizedDailyActivityGRPC(ctx, username, config.ServiceAccounts, nil, activityRecorder)
	return nil
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
