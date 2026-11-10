package gateways

import (
	"context"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// Gateway workspace_mode operator opt-in (create-only).
//
// workspace_mode is platform-managed and server-defaulted to "managed"
// (OSG-CORE-18), with one narrow create-time exception: an authorized caller MAY
// select "operator" at create (the per-gateway operator-managed workspace opt-in,
// OSG-OPWS-01). "operator" is the ONLY caller-settable value; "shared"/"managed"
// from a caller are rejected naming the field, and an entitled "operator"
// requires the platform-wide gateway:operator-workspace-creator role (SEC-RBAC-27).
// The stored value stays immutable after create for every writer class.
const (
	// operatorWorkspaceValue is the single workspace_mode a caller may select.
	operatorWorkspaceValue = "operator"

	// operatorWorkspaceForbiddenReason is the actionable 403 for a create that
	// selects operator mode without the entitlement. It names the role to ask for
	// and says why the platform does not grant it to every creator.
	operatorWorkspaceForbiddenReason = "workspace_mode=operator requires the " + roles.RoleGatewayOperatorWorkspaceCreator + " role (platform:admin also qualifies): operator mode gives a trusted external operator ownership of the gateway's workspace namespaces, so it is not granted to every gateway:creator"

	// operatorWorkspaceImportForbiddenReason rejects a declarative-import create
	// that sets workspace_mode: import writes are authored by a Repository, and the
	// entitlement is granted to a user, so an import cannot carry it.
	operatorWorkspaceImportForbiddenReason = "workspace_mode cannot be set by an import caller: the " + roles.RoleGatewayOperatorWorkspaceCreator + " entitlement is granted per user, and a declarative import is authored by a Repository rather than by a user who can hold it; create the gateway directly (hsctl or the web console) to select operator mode"
)

// operatorWorkspaceRequested reports whether an optional workspace_mode value
// means "operator selected". nil, empty, and any non-operator value are not an
// operator request (a non-operator caller value is handled by the create-request
// policy, not here).
func operatorWorkspaceRequested(mode *string) bool {
	return mode != nil && strings.TrimSpace(*mode) == operatorWorkspaceValue
}

// applyGatewayWorkspaceModeCreateRequest is the service-funnel half of the
// workspace_mode operator opt-in. For a non-privileged caller it rejects a
// GitOps-import create that sets the field at all (imports may not opt a gateway
// in) and rejects a user-supplied value other than "operator" (only operator is
// caller-settable; "shared"/"managed" are server-defaulted, OSG-OPWS-01.04). A
// control-plane internal write is privileged and keeps full access; the value is
// still defaulted and vocabulary-validated afterward by
// applyGatewayWorkspaceCreateDefault.
func applyGatewayWorkspaceModeCreateRequest(kind rbac.GatewayWriterKind, gateway *Gateway) *errors.ServiceError {
	if kind.Privileged() {
		return nil
	}
	if gateway.WorkspaceMode == nil || strings.TrimSpace(*gateway.WorkspaceMode) == "" {
		return nil
	}
	if kind == rbac.GatewayWriterImport {
		return errors.Forbidden("%s", operatorWorkspaceImportForbiddenReason)
	}
	if strings.TrimSpace(*gateway.WorkspaceMode) != operatorWorkspaceValue {
		return gatewayFieldForbidden(kind, "workspace_mode")
	}
	return nil
}

// operatorWorkspaceAuthorizer enforces the operator-workspace entitlement at the
// create boundaries. The check is body-dependent - the request selects operator
// mode - so it cannot live in the path-based authorization middleware, which
// decides the route, not the body, and never sees workspace_mode. The REST Create
// handler and the gRPC CreateGateway handler both call it, mirroring how each
// otherwise authorizes creation (gateway:creator for the path).
type operatorWorkspaceAuthorizer struct {
	bindingLookup rbac.RoleBindingLookup
	enforceRBAC   bool
}

// newOperatorWorkspaceAuthorizer builds the create gate. bindingLookup is the
// role binding service (nil when that plugin is absent, which fails closed) and
// enforceRBAC mirrors the RBAC middleware's enforcement gate (rbac.Enforced), so
// the gate is a no-op exactly when request-level authorization is.
func newOperatorWorkspaceAuthorizer(bindingLookup rbac.RoleBindingLookup, enforceRBAC bool) operatorWorkspaceAuthorizer {
	return operatorWorkspaceAuthorizer{bindingLookup: bindingLookup, enforceRBAC: enforceRBAC}
}

// authorizeCreate rejects a create that selects operator mode from a caller
// without the entitlement.
//
// Only a user-authored create is decided here: a control-plane internal create is
// privileged and bypasses the RBAC check, and an import create is rejected by the
// service funnel with its own message, because a control-plane identity holds no
// user bindings. A missing caller identity, an unwired lookup, or a lookup failure
// all fail closed with the actionable rejection rather than being treated as a
// grant; a lookup failure is reported as a server error so a transient database
// problem does not tell the caller to go ask for a role it may already hold.
func (a operatorWorkspaceAuthorizer) authorizeCreate(ctx context.Context, requested *string) *errors.ServiceError {
	if !operatorWorkspaceRequested(requested) {
		return nil
	}
	if !a.enforceRBAC {
		return nil
	}
	if rbac.GatewayWriterKindFromContext(ctx) != rbac.GatewayWriterUser {
		return nil
	}
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" || a.bindingLookup == nil {
		return errors.Forbidden("%s", operatorWorkspaceForbiddenReason)
	}
	bindings, err := a.bindingLookup.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return errors.GeneralError("failed to look up the role bindings of user %s: %s", userID, err)
	}
	if rbac.CanRequestGatewayOperatorWorkspace(bindings) {
		return nil
	}
	return errors.Forbidden("%s", operatorWorkspaceForbiddenReason)
}
