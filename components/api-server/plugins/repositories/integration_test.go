package repositories_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"

	// Register the sibling ext plugins' routes + migrations in this test binary:
	// the scenario creates a SecretSource (FK target) and AgentRuntimes, and the
	// Repository delete-guard queries the agent_runtimes table.
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/secretSources"
)

// TestRepositoryCreateAndSelectorContract exercises the Repository ext kind and
// the AgentRuntime repository selector through the full HTTP server stack
// (the /api/hypershell/ext prefixed-router path), which the service-level unit
// tests do not cover.
// Verifies: PLAT-DATA-10.01, PLAT-DATA-10.02, PLAT-DATA-10.03, PLAT-DATA-09.02
func TestRepositoryCreateAndSelectorContract(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// A SecretSource is the Repository.secret_source_id FK target.
	// agent_runtime_id is required by the SecretSource contract; the path is
	// derived server-side (a client-supplied path is ignored).
	ss, resp, err := client.DefaultAPI.CreateSecretSource(ctx).SecretSource(openapi.SecretSource{
		Name:           "gh-cred",
		AgentRuntimeId: "ar-repositories-test",
		Purpose:        "source_control",
		Backend:        "kubernetes",
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "create secret source: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	// Create a Repository (github provider, existing secret source) -> 201.
	repo, resp, err := client.DefaultAPI.CreateRepository(ctx).Repository(openapi.Repository{
		Name:           "hypershell",
		Url:            "https://github.com/openshift-online/hypershell",
		Provider:       "github",
		SecretSourceId: ss.Id,
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "create repository: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*repo.ImportPath).To(Equal(".hypershell"), "import_path should default to .hypershell")

	// A public repository with no secret_source_id -> 201 (anonymous read).
	pubRepo, resp, err := client.DefaultAPI.CreateRepository(ctx).Repository(openapi.Repository{
		Name:     "public-repo",
		Url:      "https://github.com/openshift-online/public",
		Provider: "github",
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "create public repository: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(pubRepo.SecretSourceId).To(BeNil(), "public repository should carry no secret_source_id")

	// A set-but-nonexistent secret_source_id -> 400.
	_, resp, err = client.DefaultAPI.CreateRepository(ctx).Repository(openapi.Repository{
		Name: "missing-ss", Url: "https://x", Provider: "github", SecretSourceId: openapi.PtrString("does-not-exist"),
	}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	// Unsupported provider -> 400.
	_, resp, err = client.DefaultAPI.CreateRepository(ctx).Repository(openapi.Repository{
		Name: "bad", Url: "https://x", Provider: "gitlab", SecretSourceId: ss.Id,
	}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	// AgentRuntime selector WITHOUT repository_id -> 422.
	_, resp, err = client.DefaultAPI.CreateAgentRuntime(ctx).AgentRuntime(openapi.AgentRuntime{
		Name:      "rev-norepo",
		GatewayId: "g1",
		Selector:  []string{"agent/reviewable"},
	}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusUnprocessableEntity))

	// AgentRuntime WITH repository_id + selector -> 201.
	_, resp, err = client.DefaultAPI.CreateAgentRuntime(ctx).AgentRuntime(openapi.AgentRuntime{
		Name:         "reviewer",
		GatewayId:    "g1",
		RepositoryId: repo.Id,
		Selector:     []string{"agent/reviewable", "agent/ready"},
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "create agent runtime with repository: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	// Deleting a Repository still referenced by an AgentRuntime -> 409.
	resp, err = client.DefaultAPI.DeleteRepository(ctx, *repo.Id).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusConflict))
}
