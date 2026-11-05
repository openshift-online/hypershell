package ciTrusts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// A human REST PATCH of subject_repository / subject_project_path must persist, matching
// the fields the gRPC import path can update (FINDING 4). Before the fix
// CITrustPatchRequest omitted both fields, so a PATCH that set them was silently decoded
// away and the stored value never changed.
func TestRESTPatch_PersistsSubjectRepositoryAndProjectPath(t *testing.T) {
	existing := &CITrust{
		Name:               "ct",
		CIProviderID:       "cp-1",
		GatewayID:          "gw-1",
		Role:               "openshell-user",
		SubjectRepository:  strPtr("acme/old"),
		SubjectProjectPath: strPtr("grp/old"),
	}
	existing.ID = "ct-1"
	svc := &captureCITrustService{getResult: existing}
	// Nil authorizer: RBAC enforcement is off in this unit test, so the tenancy gate
	// allows and the patch binding/apply path is exercised directly.
	h := NewCITrustHandler(svc, nil, nil)

	body := `{"subject_repository":"acme/new","subject_project_path":"grp/new"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/hypershell/v1/ci_trusts/ct-1", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "ct-1"})
	rr := httptest.NewRecorder()

	h.Patch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if svc.replaced == nil {
		t.Fatal("no Replace was issued by the PATCH")
	}
	if got := svc.replaced.SubjectRepository; got == nil || *got != "acme/new" {
		t.Errorf("subject_repository = %v, want acme/new persisted", got)
	}
	if got := svc.replaced.SubjectProjectPath; got == nil || *got != "grp/new" {
		t.Errorf("subject_project_path = %v, want grp/new persisted", got)
	}
}
