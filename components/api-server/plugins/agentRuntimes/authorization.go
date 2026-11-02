package agentRuntimes

import (
	"context"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// manageAuthorizer enforces the per-resource tenancy of AgentRuntimes behind
// the /ext middleware gate. The middleware gate admits every gateway:creator,
// so it cannot confine a creator to their own gateways: creating, updating, or
// deleting an AgentRuntime additionally requires gateway:owner or
// gateway:admin on the referenced gateway, or platform:admin (owner-equivalent
// for management, GTW-ACC-08). Without this check any signed-in creator could
// plant an AgentRuntime on another tenant's gateway, causing the control plane
// to reconcile junk resources into that gateway's namespace.
type manageAuthorizer struct {
	bindings rbac.RoleBindingLookup
}

// newManageAuthorizer wires the tenancy check from the environment's service
// registry. It returns nil when RBAC is not enforced (authentication off,
// rbac.Enforced), which the handler treats as "not enforced" (allow),
// mirroring the middleware's enforcement gate.
func newManageAuthorizer(envServices *environments.Services) *manageAuthorizer {
	if !rbac.Enforced() || envServices == nil {
		return nil
	}
	return &manageAuthorizer{bindings: roleBindings.Service(envServices)}
}

// authorizeManageGateway authorizes the caller to manage AgentRuntimes on
// gatewayID (gateway:owner, gateway:admin, or platform:admin).
func (a *manageAuthorizer) authorizeManageGateway(ctx context.Context, gatewayID string) *errors.ServiceError {
	if a == nil {
		return nil
	}
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" {
		return errors.Forbidden("no authenticated caller identity")
	}
	if a.bindings == nil {
		// RBAC is enforced but the lookup seam did not resolve: fail closed.
		return errors.GeneralError("agent runtime authorization is unavailable")
	}
	bindings, err := a.bindings.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return errors.GeneralError("unable to resolve caller access: %s", err)
	}
	if rbac.CanManageGatewayResources(bindings, gatewayID) {
		return nil
	}
	return errors.Forbidden("managing agent runtimes requires gateway:owner or gateway:admin on the target gateway")
}
