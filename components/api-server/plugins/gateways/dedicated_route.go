package gateways

import (
	"context"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// Gateway dedicated_route request field (create-only).
//
// dedicated_route is the user-facing opt-in to the dedicated per-gateway route
// and wildcard certificate path. The control plane selects that path from the
// platform-managed route JSONB carrying a non-null "certificate" sub-key, so the
// API server translates a create request into that route document when no route
// was authored, and the control plane keeps keying off route.certificate
// unchanged. The flag itself is persisted on the gateway row for display and
// audit; it is user-writable at create only, the same ownership class as
// placement/cluster_id (PLAT-DATA-23).
const (
	// dedicatedRouteCertificateJSON is the server-applied route document written for
	// a create that requests dedicated_route without supplying a route. It is the
	// route opt-in the control plane already keys on
	// (docs/investigations/per-gateway-certificates.md): "certificate": {} selects
	// dedicated per-gateway certificate provisioning.
	dedicatedRouteCertificateJSON = `{"enabled":true,"certificate":{}}`

	// dedicatedRouteImmutableReason is the user-facing reason for rejecting a
	// dedicated_route change after creation. Provisioning the dedicated route and
	// certificate happens once, when the gateway is created, so the choice cannot
	// follow a later edit.
	dedicatedRouteImmutableReason = "dedicated_route is create-only and cannot be added, removed, or changed after gateway creation; delete and re-create the gateway to change dedicated route and certificate provisioning"

	// dedicatedRouteForbiddenReason is the actionable 403 for a create that
	// requests dedicated_route without the entitlement. It names the role to ask
	// for and says why the platform does not grant it to every creator.
	dedicatedRouteForbiddenReason = "dedicated_route requires the " + roles.RoleGatewayDedicatedRouteCreator + " role (platform:admin also qualifies): a dedicated per-gateway route and certificate is a rate-limited resource, so it is not granted to every gateway:creator"

	// dedicatedRouteImportForbiddenReason rejects a declarative-import create that
	// requests dedicated_route: import writes are authored by a Repository, and the
	// entitlement is granted to a user, so an import cannot carry it.
	dedicatedRouteImportForbiddenReason = "dedicated_route cannot be requested by an import caller: the " + roles.RoleGatewayDedicatedRouteCreator + " entitlement is granted per user, and a declarative import is authored by a Repository rather than by a user who can hold it; create the gateway directly (hsctl or the web console) to request a dedicated route"
)

// dedicatedRouteRequested reports whether an optional dedicated_route value means
// "requested". nil (never set) and false both mean not requested.
func dedicatedRouteRequested(value *bool) bool {
	return value != nil && *value
}

// dedicatedRouteChanged reports whether the create-only dedicated_route opt-in
// differs between the stored gateway and an incoming update. Adding or removing
// the request is a change; nil vs false is not, because both mean "not requested",
// so an idempotent client that resends a whole object is not rejected.
func dedicatedRouteChanged(current, next *bool) bool {
	return dedicatedRouteRequested(current) != dedicatedRouteRequested(next)
}

// applyGatewayDedicatedRouteCreateRequest is the service-funnel half of the
// dedicated_route contract. It rejects a GitOps-import create that requests the
// flag (imports may not opt a gateway in; design point 4) and otherwise
// translates the request into the platform-managed route opt-in. An explicit route
// already present - which under the field policy only a control-plane internal
// write can supply - always wins: the request flag never overwrites it.
func applyGatewayDedicatedRouteCreateRequest(kind rbac.GatewayWriterKind, gateway *Gateway) *errors.ServiceError {
	if !dedicatedRouteRequested(gateway.DedicatedRoute) {
		return nil
	}
	if kind == rbac.GatewayWriterImport {
		return errors.Forbidden("%s", dedicatedRouteImportForbiddenReason)
	}
	if gateway.Route == nil {
		route := dedicatedRouteCertificateJSON
		gateway.Route = &route
	}
	return nil
}

// dedicatedRouteAuthorizer enforces the dedicated_route entitlement at the create
// boundaries. The check is body-dependent - the request flags a create - so it
// cannot live in the path-based authorization middleware: the REST middleware
// AuthorizeApi decides the route, not the body, and never sees dedicated_route. The
// REST Create handler and the gRPC CreateGateway handler both call it, mirroring
// how each otherwise authorizes creation (gateway:creator for the path).
type dedicatedRouteAuthorizer struct {
	bindingLookup rbac.RoleBindingLookup
	enforceRBAC   bool
}

// newDedicatedRouteAuthorizer builds the create gate. bindingLookup is the role
// binding service (nil when that plugin is absent, which fails closed) and
// enforceRBAC mirrors the RBAC middleware's enforcement gate (rbac.Enforced),
// so the gate is a
// no-op exactly when request-level authorization is.
func newDedicatedRouteAuthorizer(bindingLookup rbac.RoleBindingLookup, enforceRBAC bool) dedicatedRouteAuthorizer {
	return dedicatedRouteAuthorizer{bindingLookup: bindingLookup, enforceRBAC: enforceRBAC}
}

// authorizeCreate rejects a create that requests dedicated_route from a caller
// without the entitlement.
//
// Only a user-authored create is decided here: a control-plane internal create is
// privileged and bypasses the RBAC check (design point 4), and an import create is
// rejected by the service funnel with its own message, because a control-plane
// identity holds no user bindings. A missing caller identity, an unwired lookup, or
// a lookup failure all fail closed with the actionable rejection rather than being
// treated as a grant; a lookup failure is reported as a server error so a transient
// database problem does not tell the caller to go ask for a role it may already hold.
func (a dedicatedRouteAuthorizer) authorizeCreate(ctx context.Context, requested *bool) *errors.ServiceError {
	if !dedicatedRouteRequested(requested) {
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
		return errors.Forbidden("%s", dedicatedRouteForbiddenReason)
	}
	bindings, err := a.bindingLookup.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return errors.GeneralError("failed to look up the role bindings of user %s: %s", userID, err)
	}
	if rbac.CanRequestGatewayDedicatedRoute(bindings) {
		return nil
	}
	return errors.Forbidden("%s", dedicatedRouteForbiddenReason)
}
