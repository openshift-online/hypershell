package ciTrusts

import (
	"context"
	"os"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// tenancyAuthorizer enforces the per-gateway tenancy of CITrusts behind the
// /ext middleware gate (SEC-RBAC-22). The middleware gate admits any caller with
// a gateway binding, so it cannot confine a caller to the gateways they control:
// creating, updating, or deleting a CITrust additionally requires gateway:owner
// or gateway:admin on the referenced gateway, or platform:admin (owner-equivalent
// for management, GTW-ACC-08); reading a gateway's trusts additionally requires
// owner, admin, or viewer on that gateway (or platform:admin). Without these
// checks a caller bound to one gateway could author trusts on, or read the
// trusts of, a gateway they do not control.
type tenancyAuthorizer struct {
	bindings rbac.RoleBindingLookup
}

// newTenancyAuthorizer wires the tenancy check from the environment's service
// registry. It returns nil when RBAC is not enforced, which the handler treats
// as "not enforced" (allow), mirroring the middleware's RBAC_ENFORCE gate and
// the agentRuntimes manageAuthorizer.
func newTenancyAuthorizer(envServices *environments.Services) *tenancyAuthorizer {
	if os.Getenv("RBAC_ENFORCE") != "true" || envServices == nil {
		return nil
	}
	return &tenancyAuthorizer{bindings: roleBindings.Service(envServices)}
}

// callerBindings resolves the caller's role bindings. It fails closed when the
// caller has no identity or the lookup seam is unavailable.
func (a *tenancyAuthorizer) callerBindings(ctx context.Context) ([]rbac.BindingSummary, *errors.ServiceError) {
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, errors.Forbidden("no authenticated caller identity")
	}
	if a.bindings == nil {
		// RBAC is enforced but the lookup seam did not resolve: fail closed.
		return nil, errors.GeneralError("CI trust authorization is unavailable")
	}
	bindings, err := a.bindings.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return nil, errors.GeneralError("unable to resolve caller access: %s", err)
	}
	return bindings, nil
}

// authorizeCreate authorizes the caller to create a CITrust referencing
// gatewayID (gateway:owner, gateway:admin, or platform:admin). The gatewayID is
// caller-supplied in the request body, so there is no existing record to mask: a
// failure is always 403 (SEC-RBAC-22.03/22.04).
func (a *tenancyAuthorizer) authorizeCreate(ctx context.Context, gatewayID string) *errors.ServiceError {
	if a == nil {
		return nil
	}
	bindings, svcErr := a.callerBindings(ctx)
	if svcErr != nil {
		return svcErr
	}
	if rbac.CanManageGatewayResources(bindings, gatewayID) {
		return nil
	}
	return errors.Forbidden("managing CI trusts requires gateway:owner or gateway:admin on the referenced gateway")
}

// authorizeManageExisting authorizes update or delete of an existing CITrust on
// gatewayID. A caller who may manage the gateway passes; a caller who may read
// but not manage it (a viewer) is 403 (the record is already visible to them,
// SEC-RBAC-22.03); a caller with no visibility of the gateway is masked as 404
// so the endpoint does not disclose the trust's existence by id (SEC-RBAC-10 /
// SEC-RBAC-22.04).
func (a *tenancyAuthorizer) authorizeManageExisting(ctx context.Context, gatewayID string) *errors.ServiceError {
	if a == nil {
		return nil
	}
	bindings, svcErr := a.callerBindings(ctx)
	if svcErr != nil {
		return svcErr
	}
	if rbac.CanManageGatewayResources(bindings, gatewayID) {
		return nil
	}
	if rbac.CanReadGatewayResources(bindings, gatewayID) {
		return errors.Forbidden("managing CI trusts requires gateway:owner or gateway:admin on the referenced gateway")
	}
	return errors.NotFound("ci trust not found")
}

// authorizeRead authorizes the caller to read a single CITrust referencing
// gatewayID. A caller with no owner/admin/viewer binding on that gateway (and no
// platform:admin) is masked as 404 so the endpoint does not disclose the
// trust's existence (SEC-RBAC-10 / SEC-RBAC-22).
func (a *tenancyAuthorizer) authorizeRead(ctx context.Context, gatewayID string) *errors.ServiceError {
	if a == nil {
		return nil
	}
	bindings, svcErr := a.callerBindings(ctx)
	if svcErr != nil {
		return svcErr
	}
	if rbac.CanReadGatewayResources(bindings, gatewayID) {
		return nil
	}
	return errors.NotFound("ci trust not found")
}

// readableGatewayScope reports the gateways whose CITrusts the caller may list.
// unrestricted is true for platform:admin and when RBAC is not enforced (nil
// authorizer); otherwise gatewayIDs is the caller's owner/admin/viewer-bound
// gateways. Applying this as a query filter (not a post-pagination filter)
// keeps paging and the reported Total confined to the caller's tenancy, so a
// listing can neither under-fill pages nor disclose the global trust count
// (SEC-RBAC-22.05).
func (a *tenancyAuthorizer) readableGatewayScope(ctx context.Context) (gatewayIDs []string, unrestricted bool, svcErr *errors.ServiceError) {
	if a == nil {
		return nil, true, nil
	}
	bindings, svcErr := a.callerBindings(ctx)
	if svcErr != nil {
		return nil, false, svcErr
	}
	seen := map[string]bool{}
	for _, b := range bindings {
		if b.RoleName == "platform:admin" {
			return nil, true, nil
		}
		if b.Scope != "gateway" || b.GatewayID == nil {
			continue
		}
		switch b.RoleName {
		case "gateway:owner", "gateway:admin", "gateway:viewer":
			if !seen[*b.GatewayID] {
				seen[*b.GatewayID] = true
				gatewayIDs = append(gatewayIDs, *b.GatewayID)
			}
		}
	}
	return gatewayIDs, false, nil
}
