package secretSources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/pkg/secretstore"
	"github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// postSecretSource drives the real Create HTTP handler (JSON decode -> action ->
// JSON encode) so these are HTTP-stack tests of the secret-submission contract.
func postSecretSource(t *testing.T, h *secretSourceHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/hypershell/ext/secret_sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	return rec
}

const submissionBody = `{"name":"gateway-oidc","agent_runtime_id":"ar-1","purpose":"gateway-identity","backend":"vault","path":"client-supplied-ignored","value":"topsecret"}`

func createdSample() *SecretSource {
	item := sampleSecretSource()
	item.ID = "ss-1"
	item.Path = deterministicSecretLocation("ar-1", "gateway-oidc")
	return item
}

// Verifies: PLAT-DATA-12.01, PLAT-DATA-12.02, AGT-SEC-04.02
func TestSecretSubmission_WritesStoreFirstThenRecord_NeverReturnsValue(t *testing.T) {
	store := secretstore.NewMemoryStore()
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	rec := postSecretSource(t, h, submissionBody)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	// The value was written to the store at the deterministic location.
	loc := deterministicSecretLocation("ar-1", "gateway-oidc")
	if got, ok := store.Get(loc); !ok || got != "topsecret" {
		t.Fatalf("store[%s] = (%q,%v), want (topsecret,true)", loc, got, ok)
	}
	// The persisted reference points at the derived location, overriding the
	// client-supplied path; the model carries no raw value (it has no such field).
	if svc.got == nil || svc.got.Path != loc {
		t.Fatalf("recorded path = %+v, want %s", svc.got, loc)
	}
	// The response never echoes the value.
	if strings.Contains(rec.Body.String(), "topsecret") || strings.Contains(rec.Body.String(), "\"value\"") {
		t.Fatalf("response leaked the secret value: %s", rec.Body.String())
	}
	// Defense in depth: the submission response is marked no-store.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestSecretSubmission_StoreFailureLeavesNoRecord(t *testing.T) {
	store := secretstore.NewErrorStore(context.DeadlineExceeded)
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	rec := postSecretSource(t, h, submissionBody)

	if rec.Code < 400 {
		t.Fatalf("status = %d, want an error", rec.Code)
	}
	if svc.got != nil {
		t.Fatalf("Create must not run when the store write fails; got %+v", svc.got)
	}
}

func TestSecretSubmission_RetryIsIdempotent(t *testing.T) {
	store := secretstore.NewMemoryStore()
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	if rec := postSecretSource(t, h, submissionBody); rec.Code != http.StatusCreated {
		t.Fatalf("first submit status = %d", rec.Code)
	}
	if rec := postSecretSource(t, h, submissionBody); rec.Code != http.StatusCreated {
		t.Fatalf("retry status = %d", rec.Code)
	}
	// The deterministic location means the retry overwrote the same entry.
	loc := deterministicSecretLocation("ar-1", "gateway-oidc")
	if got, ok := store.Get(loc); !ok || got != "topsecret" {
		t.Fatalf("store[%s] = (%q,%v) after retry", loc, got, ok)
	}
}

