package cedar

import (
	"fmt"

	"github.com/cedar-policy/cedar-go"
	"github.com/cedar-policy/cedar-go/types"
	xast "github.com/cedar-policy/cedar-go/x/exp/ast"
)

// basePermitText is the single built-in base permit that makes admission
// allow-by-default (PLAT-PGP-02). The interceptor evaluates it together with the
// forbid statements of every applicable policy; a request is allowed only when
// no forbid matches. It is also added to the evaluated set for the write-time
// default-request check so a decision reflects forbid-overrides-permit.
const basePermitText = `permit (principal, action, resource);`

// BasePermitText returns the built-in base permit statement (PLAT-PGP-02).
func BasePermitText() string { return basePermitText }

// Annotate strips any author-written statement annotations and applies
// @id("<policyName>") to every statement, returning the annotated Cedar text in
// statement order. The control plane owns the @id annotation (PLAT-PGP-14), so
// Cedar can report which policy decided a result for denial text and audit
// findings. Statement order is preserved so the output is deterministic, which
// the bundle revision hash and mutation replay depend on (PLAT-PGP-11,
// PLAT-PGP-12).
func Annotate(cedarText, policyName string) (string, error) {
	policies, err := cedar.NewPolicyListFromBytes("policy.cedar", []byte(cedarText))
	if err != nil {
		return "", fmt.Errorf("parse cedar: %w", err)
	}
	if len(policies) == 0 {
		return "", fmt.Errorf("policy %q has no statements", policyName)
	}
	for _, p := range policies {
		ap := p.AST()
		ap.Annotations = []xast.AnnotationType{{Key: "id", Value: types.String(policyName)}}
	}
	return string(policies.MarshalCedar()), nil
}
