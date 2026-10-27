package authz_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// These tests cover the /api/hypershell/ext RBAC gap: the Extension API kinds
// are served on a router the /v1 RBAC middleware never wrapped, so any
// authenticated caller could write every ext kind (overwriting another
// tenant's secret at its deterministic location, pinning a SecretSource at any
// store path, planting AgentRuntimes on other tenants' gateways). The ext
// router now carries the same middleware (plugins/rbac registers on the "ext"
// prefix too) and the gateway-scoped ext kinds enforce per-gateway tenancy in
// their handlers.

// secretSourcePath mirrors the api-server's deterministic secret location so
// the tests can assert the server-derived path.
func secretSourcePath(agentRuntimeID, name string) string {
	return fmt.Sprintf("hypershell/secret-sources/%s/%s", agentRuntimeID, name)
}

func createAgentRuntime(t *testing.T, client *openapi.APIClient, caller principal, gatewayID string) *openapi.AgentRuntime {
	t.Helper()
	created, httpResp, err := client.DefaultAPI.CreateAgentRuntime(caller.rest).
		AgentRuntime(*openapi.NewAgentRuntime("ar-"+strings.ToLower(api.NewID())[:8], gatewayID)).
		Execute()
	Expect(err).NotTo(HaveOccurred(), "agent runtime create: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))
	return created
}

func submitSecretValue(t *testing.T, client *openapi.APIClient, caller principal, agentRuntimeID, name, value string) (*openapi.SecretSource, *http.Response, error) {
	t.Helper()
	req := openapi.NewSecretSource(name, agentRuntimeID, "github-app", "vault")
	req.Value = openapi.PtrString(value)
	return client.DefaultAPI.CreateSecretSource(caller.rest).SecretSource(*req).Execute()
}

// Verifies: SEC-RBAC-19.01, SEC-RBAC-19.02, SEC-RBAC-19.04
func TestExtSecretSourcesAreGatewayScoped(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), nil)
	attacker := newPrincipal(t, h, "attacker-"+strings.ToLower(api.NewID()), nil)
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))
	gw := createGateway(t, client, owner, spoke.clusterID)

	// The owner declares an AgentRuntime on their gateway and submits material.
	ar := createAgentRuntime(t, client, owner, *gw.Id)
	victim, httpResp, err := submitSecretValue(t, client, owner, ar.GetId(), "github-app", "VICTIM-MATERIAL")
	Expect(err).NotTo(HaveOccurred(), "owner submission failed: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(victim.GetPath()).To(Equal(secretSourcePath(ar.GetId(), "github-app")))

	// Cross-tenant overwrite: an unbound creator submitting a value for the
	// victim's agent runtime and name must be refused BEFORE any store write.
	_, httpResp, err = submitSecretValue(t, client, attacker, ar.GetId(), "github-app", "ATTACKER-OVERWRITE")
	Expect(err).To(HaveOccurred(), "an unbound creator overwrote another tenant's secret location")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	// A platform:admin is owner-equivalent and may manage.
	_, httpResp, err = submitSecretValue(t, client, admin, ar.GetId(), "github-app", "ADMIN-MATERIAL")
	Expect(err).NotTo(HaveOccurred(), "platform:admin submission refused: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))

	// Cross-tenant reference hijack: a reference-only create that supplies the
	// victim's path must not be honored; the path is always derived server-side.
	refReq := openapi.NewSecretSource("attacker-source", ar.GetId(), "github-app", "vault")
	refReq.Path = openapi.PtrString("hypershell/secret-sources/someone-else/their-secret")
	exfil, httpResp, err := client.DefaultAPI.CreateSecretSource(attacker.rest).SecretSource(*refReq).Execute()
	Expect(err).To(HaveOccurred(), "an unbound creator created a reference on another tenant's runtime")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))
	_ = exfil

	// Even the owner's reference-only create ignores a client-supplied path.
	ownerRef := openapi.NewSecretSource("owner-ref", ar.GetId(), "github-app", "vault")
	ownerRef.Path = openapi.PtrString("secret/data/manual")
	created, httpResp, err := client.DefaultAPI.CreateSecretSource(owner.rest).SecretSource(*ownerRef).Execute()
	Expect(err).NotTo(HaveOccurred(), "owner reference-only create refused: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(created.GetPath()).To(Equal(secretSourcePath(ar.GetId(), "owner-ref")), "a client-supplied path must be ignored")

	// Cross-tenant mutation of the victim's record is refused.
	patch := openapi.NewSecretSourcePatchRequestWithDefaults()
	patch.Purpose = openapi.PtrString("hijacked")
	_, httpResp, err = client.DefaultAPI.UpdateSecretSource(attacker.rest, victim.GetId()).SecretSourcePatchRequest(*patch).Execute()
	Expect(err).To(HaveOccurred(), "an unbound creator patched another tenant's secret source")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	httpResp, err = client.DefaultAPI.DeleteSecretSource(attacker.rest, victim.GetId()).Execute()
	Expect(err).To(HaveOccurred(), "an unbound creator deleted another tenant's secret source")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	// The owner may still patch and delete their own record.
	_, httpResp, err = client.DefaultAPI.UpdateSecretSource(owner.rest, victim.GetId()).SecretSourcePatchRequest(*patch).Execute()
	Expect(err).NotTo(HaveOccurred(), "owner patch refused: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	httpResp, err = client.DefaultAPI.DeleteSecretSource(owner.rest, victim.GetId()).Execute()
	Expect(err).NotTo(HaveOccurred(), "owner delete refused: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
}

// Verifies: SEC-RBAC-19.01
func TestExtSecretSourceUnknownRuntimeRejected(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), nil)

	// A submission naming an AgentRuntime that does not exist is rejected and
	// writes nothing (no planting references on nonexistent runtimes).
	_, httpResp, err := submitSecretValue(t, client, user, "ar-does-not-exist", "github-app", "MATERIAL")
	Expect(err).To(HaveOccurred(), "a submission for a nonexistent agent runtime succeeded")
	Expect(statusOf(httpResp)).To(Equal(http.StatusNotFound))
}

