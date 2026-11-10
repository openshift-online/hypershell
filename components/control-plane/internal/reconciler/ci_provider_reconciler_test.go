package reconciler

import (
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// resolveIdpAlias prefers the explicit spec idp_alias over the derived alias, and
// falls back to the derived alias only when idp_alias is unset.
// Verifies: PLAT-CITR-01.03, PLAT-DATA-24.03
func TestResolveIdpAlias(t *testing.T) {
	const issuer = "https://token.actions.githubusercontent.com"
	derived := deriveIdpAlias("github", issuer) // ci-github-token-actions-githubusercontent-com

	t.Run("explicit idp_alias is preferred over the derived alias", func(t *testing.T) {
		p := &pb.CIProvider{Forge: "github", Issuer: issuer, IdpAlias: "github-actions"}
		if got := resolveIdpAlias(p); got != "github-actions" {
			t.Fatalf("resolveIdpAlias = %q, want %q", got, "github-actions")
		}
		if derived == "github-actions" {
			t.Fatalf("test precondition: derived alias unexpectedly equals the explicit alias")
		}
	})

	t.Run("unset idp_alias falls back to the derived alias", func(t *testing.T) {
		p := &pb.CIProvider{Forge: "github", Issuer: issuer}
		if got := resolveIdpAlias(p); got != derived {
			t.Fatalf("resolveIdpAlias = %q, want derived %q", got, derived)
		}
	})
}
