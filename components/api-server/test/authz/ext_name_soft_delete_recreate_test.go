package authz_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestExtKindsDeleteThenRecreateSameName is the soft-delete convergence regression
// for SDLC-CONF-13 (FINDING 1). The name unique index must be PARTIAL
// (WHERE deleted_at IS NULL): GORM soft-delete filters deleted_at IS NULL on reads,
// so after a resource is deleted a reconcile's find-by-name sees nothing and
// re-issues CREATE, which derives the SAME name. A FULL unique index would still
// carry the soft-deleted row's name and reject that INSERT with a 409, breaking
// convergence (for example re-applying a CITrust). A partial index constrains only
// live rows, so the re-create succeeds and find-by-name resolves the NEW live row.
//
// For every Kind that gained a derived name key this test creates a row with a
// DERIVED name (no explicit name), soft-deletes it, then re-creates the SAME
// identity and asserts the re-create succeeds and find-by-name resolves the new
// row - never the deleted one. Without the partial index the re-create is a 409 and
// this test fails.
func TestExtKindsDeleteThenRecreateSameName(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	// Stable per-kind identities so both creates derive the SAME name.
	const ciTrustRole = "developer"
	const irModel = "gpt-4o-recreate"
	wmSubject := "subject-" + short()

	cases := []struct {
		kind     string
		wantName string
		// create returns the id of a freshly created row with the derived name.
		create func() string
	}{
		{
			kind: "ci_providers",
			// Name derives from forge; "gitlab" is distinct from the env's "github"
			// provider so the only name collision is with this test's own rows. The
			// issuer is uniquely indexed independently, so each create uses a fresh
			// issuer; only the name (forge) repeats.
			wantName: "gitlab",
			create: func() string {
				req := openapi.NewCIProvider("gitlab", "https://issuer.example/recreate-"+short(), "https://example.com/jwks", "hypershell")
				cp, _, err := client.DefaultAPI.CreateCIProvider(env.owner.rest).CIProvider(*req).Execute()
				Expect(err).NotTo(HaveOccurred(), "create ci_provider: %s", test.APIErrorReason(err))
				return cp.GetId()
			},
		},
		{
			kind:     "ci_trusts",
			wantName: env.ciProviderID + "/" + env.gatewayID + "/" + ciTrustRole,
			create: func() string {
				req := openapi.NewCITrust(env.ciProviderID, env.gatewayID,
					openapi.CITrustAllOfSubject{Repository: openapi.PtrString("org/repo")}, ciTrustRole)
				ct, _, err := client.DefaultAPI.CreateCITrust(env.owner.rest).CITrust(*req).Execute()
				Expect(err).NotTo(HaveOccurred(), "create ci_trust: %s", test.APIErrorReason(err))
				return ct.GetId()
			},
		},
		{
			kind:     "inference_routes",
			wantName: env.workspaceID + "/" + env.providerBindingID + "/" + irModel,
			create: func() string {
				req := openapi.NewInferenceRoute(env.workspaceID, env.providerBindingID, irModel)
				ir, _, err := client.DefaultAPI.CreateInferenceRoute(env.owner.rest).InferenceRoute(*req).Execute()
				Expect(err).NotTo(HaveOccurred(), "create inference_route: %s", test.APIErrorReason(err))
				return ir.GetId()
			},
		},
		{
			kind:     "workspace_memberships",
			wantName: env.workspaceID + "/" + wmSubject,
			create: func() string {
				req := openapi.NewWorkspaceMembership(env.workspaceID, wmSubject, "member")
				wm, _, err := client.DefaultAPI.CreateWorkspaceMembership(env.owner.rest).WorkspaceMembership(*req).Execute()
				Expect(err).NotTo(HaveOccurred(), "create workspace_membership: %s", test.APIErrorReason(err))
				return wm.GetId()
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.kind, func(t *testing.T) {
			RegisterTestingT(t)

			id1 := tc.create()
			assertFindByName(t, env, tc.kind, id1, tc.wantName)

			// Soft-delete the first row (deleted_at set; the row lingers).
			status, body := env.do(t, http.MethodDelete, tc.kind+"/"+id1, "")
			Expect(status).To(Or(Equal(http.StatusNoContent), Equal(http.StatusOK)),
				"delete %s %s failed: %s", tc.kind, id1, body)

			// Re-create the SAME identity. With a PARTIAL index this succeeds; a FULL
			// index would 409 on the lingering soft-deleted row (FINDING 1).
			id2 := tc.create()
			Expect(id2).NotTo(Equal(id1), "re-create should produce a new row")

			// find-by-name resolves the NEW live row, and no longer the deleted one.
			assertFindByName(t, env, tc.kind, id2, tc.wantName)
			assertNotResolvableByName(t, env, tc.kind, id1, tc.wantName)
		})
	}
}
