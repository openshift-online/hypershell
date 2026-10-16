package sandboxTemplates_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestSandboxTemplateRepositoryOwnershipIsReadOnly exercises the nullable
// repository_id ownership mark through the full HTTP server stack. The mark is
// set only by the control-plane declarative-import reconciler, so a direct REST
// create SHALL leave it null, and a client-supplied repository_id SHALL be
// ignored (read-only in the client contract). This also proves the ownership
// migration applied cleanly on server boot.
func TestSandboxTemplateRepositoryOwnershipIsReadOnly(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Direct create: no ownership -> repository_id is null.
	st, resp, err := client.DefaultAPI.CreateSandboxTemplate(ctx).SandboxTemplate(openapi.SandboxTemplate{
		Name:  "direct-created",
		Image: "quay.io/example/sandbox:latest",
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "create sandbox template: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(st.RepositoryId).To(BeNil(), "directly-created resource must not be owned by a Repository")

	// A client-supplied repository_id is read-only and SHALL be ignored.
	owned := "some-repository-id"
	st2, resp, err := client.DefaultAPI.CreateSandboxTemplate(ctx).SandboxTemplate(openapi.SandboxTemplate{
		Name:         "client-cannot-own",
		Image:        "quay.io/example/sandbox:latest",
		RepositoryId: &owned,
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "create sandbox template with client repository_id: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(st2.RepositoryId).To(BeNil(), "client-supplied repository_id must be ignored (read-only)")

	// Re-read confirms the persisted row carries no ownership mark.
	got, resp, err := client.DefaultAPI.GetSandboxTemplate(ctx, *st2.Id).Execute()
	Expect(err).NotTo(HaveOccurred(), "get sandbox template: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(got.RepositoryId).To(BeNil())
}
