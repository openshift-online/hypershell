package secretSources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/secretstore"
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

func TestSecretSubmission_WritesStoreFirstThenRecord_NeverReturnsValue(t *testing.T) {
	store := secretstore.NewMemoryStore()
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store)

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
	h := NewSecretSourceHandler(svc, nil, store)

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
	h := NewSecretSourceHandler(svc, nil, store)

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

func TestSecretSubmission_ReferenceOnlyWithoutPathDerivesConventionPath(t *testing.T) {
	// A reference-only create that omits both value and path must succeed and
	// persist the derived convention location, so the value can be uploaded later
	// to a known path (agent-secret-late-binding.spec.md "Reference-Only
	// SecretSource Submission"). An unconfigured store proves no write occurs.
	store := secretstore.NewErrorStore(context.DeadlineExceeded)
	svc := &stubSecretSourceService{ret: createdSample()}
	h := NewSecretSourceHandler(svc, nil, store)

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
	h := NewSecretSourceHandler(svc, nil, store)

	body := `{"name":"gateway-oidc","agent_runtime_id":"ar-1","purpose":"gateway-identity","backend":"vault","path":"secret/data/manual"}`
	rec := postSecretSource(t, h, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("reference-only create status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if svc.got == nil || svc.got.Path != "secret/data/manual" {
		t.Fatalf("reference-only create must keep the client path; got %+v", svc.got)
	}
}
