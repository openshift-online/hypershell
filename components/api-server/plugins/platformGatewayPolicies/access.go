package platformGatewayPolicies

import (
	"context"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// targetVisibility decides whether the caller may see policy targets and
// exclusions (SEC-RBAC-31): platform-policy:admin, fleet:reader, and
// platform:admin may; a gateway user may not, so the effective-policies read
// never discloses which other gateways a policy covers.
type targetVisibility struct {
	enforced bool
	bindings rbac.RoleBindingLookup
}

func (v targetVisibility) canSeeTargets(ctx context.Context) (bool, *errors.ServiceError) {
	if !v.enforced {
		return true, nil
	}
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" {
		return false, nil
	}
	if v.bindings == nil {
		return false, errors.GeneralError("policy target authorization is unavailable")
	}
	bindings, err := v.bindings.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return false, errors.GeneralError("unable to resolve caller access: %s", err)
	}
	return rbac.CanSeePolicyTargets(bindings), nil
}