// Verifies: PLAT-DATA-12.03, AGT-SEC-04.01
func TestSecretSubmission_ReferenceOnlyWithoutPathDerivesConventionPath(t *testing.T) {
	// A reference-only create that omits both value and path must succeed and
	// persist the derived convention location, so the value can be uploaded later
	// to a known path (agent-secret-late-binding.spec.md "Reference-Only
	// SecretSource Submission"). An unconfigured store proves no write occurs.
	store := secretstore.NewErrorStore(context.DeadlineExceeded)
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	body := `{"name":"gateway-oidc","agent_runtime_id":"ar-1","purpose":"gateway-identity","backend":"vault"}`
	rec := postSecretSource(t, h, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("reference-only create status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	want := deterministicSecretLocation("ar-1", "gateway-oidc")
	if svc.got == nil || svc.got.Path != want {
		t.Fatalf("derived path = %+v, want %s", svc.got, want)
	}
}

func TestSecretSubmission_ReferenceOnlyCreateDoesNotTouchStore(t *testing.T) {
	// An unconfigured store would fail any write; a reference-only create (no
	// value) must still succeed because it never calls the store.
	store := secretstore.NewErrorStore(context.DeadlineExceeded)
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	body := `{"name":"gateway-oidc","agent_runtime_id":"ar-1","purpose":"gateway-identity","backend":"vault","path":"secret/data/manual"}`
	rec := postSecretSource(t, h, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("reference-only create status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	// The client-supplied path is IGNORED: the reference is pinned to the
	// derived convention location. Honoring a client path let any creator
	// point a SecretSource at another tenant's store location, which the
	// control plane would then place into the caller's namespace.
	want := deterministicSecretLocation("ar-1", "gateway-oidc")
	if svc.got == nil || svc.got.Path != want {
		t.Fatalf("reference-only create must derive the path server-side; got %+v, want %s", svc.got, want)
	}
}

// recordingStore records Put calls so a test can assert that no store write
// happened for a rejected submission.
type recordingStore struct {
	puts []string
}

func (s *recordingStore) Put(_ context.Context, location, _ string) error {
	s.puts = append(s.puts, location)
	return nil
}

func TestSecretSubmission_UnauthorizedCallerWritesNothing(t *testing.T) {
	// When the tenancy check rejects the caller, the handler must fail BEFORE
	// the store write and before the record is created: a rejection after the
	// Put would already have overwritten the target location.
	store := &recordingStore{}
	svc := &stubSecretSourceService{ret: createdSample()}
	authorizer := &manageAuthorizer{
		bindings: stubBindingLookup{summaries: []rbac.BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}},
		runtimes: stubRuntimeLookup{runtime: &agentRuntimes.AgentRuntime{GatewayId: "gw-victim"}},
	}
	h := NewSecretSourceHandler(svc, nil, store, authorizer)

	req := httptest.NewRequest(http.MethodPost, "/api/hypershell/ext/secret_sources", strings.NewReader(submissionBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), rbac.ContextUserIDKey, "user-attacker"))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if len(store.puts) != 0 {
		t.Fatalf("rejected submission must not write the store; wrote %v", store.puts)
	}
	if svc.got != nil {
		t.Fatalf("rejected submission must not create a record; got %+v", svc.got)
	}
}

func TestSecretSubmission_AuthorizedOwnerWrites(t *testing.T) {
	store := secretstore.NewMemoryStore()
	svc := &stubSecretSourceService{ret: createdSample()}
	authorizer := &manageAuthorizer{
		bindings: stubBindingLookup{summaries: []rbac.BindingSummary{
			{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-victim")},
		}},
		runtimes: stubRuntimeLookup{runtime: &agentRuntimes.AgentRuntime{GatewayId: "gw-victim"}},
	}
	h := NewSecretSourceHandler(svc, nil, store, authorizer)

	req := httptest.NewRequest(http.MethodPost, "/api/hypershell/ext/secret_sources", strings.NewReader(submissionBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), rbac.ContextUserIDKey, "user-owner"))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	loc := deterministicSecretLocation("ar-1", "gateway-oidc")
	if got, ok := store.Get(loc); !ok || got != "topsecret" {
		t.Fatalf("store[%s] = (%q,%v), want (topsecret,true)", loc, got, ok)
	}
}

func TestSecretSubmission_MissingRuntimeIsRejected(t *testing.T) {
	store := &recordingStore{}
	svc := &stubSecretSourceService{ret: createdSample()}
	authorizer := &manageAuthorizer{
		bindings: stubBindingLookup{summaries: []rbac.BindingSummary{{RoleName: "platform:admin", Scope: "global"}}},
		runtimes: stubRuntimeLookup{svcErr: errors.NotFound("agent runtime not found")},
	}
	h := NewSecretSourceHandler(svc, nil, store, authorizer)

	req := httptest.NewRequest(http.MethodPost, "/api/hypershell/ext/secret_sources", strings.NewReader(submissionBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), rbac.ContextUserIDKey, "user-admin"))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	if len(store.puts) != 0 {
		t.Fatalf("a submission for a nonexistent runtime must not write the store; wrote %v", store.puts)
	}
}

// muxSetVar injects a gorilla/mux route variable onto a request so a handler that
// reads mux.Vars(r) can be driven directly without a full router.
func muxSetVar(r *http.Request, key, value string) *http.Request {
	return mux.SetURLVars(r, map[string]string{key: value})
}

// postMaterialUpload drives the real UploadMaterial HTTP handler against an
// existing SecretSource id.
func postMaterialUpload(t *testing.T, h *secretSourceHandler, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/hypershell/ext/secret_sources/"+id+"/material", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = muxSetVar(req, "id", id)
	rec := httptest.NewRecorder()
	h.UploadMaterial(rec, req)
	return rec
}

// Verifies: PLAT-DATA-27.01, PLAT-DATA-27.02, AGT-SEC-14.01, AGT-SEC-14.02
func TestMaterialUpload_WritesConventionPath_NeverReturnsValue(t *testing.T) {
	store := secretstore.NewMemoryStore()
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	rec := postMaterialUpload(t, h, "ss-1", `{"value":"topsecret"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	loc := deterministicSecretLocation("ar-1", "gateway-oidc")
	if got, ok := store.Get(loc); !ok || got != "topsecret" {
		t.Fatalf("store[%s] = (%q,%v), want (topsecret,true)", loc, got, ok)
	}
	// The upload never created or replaced the DB record; it only wrote the store.
	if svc.got != nil {
		t.Fatalf("material upload must not persist the model; got %+v", svc.got)
	}
	// The response never echoes the material.
	if strings.Contains(rec.Body.String(), "topsecret") || strings.Contains(rec.Body.String(), "\"value\"") {
		t.Fatalf("response leaked the material: %s", rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

// Verifies: PLAT-DATA-27.04, AGT-SEC-14.05
func TestMaterialUpload_UnwiredStoreFailsLoudlyWritingNothing(t *testing.T) {
	// A disabled store (HYPERSHELL_SECRET_STORE_BACKEND unset) must reject the
	// upload loudly rather than silently dropping the material.
	store := secretstore.NewErrorStore(context.DeadlineExceeded)
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	rec := postMaterialUpload(t, h, "ss-1", `{"value":"topsecret"}`)

	if rec.Code < 400 {
		t.Fatalf("status = %d, want an error", rec.Code)
	}
}

func TestMaterialUpload_MissingValueRejected(t *testing.T) {
	store := &recordingStore{}
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	rec := postMaterialUpload(t, h, "ss-1", `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if len(store.puts) != 0 {
		t.Fatalf("a bodyless upload must not write the store; wrote %v", store.puts)
	}
}

// Verifies: PLAT-DATA-27.03, AGT-SEC-14.05
func TestMaterialUpload_UnknownSecretSourceIs404WritingNothing(t *testing.T) {
	store := &recordingStore{}
	svc := &stubSecretSourceService{err: errors.NotFound("secret source not found")}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	rec := postMaterialUpload(t, h, "ss-missing", `{"value":"topsecret"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	if len(store.puts) != 0 {
		t.Fatalf("an upload for a nonexistent source must not write the store; wrote %v", store.puts)
	}
}

// Verifies: PLAT-DATA-27.01, AGT-SEC-14.04
func TestMaterialUpload_ReUploadOverwritesSamePathIdempotently(t *testing.T) {
	store := secretstore.NewMemoryStore()
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store, nil)

	if rec := postMaterialUpload(t, h, "ss-1", `{"value":"first"}`); rec.Code != http.StatusOK {
		t.Fatalf("first upload status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if rec := postMaterialUpload(t, h, "ss-1", `{"value":"second"}`); rec.Code != http.StatusOK {
		t.Fatalf("re-upload status = %d; body=%s", rec.Code, rec.Body.String())
	}
	loc := deterministicSecretLocation("ar-1", "gateway-oidc")
	if got, ok := store.Get(loc); !ok || got != "second" {
		t.Fatalf("store[%s] = (%q,%v) after re-upload, want (second,true)", loc, got, ok)
	}
}

// Authorization runs before any store write, so a rejected upload writes nothing.
// Verifies: AGT-SEC-14.03
func TestMaterialUpload_UnauthorizedCallerDeniedWritesNothing(t *testing.T) {
	store := &recordingStore{}
	svc := &stubSecretSourceService{ret: createdSample()}
	authorizer := &manageAuthorizer{
		bindings: stubBindingLookup{summaries: []rbac.BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}},
		runtimes: stubRuntimeLookup{runtime: &agentRuntimes.AgentRuntime{GatewayId: "gw-victim"}},
	}
	h := NewSecretSourceHandler(svc, nil, store, authorizer)

	req := httptest.NewRequest(http.MethodPost, "/api/hypershell/ext/secret_sources/ss-1/material", strings.NewReader(`{"value":"topsecret"}`))
	req.Header.Set("Content-Type", "application/json")
	req = muxSetVar(req, "id", "ss-1")
	req = req.WithContext(context.WithValue(req.Context(), rbac.ContextUserIDKey, "user-attacker"))
	rec := httptest.NewRecorder()
	h.UploadMaterial(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if len(store.puts) != 0 {
		t.Fatalf("rejected upload must not write the store; wrote %v", store.puts)
	}
}

func TestMaterialUpload_AuthorizedOwnerWrites(t *testing.T) {
	store := secretstore.NewMemoryStore()
	svc := &stubSecretSourceService{ret: createdSample()}
	authorizer := &manageAuthorizer{
		bindings: stubBindingLookup{summaries: []rbac.BindingSummary{
			{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-victim")},
		}},
		runtimes: stubRuntimeLookup{runtime: &agentRuntimes.AgentRuntime{GatewayId: "gw-victim"}},
	}
	h := NewSecretSourceHandler(svc, nil, store, authorizer)

	req := httptest.NewRequest(http.MethodPost, "/api/hypershell/ext/secret_sources/ss-1/material", strings.NewReader(`{"value":"topsecret"}`))
	req.Header.Set("Content-Type", "application/json")
	req = muxSetVar(req, "id", "ss-1")
	req = req.WithContext(context.WithValue(req.Context(), rbac.ContextUserIDKey, "user-owner"))
	rec := httptest.NewRecorder()
	h.UploadMaterial(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	loc := deterministicSecretLocation("ar-1", "gateway-oidc")
	if got, ok := store.Get(loc); !ok || got != "topsecret" {
		t.Fatalf("store[%s] = (%q,%v), want (topsecret,true)", loc, got, ok)
	}
}

// stubBindingLookup satisfies rbac.RoleBindingLookup.
type stubBindingLookup struct {
	summaries []rbac.BindingSummary
	err       error
}

func (s stubBindingLookup) FindBindingsByUserID(context.Context, string) ([]rbac.BindingSummary, error) {
	return s.summaries, s.err
}

// stubRuntimeLookup satisfies the agentRuntimes.AgentRuntimeService surface the
// authorizer uses (Get only).
type stubRuntimeLookup struct {
	runtime *agentRuntimes.AgentRuntime
	svcErr  *errors.ServiceError
}

func (s stubRuntimeLookup) Get(context.Context, string) (*agentRuntimes.AgentRuntime, *errors.ServiceError) {
	return s.runtime, s.svcErr
}

func strPtr(s string) *string { return &s }
