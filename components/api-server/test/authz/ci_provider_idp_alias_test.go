package authz_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestCIProviderIdpAliasCRUD exercises the explicit idp_alias field (PLAT-DATA-24,
// Bug B of #1012) end to end over the real HTTP and database stack: create with
// idp_alias set, GET round-trips it, and a PATCH that changes only idp_alias
// persists. The create -> get path fails if the new column's migration did not run
// or the presenter/model does not carry the field; the integration harness runs the
// same lib/pq + PreferSimpleProtocol session production uses (SDLC-CONF-04), so a
// broken ALTER TABLE surfaces here as a 500 / SQLSTATE 25P02.
func TestCIProviderIdpAliasCRUD(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	// Create with an explicit idp_alias (the terraform ci_forges map key). A unique
	// name avoids the one-live-provider-per-forge name index (SDLC-CONF-13).
	req := openapi.NewCIProvider("github", "https://issuer.example/"+short(), "https://example.com/jwks", "hypershell")
	req.Name = openapi.PtrString("cip-alias-" + short())
	req.IdpAlias = openapi.PtrString("github-actions")
	created, _, err := client.DefaultAPI.CreateCIProvider(env.owner.rest).CIProvider(*req).Execute()
	Expect(err).NotTo(HaveOccurred(), "create ci provider with idp_alias: %s", test.APIErrorReason(err))
	Expect(created.GetIdpAlias()).To(Equal("github-actions"), "create response should echo the explicit idp_alias")

	id := created.GetId()
	Expect(id).NotTo(BeEmpty())
	// The integration DB is shared across this package (sync.Once), so remove the
	// providers this test creates, otherwise they persist as live rows into later
	// tests that re-backfill and de-duplicate the shared ci_providers table.
	t.Cleanup(func() {
		_, _ = client.DefaultAPI.DeleteCIProvider(env.owner.rest, id).Execute()
	})

	// GET round-trips the stored idp_alias (verifies the column persists).
	got, _, err := client.DefaultAPI.GetCIProvider(env.owner.rest, id).Execute()
	Expect(err).NotTo(HaveOccurred(), "get ci provider: %s", test.APIErrorReason(err))
	Expect(got.GetIdpAlias()).To(Equal("github-actions"), "stored idp_alias should round-trip on GET")

	// PATCH changing only idp_alias persists the new value.
	patch := openapi.NewCIProviderPatchRequest()
	patch.IdpAlias = openapi.PtrString("github-actions-v2")
	updated, _, err := client.DefaultAPI.UpdateCIProvider(env.owner.rest, id).CIProviderPatchRequest(*patch).Execute()
	Expect(err).NotTo(HaveOccurred(), "patch ci provider idp_alias: %s", test.APIErrorReason(err))
	Expect(updated.GetIdpAlias()).To(Equal("github-actions-v2"), "PATCH should update idp_alias")

	// And the change is durable on a fresh GET.
	reread, _, err := client.DefaultAPI.GetCIProvider(env.owner.rest, id).Execute()
	Expect(err).NotTo(HaveOccurred(), "re-get ci provider: %s", test.APIErrorReason(err))
	Expect(reread.GetIdpAlias()).To(Equal("github-actions-v2"), "patched idp_alias should be durable")

	// A provider created without idp_alias reports it empty (the reconciler then
	// falls back to the derived alias).
	bare := openapi.NewCIProvider("gitlab", "https://issuer.example/"+short(), "https://example.com/jwks", "hypershell")
	bare.Name = openapi.PtrString("cip-bare-" + short())
	createdBare, _, err := client.DefaultAPI.CreateCIProvider(env.owner.rest).CIProvider(*bare).Execute()
	Expect(err).NotTo(HaveOccurred(), "create ci provider without idp_alias: %s", test.APIErrorReason(err))
	Expect(createdBare.GetIdpAlias()).To(BeEmpty(), "unset idp_alias should be empty")
	t.Cleanup(func() {
		_, _ = client.DefaultAPI.DeleteCIProvider(env.owner.rest, createdBare.GetId()).Execute()
	})
}
