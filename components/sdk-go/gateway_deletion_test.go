package sdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openshift-online/hypershell/components/sdk-go/client"
	"github.com/openshift-online/hypershell/components/sdk-go/types"
)

func TestGatewayDeletionStatusPreservesExactReference(t *testing.T) {
	reference := "control/project?x=one&generation=two+three"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hypershell/v1/gateways/deletion" || r.URL.Query().Get("external_reference") != reference || len(r.URL.Query()) != 1 {
			t.Errorf("unexpected deletion request: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("request has no caller credential")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(types.GatewayDeletionStatus{GatewayID: "gateway-id", ExternalReference: reference, State: "requested"}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	sdk, err := client.NewClient(server.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.Gateways().GetDeletionStatus(context.Background(), reference)
	if err != nil || result.GatewayID != "gateway-id" || result.State != "requested" {
		t.Fatalf("status=%v error=%v", result, err)
	}
}
