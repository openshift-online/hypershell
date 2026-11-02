package authz_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestCITrustPatchRecomputesDerivedName is the PATCH-recompute conformance for a
// DERIVED-name kind (SDLC-CONF-13, FINDING 1). CITrust.name defaults to
// DeriveName(ci_provider_id, gateway_id, role). A PATCH that mutates a DeriveName
// input (role) must re-derive name so find-by-name resolves the NEW identity and
// the OLD derived name no longer resolves; otherwise hsctl apply, which resolves
// by name, would keep hitting a stale record after an identity change.
func TestCITrustPatchRecomputesDerivedName(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	const oldRole = "developer"
	const newRole = "maintainer"

	// Create with NO explicit name so the server derives it from the identity.
	req := openapi.NewCITrust(env.ciProviderID, env.gatewayID,
		openapi.CITrustAllOfSubject{Repository: openapi.PtrString("org/repo")}, oldRole)
	ct, _, err := client.DefaultAPI.CreateCITrust(env.owner.rest).CITrust(*req).Execute()
	Expect(err).NotTo(HaveOccurred(), "create ci_trust: %s", test.APIErrorReason(err))
	id := ct.GetId()

	oldName := env.ciProviderID + "/" + env.gatewayID + "/" + oldRole
	newName := env.ciProviderID + "/" + env.gatewayID + "/" + newRole

	// The derived name resolves the record before the patch.
	assertFindByName(t, env, "ci_trusts", id, oldName)

	// PATCH the role (a DeriveName input).
	status, body := env.do(t, http.MethodPatch, "ci_trusts/"+id, `{"role":"`+newRole+`"}`)
	Expect(status).To(Equal(http.StatusOK), "patch ci_trust role failed: %s", body)

	// find-by-name now resolves the NEW derived identity.
	assertFindByName(t, env, "ci_trusts", id, newName)

	// The OLD derived name no longer resolves any record.
	assertNotResolvableByName(t, env, "ci_trusts", id, oldName)
}

// assertNotResolvableByName asserts a find-by-name search for name returns a
// clean 200 whose body does NOT contain id: the record is no longer resolvable
// under that name. It must never be a 500 / SQLSTATE 25P02.
func assertNotResolvableByName(t *testing.T, env *conformanceEnv, kind, id, name string) {
	t.Helper()
	quoted := "'" + strings.ReplaceAll(name, "'", "''") + "'"
	q := kind + "?search=" + url.QueryEscape("name = "+quoted)
	status, body := env.do(t, http.MethodGet, q, "")
	Expect(status).To(Equal(http.StatusOK),
		"find-by-name list for %s should be a clean 200, got %d: %s", kind, status, body)
	Expect(body).NotTo(ContainSubstring(id),
		"find-by-name for %s with the old name %q should no longer resolve record %s: %s", kind, name, id, body)
}

// TestCoreKindsSearchUnknownColumn is the core-kind half of the unknown-search
// -column guard (issue #871, FINDING 2/6). The guard is centralized in the shared
// generic-list wrapper, so it must protect CORE kinds (gateways, managed_clusters)
// that share the same list path, not only the extension kinds. A search naming an
// unknown column SHALL be a clean 400 that names the bad column, never a
// transaction-poisoning 500 / SQLSTATE 25P02.
func TestCoreKindsSearchUnknownColumn(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	// Reuse the conformance env so a spoke cluster and gateway exist; the owner
	// token authorizes the core list endpoints.
	env := newConformanceEnv(t, h, client)

	for _, kind := range []string{"managed_clusters", "gateways"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			RegisterTestingT(t)
			q := kind + "?search=" + url.QueryEscape("definitely_not_a_column = 'x'")
			status, body := coreDo(t, env, http.MethodGet, q)
			Expect(status).To(Equal(http.StatusBadRequest),
				"unknown search column for core kind %s should be a clean 400, not %d: %s", kind, status, body)
			Expect(body).To(ContainSubstring("definitely_not_a_column"),
				"the 400 for core kind %s should name the invalid column: %s", kind, body)
			Expect(body).NotTo(ContainSubstring("25P02"),
				"the unknown column for core kind %s must never reach the database (no 25P02): %s", kind, body)
		})
	}
}

// coreDo issues an authenticated HTTP request against the core v1 surface
// (/api/hypershell/v1/), as opposed to conformanceEnv.do which targets the ext
// surface.
func coreDo(t *testing.T, env *conformanceEnv, method, pathSuffix string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, "http://localhost:8000/api/hypershell/v1/"+pathSuffix, nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+env.owner.token)
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}
