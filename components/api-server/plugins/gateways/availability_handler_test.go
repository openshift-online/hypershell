package gateways

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

func TestGetPlacementAvailabilityAcceptsBatchPost(t *testing.T) {
	handler := gatewayHandler{
		availability: func(_ context.Context, selectors [][]string) (openapi.GatewayPlacementAvailability, *errors.ServiceError) {
			if len(selectors) != 1 {
				t.Fatalf("selectors = %v", selectors)
			}
			return openapi.GatewayPlacementAvailability{}, nil
		},
	}

	request := httptest.NewRequest(http.MethodPost, "/gateways/placement-availability", strings.NewReader(`{"selectors":[["provider/aws","network/public"]]}`))
	response := httptest.NewRecorder()
	handler.GetPlacementAvailability(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"placements"`) {
		t.Fatalf("response body = %s", response.Body.String())
	}
}
