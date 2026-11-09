package managedClusters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v4"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

type registrationServiceStub struct {
	ManagedClusterService
	capabilities []string
	health       *HealthVerdict
	called       bool
}

func (s *registrationServiceStub) Register(_ context.Context, _ string, capabilities []string, _ string, health *HealthVerdict) (*ManagedCluster, bool, *errors.ServiceError) {
	s.called = true
	s.capabilities = capabilities
	s.health = health
	cluster := &ManagedCluster{}
	cluster.ID = "cluster-id"
	return cluster, true, nil
}

// Verifies: PLAT-CLUS-07
func TestRegisterCapabilitiesJSONSemantics(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		wantStatus       int
		wantCalled       bool
		wantCapabilities []string
	}{
		{
			name:       "omitted is rejected",
			body:       `{"name":"cluster-one"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty array is rejected",
			body:       `{"name":"cluster-one","capabilities":[]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "null is rejected",
			body:       `{"name":"cluster-one","capabilities":null}`,
			wantStatus: http.StatusBadRequest,
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

// The optional self-check verdict on the heartbeat (PLAT-CLUS-16): a present,
// well-formed verdict is passed through to the service; an absent or null verdict
// is a nil no-op; a malformed verdict (bad enum, wrong type, or missing required
// field) is rejected with 400 before the service is called, so the stored verdict
// and last_seen_at are untouched (PLAT-CLUS-16.03).
func TestRegisterHealthVerdictJSONSemantics(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantStatus    int
		wantCalled    bool
		wantHealthNil bool
		wantOverall   string
	}{
		{
			name:          "omitted health is a nil no-op",
			body:          `{"name":"cluster-one","capabilities":["provider/aws","network/public"]}`,
			wantStatus:    http.StatusCreated,
			wantCalled:    true,
			wantHealthNil: true,
		},
		{
			name:          "null health is a nil no-op",
			body:          `{"name":"cluster-one","capabilities":["provider/aws","network/public"],"health":null}`,
			wantStatus:    http.StatusCreated,
			wantCalled:    true,
			wantHealthNil: true,
		},
		{
			name:        "well-formed verdict is passed through",
			body:        `{"name":"cluster-one","capabilities":["provider/aws","network/public"],"health":{"overall":"failing","observed_at":"2026-11-09T12:00:00Z","duration_ms":842,"conditions":[{"name":"oidc-issuer-reachable","status":"fail","message":"503"}]}}`,
			wantStatus:  http.StatusCreated,
			wantCalled:  true,
			wantOverall: "failing",
		},
		{
			name:       "bad overall enum is rejected with 400",
			body:       `{"name":"cluster-one","capabilities":["provider/aws","network/public"],"health":{"overall":"bogus","conditions":[]}}`,
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "bad condition status enum is rejected with 400",
			body:       `{"name":"cluster-one","capabilities":["provider/aws","network/public"],"health":{"overall":"healthy","conditions":[{"name":"images-pullable","status":"ok"}]}}`,
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "wrong type for overall is rejected with 400",
			body:       `{"name":"cluster-one","capabilities":["provider/aws","network/public"],"health":{"overall":123,"conditions":[]}}`,
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "missing required overall is rejected with 400",
			body:       `{"name":"cluster-one","capabilities":["provider/aws","network/public"],"health":{"conditions":[]}}`,
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
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
			if !tt.wantCalled {
				return
			}
			if tt.wantHealthNil {
				if service.health != nil {
					t.Fatalf("health = %+v, want nil", service.health)
				}
				return
			}
			if service.health == nil {
				t.Fatalf("health = nil, want a verdict with overall %q", tt.wantOverall)
			}
			if service.health.Overall != tt.wantOverall {
				t.Fatalf("health.Overall = %q, want %q", service.health.Overall, tt.wantOverall)
			}
		})
	}
}