// Verifies: SEC-RBAC-19.03, SEC-RBAC-19.04
func TestExtAgentRuntimesAreGatewayScoped(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), nil)
	attacker := newPrincipal(t, h, "attacker-"+strings.ToLower(api.NewID()), nil)
	gw := createGateway(t, client, owner, spoke.clusterID)

	// An unbound creator cannot plant an AgentRuntime on another tenant's
	// gateway (the control plane would reconcile junk into that gateway).
	_, httpResp, err := client.DefaultAPI.CreateAgentRuntime(attacker.rest).
		AgentRuntime(*openapi.NewAgentRuntime("evil-runtime", *gw.Id)).
		Execute()
	Expect(err).To(HaveOccurred(), "an unbound creator planted an agent runtime on another tenant's gateway")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	ar := createAgentRuntime(t, client, owner, *gw.Id)

	patch := openapi.NewAgentRuntimePatchRequestWithDefaults()
	patch.Description = openapi.PtrString("hijacked")
	_, httpResp, err = client.DefaultAPI.UpdateAgentRuntime(attacker.rest, ar.GetId()).AgentRuntimePatchRequest(*patch).Execute()
	Expect(err).To(HaveOccurred(), "an unbound creator patched another tenant's agent runtime")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	httpResp, err = client.DefaultAPI.DeleteAgentRuntime(attacker.rest, ar.GetId()).Execute()
	Expect(err).To(HaveOccurred(), "an unbound creator deleted another tenant's agent runtime")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	// The owner keeps full management of their own runtime.
	_, httpResp, err = client.DefaultAPI.UpdateAgentRuntime(owner.rest, ar.GetId()).AgentRuntimePatchRequest(*patch).Execute()
	Expect(err).NotTo(HaveOccurred(), "owner patch refused: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	httpResp, err = client.DefaultAPI.DeleteAgentRuntime(owner.rest, ar.GetId()).Execute()
	Expect(err).NotTo(HaveOccurred(), "owner delete refused: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
}

func TestExtRequestsProvisionUsersAndServeCreatorReads(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	user := newPrincipal(t, h, "extreader-"+strings.ToLower(api.NewID()), nil)

	// A creator (the authz suite runs with the gateway:creator default, as the
	// deployed hubs do) may read the ext kinds...
	_, httpResp, err := client.DefaultAPI.ListSecretSources(user.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), "creator list secret sources refused: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	_, httpResp, err = client.DefaultAPI.ListRepositories(user.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), "creator list repositories refused: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))

	// ...and the ext request itself provisioned the caller: the /ext router
	// carries the user-provisioning middleware, not just authentication.
	u, svcErr := users.Service(&environments.Environment().Services).GetByUsername(context.Background(), user.name)
	Expect(svcErr).NotTo(HaveOccurred(), "the ext request must provision the caller's User record")
	Expect(u).NotTo(BeNil())
}
