package authz_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestExtKindsCRUDConformance is the generic CRUD conformance test
// (SDLC-CONF-06). It enumerates every registered Extension API kind from the
// RBAC registration source (rbac.ExtKinds, SDLC-CONF-01) and drives a full
// create -> get -> list -> delete round-trip for each over the real HTTP and
// database stack, on the integration-testing harness. That harness connects
// GORM through lib/pq with PreferSimpleProtocol, the same simple query protocol
// production uses (production itself runs the pgx driver,
// gorm.io/driver/postgres), so a migration or query that aborts under the
// production protocol aborts here too (SDLC-CONF-04): the generic LIST and GET
// below would return 500 with SQLSTATE 25P02 rather than 200, failing the test.
//
// Kinds are discovered from the registry, never a hand-kept list: a newly
// registered ext kind is included automatically and fails this test until it
// has a fixture in crudFixtures, which is itself a conformance requirement.
func TestExtKindsCRUDConformance(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	kinds := rbac.ExtKinds()
	Expect(kinds).NotTo(BeEmpty(), "no ext kinds were enumerated from the RBAC registration source")

	for _, kind := range kinds {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			RegisterTestingT(t)
			fixture, ok := crudFixtures[kind]
			Expect(ok).To(BeTrue(),
				"ext kind %q has no CRUD conformance fixture; every registered ext kind must have one (SDLC-CONF-06)", kind)

			id, name := fixture(t, env)
			Expect(id).NotTo(BeEmpty(), "fixture for %q returned no id", kind)
			Expect(name).NotTo(BeEmpty(), "fixture for %q returned no name; find-by-name conformance requires one (SDLC-CONF-13)", kind)

			// GET /{id}: the created resource is retrievable over the real stack.
			status, _ := env.do(t, http.MethodGet, kind+"/"+id, "")
			Expect(status).To(Equal(http.StatusOK), "GET %s/%s round-trip failed (migration or query break?)", kind, id)

			// LIST: the collection query succeeds. A broken migration surfaces
			// here as 500 (SQLSTATE 25P02), not 200.
			status, body := env.do(t, http.MethodGet, kind, "")
			Expect(status).To(Equal(http.StatusOK), "LIST %s failed: %s", kind, body)

			// FIND-BY-NAME (SDLC-CONF-13): the search path builds a different
			// query than the unfiltered list. A Kind that lacks a queryable name
			// column aborts the request transaction here and returns 500 /
			// SQLSTATE 25P02 instead of the created record. This is the exact
			// resolution step every hsctl apply performs.
			assertFindByName(t, env, kind, id, name)

			// DELETE /{id}: removal succeeds over the real stack.
			status, body = env.do(t, http.MethodDelete, kind+"/"+id, "")
			Expect(status).To(Or(Equal(http.StatusOK), Equal(http.StatusNoContent)), "DELETE %s/%s failed: %s", kind, id, body)

			// GET /{id} after delete: the resource is gone.
			status, _ = env.do(t, http.MethodGet, kind+"/"+id, "")
			Expect(status).To(Equal(http.StatusNotFound), "GET %s/%s after delete should be 404", kind, id)
		})
	}
}

// conformanceEnv holds the owner principal and the shared foreign-key targets
// that the per-kind fixtures reference.
type conformanceEnv struct {
	h      *test.Helper
	client *openapi.APIClient
	owner  principal

	clusterID         string
	gatewayID         string
	sandboxTemplateID string
	agentRuntimeID    string
	workspaceID       string
	secretSourceID    string
	providerSpecID    string
	providerBindingID string
	ciProviderID      string
}

