package roleBindings

import (
	"context"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/extsearch"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// readScope is the set of RoleBindings a caller may read through the low-level
// role_bindings resource (SEC-RBAC-10). A caller sees its own bindings and every
// binding on a gateway where it holds any binding, which is the audience of the
// gateway access list (GTW-ACC-03). platform:admin sees every binding.
type readScope struct {
	all        bool
	userID     string
	gatewayIDs []string
}

// resolveReadScope computes the caller's read scope. Without enforcement there is
// no caller identity to scope by, so every binding is visible (SEC-RBAC-14).
func resolveReadScope(ctx context.Context, lookup interface {
	FindGatewayIDsByUserID(ctx context.Context, userID string) ([]string, *errors.ServiceError)
}) (readScope, *errors.ServiceError) {
	if !rbac.Enforced() {
		return readScope{all: true}, nil
	}
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" {
		// No identity: nothing is visible.
		return readScope{}, nil
	}
	if rbac.HasPlatformAdminRole(ctx, userID) {
		return readScope{all: true}, nil
	}
	gatewayIDs, err := lookup.FindGatewayIDsByUserID(ctx, userID)
	if err != nil {
		return readScope{}, err
	}
	return readScope{userID: userID, gatewayIDs: gatewayIDs}, nil
}

// canSee reports whether rb is inside the scope.
func (s readScope) canSee(rb *RoleBinding) bool {
	if s.all {
		return true
	}
	if s.userID == "" || rb == nil {
		return false
	}
	if rb.UserID != nil && *rb.UserID == s.userID {
		return true
	}
	if rb.Scope != ScopeGateway || rb.GatewayID == nil {
		return false
	}
	for _, id := range s.gatewayIDs {
		if id == *rb.GatewayID {
			return true
		}
	}
	return false
}

// restrictSearch returns the list search with the scope applied. The caller's
// own search is validated on its own first: only a complete, valid expression
// can be wrapped in parentheses without closing them early, so a crafted value
// cannot widen the result (SEC-RBAC-10.05).
func (s readScope) restrictSearch(search string) (string, *errors.ServiceError) {
	if s.all {
		return search, nil
	}
	if s.userID == "" {
		return "", errors.Forbidden("missing caller identity")
	}
	if strings.TrimSpace(search) != "" {
		if err := extsearch.ValidateSearchColumns(search, &RoleBinding{}); err != nil {
			return "", err
		}
	}
	filter := "user_id = " + quoteTSL(s.userID)
	if len(s.gatewayIDs) > 0 {
		quoted := make([]string, len(s.gatewayIDs))
		for i, id := range s.gatewayIDs {
			quoted[i] = quoteTSL(id)
		}
		filter = "(" + filter + " or gateway_id in (" + strings.Join(quoted, ",") + "))"
	}
	if strings.TrimSpace(search) == "" {
		return filter, nil
	}
	return "(" + search + ") and " + filter, nil
}

// quoteTSL quotes a stored identifier as a TSL string literal. Identifiers are
// server-generated, but a quote is still dropped so a value can never end the
// literal early.
func quoteTSL(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "") + "'"
}
