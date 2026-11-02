package authz_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestCIProviderDeleteThenRecreateSameIssuer is the soft-delete convergence
// regression for the ci_providers ISSUER unique index (SDLC-CONF-13, FINDING 1,
// same root cause as the name index). The issuer unique index must be PARTIAL
// (WHERE deleted_at IS NULL): a gitops reconcile / re-apply of a CIProvider deletes
// the record then re-creates it reusing the SAME issuer. GORM soft-delete filters
// deleted_at IS NULL on reads, so find-by-issuer sees nothing after the delete and
// the reconcile re-issues CREATE with the same issuer. A FULL unique index would
// still carry the soft-deleted row's issuer and reject that INSERT with a 409,
// breaking convergence. A partial index constrains only live rows, so the re-create
// succeeds.
//
// The two creates use DISTINCT explicit names (name derives from forge, so two
// providers on the same forge would otherwise collide on the name index) to isolate
// the issuer constraint under test. Without the partial issuer index the re-create
// is a 409 and this test fails.
func TestCIProviderDeleteThenRecreateSameIssuer(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	issuer := "https://issuer.example/reuse-" + short()

	create := func(name string) string {
		req := openapi.NewCIProvider("gitlab", issuer, "https://example.com/jwks", "hypershell")
		req.Name = openapi.PtrString(name)
		cp, _, err := client.DefaultAPI.CreateCIProvider(env.owner.rest).CIProvider(*req).Execute()
		Expect(err).NotTo(HaveOccurred(), "create ci_provider reusing issuer %q: %s", issuer, test.APIErrorReason(err))
		return cp.GetId()
	}

	id1 := create("cip-first-" + short())

	// Soft-delete the first provider (deleted_at set; the row lingers).
	status, body := env.do(t, http.MethodDelete, "ci_providers/"+id1, "")
	Expect(status).To(Or(Equal(http.StatusNoContent), Equal(http.StatusOK)),
		"delete ci_provider %s failed: %s", id1, body)

	// Re-create reusing the SAME issuer. With a PARTIAL issuer index this succeeds;
	// a FULL index would 409 on the lingering soft-deleted row (FINDING 1).
	id2 := create("cip-second-" + short())
	Expect(id2).NotTo(Equal(id1), "re-create should produce a new row")
}