func newConformanceEnv(t *testing.T, h *test.Helper, client *openapi.APIClient) *conformanceEnv {
	t.Helper()
	// platform:admin is owner-equivalent for the gateway-scoped kinds and
	// authorizes ci_providers; gateway:creator authorizes gateway creation.
	owner := newPrincipal(t, h, "conformance-"+strings.ToLower(api.NewID()), realmRoles("platform:admin", "gateway:creator"))
	spoke := registerSpoke(t, h, client)
	gw := createGateway(t, client, owner, spoke.clusterID)

	env := &conformanceEnv{
		h:         h,
		client:    client,
		owner:     owner,
		clusterID: spoke.clusterID,
		gatewayID: *gw.Id,
	}

	// Shared FK targets, built in dependency order.
	st, _, err := client.DefaultAPI.CreateSandboxTemplate(owner.rest).
		SandboxTemplate(*openapi.NewSandboxTemplate("st-"+short(), "quay.io/openshift-online/sandbox:latest")).Execute()
	Expect(err).NotTo(HaveOccurred(), "setup sandbox template: %s", test.APIErrorReason(err))
	env.sandboxTemplateID = st.GetId()

	ar := createAgentRuntime(t, client, owner, env.gatewayID)
	env.agentRuntimeID = ar.GetId()

	ws, _, err := client.DefaultAPI.CreateAgentWorkspace(owner.rest).
		AgentWorkspace(*openapi.NewAgentWorkspace(env.agentRuntimeID, env.gatewayID, "ws-"+short())).Execute()
	Expect(err).NotTo(HaveOccurred(), "setup agent workspace: %s", test.APIErrorReason(err))
	env.workspaceID = ws.GetId()

	ss, _, err := client.DefaultAPI.CreateSecretSource(owner.rest).
		SecretSource(*openapi.NewSecretSource("ss-"+short(), env.agentRuntimeID, "source_control", "kubernetes")).Execute()
	Expect(err).NotTo(HaveOccurred(), "setup secret source: %s", test.APIErrorReason(err))
	env.secretSourceID = ss.GetId()

	ps, _, err := client.DefaultAPI.CreateProviderSpec(owner.rest).
		ProviderSpec(*openapi.NewProviderSpec("ps-"+short(), "inference")).Execute()
	Expect(err).NotTo(HaveOccurred(), "setup provider spec: %s", test.APIErrorReason(err))
	env.providerSpecID = ps.GetId()

	pbReq := openapi.NewProviderBinding("pb-"+short(), env.workspaceID, env.providerSpecID)
	pbReq.SecretSourceId = openapi.PtrString(env.secretSourceID)
	pb, _, err := client.DefaultAPI.CreateProviderBinding(owner.rest).ProviderBinding(*pbReq).Execute()
	Expect(err).NotTo(HaveOccurred(), "setup provider binding: %s", test.APIErrorReason(err))
	env.providerBindingID = pb.GetId()

	cp, _, err := client.DefaultAPI.CreateCIProvider(owner.rest).
		CIProvider(*openapi.NewCIProvider("github", "https://token.actions.githubusercontent.com/"+short(), "https://example.com/jwks", "hypershell")).Execute()
	Expect(err).NotTo(HaveOccurred(), "setup ci provider: %s", test.APIErrorReason(err))
	env.ciProviderID = cp.GetId()

	return env
}

