package managedClusters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

type registrationServiceStub struct {
	ManagedClusterService
	capabilities []string
	called       bool
}

func (s *registrationServiceStub) Register(_ context.Context, _ string, capabilities []string, _ string) (*ManagedCluster, bool, *errors.ServiceError) {
	s.called = true
	s.capabilities = capabilities
	cluster := &ManagedCluster{}
	cluster.ID = "cluster-id"
	return cluster, true, nil
}

func TestRegisterCapabilitiesJSONSemantics(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		wantStatus       int
		wantCalled       bool
		wantCapabilities []string
	}{
		{
			// Backward compatibility: an older control plane sends a
			// capability-less heartbeat, which is accepted as a no-op rather than
			// rejected, so a rolling upgrade does not disconnect the cluster.
			name:       "omitted is accepted as a capability-less heartbeat",
			body:       `{"name":"cluster-one"}`,
			wantStatus: http.StatusCreated,
			wantCalled: true,
		},
		{
			name:       "empty array is accepted as a capability-less heartbeat",
			body:       `{"name":"cluster-one","capabilities":[]}`,
			wantStatus: http.StatusCreated,
			wantCalled: true,
		},
		{
			name:       "null is accepted as a capability-less heartbeat",
			body:       `{"name":"cluster-one","capabilities":null}`,
			wantStatus: http.StatusCreated,
			wantCalled: true,
		},
		{
			name:             "valid array is accepted",
			body:             `{"name":"cluster-one","capabilities":["provider/aws","network/public"]}`,
			wantStatus:       http.StatusCreated,
			wantCalled:       true,
			wantCapabilities: []string{"network/public", "provider/aws"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &registrationServiceStub{}
			handler := NewManagedClusterHandler(service, nil)
			request := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/managed_clusters/registration", strings.NewReader(tt.body))
			token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "control-plane"})
			request = request.WithContext(context.WithValue(request.Context(), auth.ContextAuthKey, token))
			response := httptest.NewRecorder()

			handler.Register(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if service.called != tt.wantCalled {
				t.Fatalf("service called = %t, want %t", service.called, tt.wantCalled)
			}
			if !reflect.DeepEqual(service.capabilities, tt.wantCapabilities) {
				t.Errorf("capabilities = %v, want %v", service.capabilities, tt.wantCapabilities)
			}
		})
	}
}
