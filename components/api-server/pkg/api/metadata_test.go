package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	trexapi "github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
)

func TestMetadataHandlerReportsBuildIdentity(t *testing.T) {
	previousVersion := trexapi.Version
	previousBuildTime := trexapi.BuildTime
	defer func() {
		trexapi.Version = previousVersion
		trexapi.BuildTime = previousBuildTime
		handlers.SetMetadataID("hypershell")
	}()

	trexapi.Version = "v1.6.0-1234567"
	trexapi.BuildTime = "2026-09-02T15:00:00Z"
	handlers.SetMetadataID("hypershell")

	request := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/metadata", nil)
	response := httptest.NewRecorder()
	handlers.NewMetadataHandler().Get(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("metadata status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("metadata content type = %q, want application/json", got)
	}

	var metadata trexapi.Metadata
	if err := json.Unmarshal(response.Body.Bytes(), &metadata); err != nil {
		t.Fatalf("decode metadata response: %v", err)
	}
	if metadata.ID != "hypershell" {
		t.Errorf("metadata id = %q, want hypershell", metadata.ID)
	}
	if metadata.HREF != "/api/hypershell/v1/metadata" {
		t.Errorf("metadata href = %q", metadata.HREF)
	}
	if metadata.Kind != "API" {
		t.Errorf("metadata kind = %q, want API", metadata.Kind)
	}
	if metadata.Version != "v1.6.0-1234567" {
		t.Errorf("metadata version = %q", metadata.Version)
	}
	if metadata.BuildTime != "2026-09-02T15:00:00Z" {
		t.Errorf("metadata build time = %q", metadata.BuildTime)
	}
}