// do issues a raw authenticated HTTP request against the ext surface and
// returns the status code and response body.
func (e *conformanceEnv) do(t *testing.T, method, pathSuffix, body string) (int, string) {
	t.Helper()
	url := "http://localhost:8000/api/hypershell/ext/" + pathSuffix
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+e.owner.token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func short() string { return strings.ToLower(api.NewID())[:10] }

// assertFindByName runs the find-by-name list search (SDLC-CONF-13) for a Kind
// and asserts it returns 200 with the created record, never 500 / SQLSTATE
// 25P02. It mirrors the query hsctl apply issues: search=name = '<name>'.
func assertFindByName(t *testing.T, env *conformanceEnv, kind, id, name string) {
	t.Helper()
	quoted := "'" + strings.ReplaceAll(name, "'", "''") + "'"
	q := kind + "?search=" + url.QueryEscape("name = "+quoted)
	status, body := env.do(t, http.MethodGet, q, "")
	Expect(status).To(Equal(http.StatusOK),
		"find-by-name list for %s aborted (migration/search break? 25P02?): %s", kind, body)
	Expect(body).To(ContainSubstring(id),
		"find-by-name list for %s with name %q did not return the created record %s: %s", kind, name, id, body)
}

// TestExtKindsSearchUnknownColumn is scenario SDLC-CONF-13.06: a search filter
// that names an unknown or invalid column SHALL be a clean 400 that names the bad
// column, validated before the clause reaches the database, never a
// transaction-poisoning 500 / SQLSTATE 25P02. The validation is defensive depth
// in the shared ext list search path, independent of any one Kind's columns, so
// it is asserted for every registered ext Kind.
func TestExtKindsSearchUnknownColumn(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	for _, kind := range rbac.ExtKinds() {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			RegisterTestingT(t)
			q := kind + "?search=" + url.QueryEscape("definitely_not_a_column = 'x'")
			status, body := env.do(t, http.MethodGet, q, "")
			Expect(status).To(Equal(http.StatusBadRequest),
				"unknown search column for %s should be a clean 400, not %d: %s", kind, status, body)
			Expect(body).To(ContainSubstring("definitely_not_a_column"),
				"the 400 for %s should name the invalid column: %s", kind, body)
			Expect(body).NotTo(ContainSubstring("25P02"),
				"the unknown column for %s must never reach the database (no 25P02): %s", kind, body)
		})
	}
}

