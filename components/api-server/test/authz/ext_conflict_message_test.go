package authz_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestCIProviderConflictNamesActualField is the conflict-attribution regression
// for FINDING 2. CIProvider has two unique indexes (name and issuer). The prior
// create path reported "a CIProvider with issuer %q already exists" for EVERY
// unique violation, so a duplicate NAME was attributed to the wrong field. This
// test provokes each conflict independently and asserts the 409 names the field
// that actually collided.
func TestCIProviderConflictNamesActualField(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	// Baseline provider with an explicit name and issuer.
	dupName := "cip-conflict-" + short()
	dupIssuer := "https://issuer.example/conflict-" + short()
	base := openapi.NewCIProvider("github", dupIssuer, "https://example.com/jwks", "hypershell")
	base.Name = openapi.PtrString(dupName)
	_, _, err := client.DefaultAPI.CreateCIProvider(env.owner.rest).CIProvider(*base).Execute()
	Expect(err).NotTo(HaveOccurred(), "create baseline ci_provider: %s", test.APIErrorReason(err))

	// Duplicate NAME, distinct issuer -> the conflict is the name, so the message
	// must name the name (not the issuer).
	dupNameReq := openapi.NewCIProvider("github", "https://issuer.example/other-"+short(), "https://example.com/jwks", "hypershell")
	dupNameReq.Name = openapi.PtrString(dupName)
	_, resp, err := client.DefaultAPI.CreateCIProvider(env.owner.rest).CIProvider(*dupNameReq).Execute()
	Expect(err).To(HaveOccurred(), "duplicate-name create should conflict")
	Expect(resp.StatusCode).To(Equal(http.StatusConflict), "duplicate-name create should be 409")
	Expect(test.APIErrorReason(err)).To(And(
		ContainSubstring("named"),
		ContainSubstring(dupName),
	), "duplicate-name 409 must name the name field, got: %s", test.APIErrorReason(err))

	// Duplicate ISSUER, distinct name -> the conflict is the issuer, so the message
	// must still name the issuer (the fix must not misattribute this one to name).
	dupIssuerReq := openapi.NewCIProvider("github", dupIssuer, "https://example.com/jwks", "hypershell")
	dupIssuerReq.Name = openapi.PtrString("cip-conflict-" + short())
	_, resp, err = client.DefaultAPI.CreateCIProvider(env.owner.rest).CIProvider(*dupIssuerReq).Execute()
	Expect(err).To(HaveOccurred(), "duplicate-issuer create should conflict")
	Expect(resp.StatusCode).To(Equal(http.StatusConflict), "duplicate-issuer create should be 409")
	Expect(test.APIErrorReason(err)).To(And(
		ContainSubstring("issuer"),
		ContainSubstring(dupIssuer),
	), "duplicate-issuer 409 must name the issuer field, got: %s", test.APIErrorReason(err))
}
