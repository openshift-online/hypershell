package secretSources

import (
	"context"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// manageAuthorizer enforces the per-resource tenancy of secret sources behind
// the /ext middleware gate. The middleware gate admits every gateway:creator
// (the authoring posture of the Extension API kinds), so it cannot confine a
// creator to their own gateways: managing a SecretSource additionally requires
// gateway:owner or gateway:admin on the gateway the target AgentRuntime runs
// on, or platform:admin (owner-equivalent for management, GTW-ACC-08). Without
// this check any signed-in creator could overwrite another tenant's secret at
// its deterministic location or pin a reference at any store path.
type manageAuthorizer struct {
	bindings rbac.RoleBindingLookup
	runtimes runtimeGatewayResolver
}

// runtimeGatewayResolver is the slice of the AgentRuntimes service the tenancy
// check needs: resolving an AgentRuntime to the gateway it runs on.
type runtimeGatewayResolver interface {
	Get(ctx context.Context, id string) (*agentRuntimes.AgentRuntime, *errors.ServiceError)
}

// newManageAuthorizer wires the tenancy check from the environment's service
// registry. It returns nil when RBAC is not enforced (authentication off,
// rbac.Enforced), which the handler treats as "not enforced" (allow),
// mirroring the middleware's enforcement gate. When RBAC IS enforced the
// lookup seams must resolve; a missing seam fails closed inside the check
// rather than allowing.
func newManageAuthorizer(envServices *environments.Services) *manageAuthorizer {
	if !rbac.Enforced() || envServices == nil {
		return nil
	}
	return &manageAuthorizer{
		bindings: roleBindings.Service(envServices),
		runtimes: agentRuntimes.Service(envServices),
	}
}

// authorizeManageRuntime resolves the target AgentRuntime and authorizes the
// caller to manage secret sources on its gateway. A runtime that does not
// exist is a 404: a SecretSource references an existing AgentRuntime
// (data-model.spec.md), and refusing here also blocks planting sources on
// nonexistent runtimes.
func (a *manageAuthorizer) authorizeManageRuntime(ctx context.Context, agentRuntimeID string) *errors.ServiceError {
	if a == nil {
		return nil
	}
	if a.runtimes == nil || a.bindings == nil {
		// RBAC is enforced but a lookup seam did not resolve: fail closed.
		return errors.GeneralError("secret source authorization is unavailable")
	}
	runtime, svcErr := a.runtimes.Get(ctx, agentRuntimeID)
	if svcErr != nil {
		return svcErr
	}
	return a.authorizeManageGateway(ctx, runtime.GatewayId)
}

// authorizeManageGateway authorizes the caller to manage gateway-scoped
// resources on gatewayID (gateway:owner, gateway:admin, or platform:admin).
func (a *manageAuthorizer) authorizeManageGateway(ctx context.Context, gatewayID string) *errors.ServiceError {
	if a == nil {
		return nil
	}
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" {
		return errors.Forbidden("no authenticated caller identity")
	}
	if a.bindings == nil {
		return errors.GeneralError("secret source authorization is unavailable")
	}
	bindings, err := a.bindings.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return errors.GeneralError("unable to resolve caller access: %s", err)
	}
	if rbac.CanManageGatewayResources(bindings, gatewayID) {
		return nil
	}
	return errors.Forbidden("managing secret sources requires gateway:owner or gateway:admin on the target gateway")
}