// crudFixtures creates a fresh instance of each ext kind, referencing the
// shared FK targets, and returns its id and its find-by-name key (name). The
// generic round-trip then exercises GET, LIST, find-by-name, and DELETE. Every
// registered ext kind MUST appear here. The four Kinds that previously lacked a
// queryable name column (ci_providers, ci_trusts, inference_routes,
// workspace_memberships) set an explicit name so the find-by-name step resolves
// a known key (SDLC-CONF-13).
var crudFixtures = map[string]func(t *testing.T, e *conformanceEnv) (string, string){
	"ci_providers": func(t *testing.T, e *conformanceEnv) (string, string) {
		name := "cip-" + short()
		req := openapi.NewCIProvider("github", "https://issuer.example/"+short(), "https://example.com/jwks", "hypershell")
		req.Name = openapi.PtrString(name)
		cp, _, err := e.client.DefaultAPI.CreateCIProvider(e.owner.rest).CIProvider(*req).Execute()
		Expect(err).NotTo(HaveOccurred(), "create ci_provider: %s", test.APIErrorReason(err))
		return cp.GetId(), name
	},
	"ci_trusts": func(t *testing.T, e *conformanceEnv) (string, string) {
		name := "cit-" + short()
		req := openapi.NewCITrust(e.ciProviderID, e.gatewayID, openapi.CITrustAllOfSubject{Repository: openapi.PtrString("org/repo")}, "developer")
		req.Name = openapi.PtrString(name)
		ct, _, err := e.client.DefaultAPI.CreateCITrust(e.owner.rest).CITrust(*req).Execute()
		Expect(err).NotTo(HaveOccurred(), "create ci_trust: %s", test.APIErrorReason(err))
		return ct.GetId(), name
	},
	"agent_runtimes": func(t *testing.T, e *conformanceEnv) (string, string) {
		ar := createAgentRuntime(t, e.client, e.owner, e.gatewayID)
		return ar.GetId(), ar.GetName()
	},
	"agent_workspaces": func(t *testing.T, e *conformanceEnv) (string, string) {
		// One AgentWorkspace per AgentRuntime, so create a fresh runtime.
		ar := createAgentRuntime(t, e.client, e.owner, e.gatewayID)
		name := "ws-" + short()
		ws, _, err := e.client.DefaultAPI.CreateAgentWorkspace(e.owner.rest).
			AgentWorkspace(*openapi.NewAgentWorkspace(ar.GetId(), e.gatewayID, name)).Execute()
		Expect(err).NotTo(HaveOccurred(), "create agent_workspace: %s", test.APIErrorReason(err))
		return ws.GetId(), name
	},
	"secret_sources": func(t *testing.T, e *conformanceEnv) (string, string) {
		name := "ss-" + short()
		ss, _, err := e.client.DefaultAPI.CreateSecretSource(e.owner.rest).
			SecretSource(*openapi.NewSecretSource(name, e.agentRuntimeID, "source_control", "kubernetes")).Execute()
		Expect(err).NotTo(HaveOccurred(), "create secret_source: %s", test.APIErrorReason(err))
		return ss.GetId(), name
	},
	"provider_specs": func(t *testing.T, e *conformanceEnv) (string, string) {
		name := "ps-" + short()
		ps, _, err := e.client.DefaultAPI.CreateProviderSpec(e.owner.rest).
			ProviderSpec(*openapi.NewProviderSpec(name, "inference")).Execute()
		Expect(err).NotTo(HaveOccurred(), "create provider_spec: %s", test.APIErrorReason(err))
		return ps.GetId(), name
	},
	"provider_bindings": func(t *testing.T, e *conformanceEnv) (string, string) {
		name := "pb-" + short()
		req := openapi.NewProviderBinding(name, e.workspaceID, e.providerSpecID)
		req.SecretSourceId = openapi.PtrString(e.secretSourceID)
		pb, _, err := e.client.DefaultAPI.CreateProviderBinding(e.owner.rest).ProviderBinding(*req).Execute()
		Expect(err).NotTo(HaveOccurred(), "create provider_binding: %s", test.APIErrorReason(err))
		return pb.GetId(), name
	},
	"inference_routes": func(t *testing.T, e *conformanceEnv) (string, string) {
		name := "ir-" + short()
		req := openapi.NewInferenceRoute(e.workspaceID, e.providerBindingID, "gpt-4o")
		req.Name = openapi.PtrString(name)
		ir, _, err := e.client.DefaultAPI.CreateInferenceRoute(e.owner.rest).InferenceRoute(*req).Execute()
		Expect(err).NotTo(HaveOccurred(), "create inference_route: %s", test.APIErrorReason(err))
		return ir.GetId(), name
	},
	"workspace_memberships": func(t *testing.T, e *conformanceEnv) (string, string) {
		name := "wm-" + short()
		req := openapi.NewWorkspaceMembership(e.workspaceID, "subject-"+short(), "member")
		req.Name = openapi.PtrString(name)
		wm, _, err := e.client.DefaultAPI.CreateWorkspaceMembership(e.owner.rest).WorkspaceMembership(*req).Execute()
		Expect(err).NotTo(HaveOccurred(), "create workspace_membership: %s", test.APIErrorReason(err))
		return wm.GetId(), name
	},
	"repositories": func(t *testing.T, e *conformanceEnv) (string, string) {
		// Public repository: no secret_source_id required.
		name := "repo-" + short()
		repo, _, err := e.client.DefaultAPI.CreateRepository(e.owner.rest).
			Repository(*openapi.NewRepository(name, "https://github.com/openshift-online/"+short(), "github")).Execute()
		Expect(err).NotTo(HaveOccurred(), "create repository: %s", test.APIErrorReason(err))
		return repo.GetId(), name
	},
	"sandbox_templates": func(t *testing.T, e *conformanceEnv) (string, string) {
		name := "st-" + short()
		st, _, err := e.client.DefaultAPI.CreateSandboxTemplate(e.owner.rest).
			SandboxTemplate(*openapi.NewSandboxTemplate(name, "quay.io/openshift-online/sandbox:latest")).Execute()
		Expect(err).NotTo(HaveOccurred(), "create sandbox_template: %s", test.APIErrorReason(err))
		return st.GetId(), name
	},
}
