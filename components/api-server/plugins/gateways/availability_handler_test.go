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

func TestGetPlacementAvailabilityAcceptsEmptyGetBody(t *testing.T) {
	handler := gatewayHandler{
		availability: func(context.Context) (openapi.GatewayPlacementAvailability, *errors.ServiceError) {
			return openapi.GatewayPlacementAvailability{LocalKind: true}, nil
		},
	}

	request := httptest.NewRequest(http.MethodGet, "/gateways/placement-availability", nil)
	response := httptest.NewRecorder()
	handler.GetPlacementAvailability(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"local_kind":true`) {
		t.Fatalf("response body = %s", response.Body.String())
	}
}
