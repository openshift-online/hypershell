package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamic "k8s.io/client-go/dynamic/fake"
)

func TestGatewayNamespace(t *testing.T) {
	t.Run("returns the recorded namespace", func(t *testing.T) {
		gw := &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: "gw-123"},
			Name:      "my-gateway",
			Namespace: "openshell-0011223344556677",
		}
		ns, err := gatewayNamespace(gw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ns != "openshell-0011223344556677" {
			t.Errorf("namespace = %q, want %q", ns, "openshell-0011223344556677")
		}
	})

	t.Run("errors instead of guessing when namespace is empty", func(t *testing.T) {
		// A missing namespace must never be synthesized from the gateway name.
		// The real scheme is openshell-<hex(ksuid)> (set in Gateway.BeforeCreate),
		// so an openshell-<name> guess would target a different namespace; on the
		// delete path that could destroy the wrong (possibly live) namespace.
		// Callers are expected to refuse to act on the returned error.
		gw := &pb.Gateway{
			Metadata: &pb.ObjectReference{Id: "gw-456"},
			Name:     "my-gateway",
		}
		ns, err := gatewayNamespace(gw)
		if err == nil {
			t.Fatalf("expected error for empty namespace, got namespace %q", ns)
		}
		if ns != "" {
			t.Errorf("namespace = %q, want empty string on error", ns)
		}
	})
}

// mockKeycloakAdminServer provides a test Keycloak Admin REST API server with
// dynamic credentials and token validation for testing GatewayReconciler.
type mockKeycloakAdminServer struct {
	server       *httptest.Server
	realm        string
	adminClient  string
	adminSecret  string
	dynamicToken string
	clientUUID   string
	clientID     string

	mu           sync.Mutex
	clientRep    map[string]interface{}
	putCalled    int
	putBodies    []map[string]interface{}
	forceUUIDErr bool
	forcePutErr  bool
}

func newMockKeycloakAdminServer(t *testing.T, clientID, clientUUID string, initialRep map[string]interface{}) *mockKeycloakAdminServer {
	t.Helper()

	m := &mockKeycloakAdminServer{
		realm:        fmt.Sprintf("realm-%s", uuid.New().String()[:8]),
		adminClient:  fmt.Sprintf("admin-%s", uuid.New().String()[:8]),
		adminSecret:  fmt.Sprintf("secret-%s", uuid.New().String()),
		dynamicToken: fmt.Sprintf("tok-%s", uuid.New().String()),
		clientID:     clientID,
		clientUUID:   clientUUID,
		clientRep:    initialRep,
	}

	mux := http.NewServeMux()

	// Token endpoint
	tokenPath := fmt.Sprintf("/realms/%s/protocol/openid-connect/token", m.realm)
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, fmt.Sprintf("bad form: %v", err), http.StatusBadRequest)
			return
		}
		if r.FormValue("client_id") != m.adminClient || r.FormValue("client_secret") != m.adminSecret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": m.dynamicToken,
			"expires_in":   300,
		}); err != nil {
			t.Errorf("mock token encode error: %v", err)
		}
	})

	// Client list / query endpoint
	clientsPath := fmt.Sprintf("/admin/realms/%s/clients", m.realm)
	mux.HandleFunc(clientsPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", m.dynamicToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		m.mu.Lock()
		forceErr := m.forceUUIDErr
		targetClientID := m.clientID
		targetUUID := m.clientUUID
		m.mu.Unlock()

		if forceErr {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		reqClientID := r.URL.Query().Get("clientId")
		w.Header().Set("Content-Type", "application/json")
		if reqClientID == targetClientID && targetUUID != "" {
			if err := json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": targetUUID, "clientId": targetClientID},
			}); err != nil {
				t.Errorf("mock client query encode error: %v", err)
			}
			return
		}

		if err := json.NewEncoder(w).Encode([]map[string]interface{}{}); err != nil {
			t.Errorf("mock empty client query encode error: %v", err)
		}
	})

	// Specific client representation endpoint
	if clientUUID != "" {
		clientPath := fmt.Sprintf("/admin/realms/%s/clients/%s", m.realm, clientUUID)
		mux.HandleFunc(clientPath, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", m.dynamicToken) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			switch r.Method {
			case http.MethodGet:
				m.mu.Lock()
				rep := m.clientRep
				m.mu.Unlock()

				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(rep); err != nil {
					t.Errorf("mock client rep encode error: %v", err)
				}

			case http.MethodPut:
				m.mu.Lock()
				forceErr := m.forcePutErr
				m.mu.Unlock()

				if forceErr {
					http.Error(w, "internal server error on put", http.StatusInternalServerError)
					return
				}

				bodyBytes, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, fmt.Sprintf("read body error: %v", err), http.StatusBadRequest)
					return
				}

				var bodyMap map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &bodyMap); err != nil {
					http.Error(w, fmt.Sprintf("unmarshal body error: %v", err), http.StatusBadRequest)
					return
				}

				m.mu.Lock()
				m.putCalled++
				m.putBodies = append(m.putBodies, bodyMap)
				m.clientRep = bodyMap
				m.mu.Unlock()

				w.WriteHeader(http.StatusNoContent)

			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
	}

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func TestGatewayReconciler_Handle_GatedPhases_ReconcilesKeycloakAndBypassesKubernetes(t *testing.T) {
	ctx := context.Background()
	gatedPhases := []string{"Running", "Provisioning", "Degraded"}

	for _, phase := range gatedPhases {
		t.Run(fmt.Sprintf("phase=%s", phase), func(t *testing.T) {
			gatewayID := fmt.Sprintf("gw-%s", uuid.New().String()[:8])
			gatewayName := "my-gateway"
			clientID := fmt.Sprintf("%s-%s", gatewayName, gatewayID)
			clientUUID := fmt.Sprintf("uuid-%s", uuid.New().String())

			initialRep := map[string]interface{}{
				"id":                        clientUUID,
				"clientId":                  clientID,
				"name":                      "Test Gateway Client",
				"description":               "Gateway client for testing",
				"directAccessGrantsEnabled": true,
				"redirectUris":              []interface{}{"https://gateway.example.com/callback"},
				"attributes": map[string]interface{}{
					"custom_tenant": "tenant-abc",
					"custom_env":    "prod",
				},
			}

			mockKC := newMockKeycloakAdminServer(t, clientID, clientUUID, initialRep)

			kcClient := keycloak.NewClient(
				mockKC.server.URL,
				mockKC.realm,
				mockKC.adminClient,
				mockKC.adminSecret,
			)

			fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())

			r := &GatewayReconciler{
				active:                make(map[string]struct{}),
				dynamicClient:         fakeDynamic,
				clientset:             nil, // nil client proves Kubernetes reprovisioning is completely bypassed without dereferencing
				controlPlaneNamespace: "hypershell-system",
				keycloakClient:        kcClient,
				keycloakConfig: &gateway.KeycloakConfig{
					ServerURL:    mockKC.server.URL,
					Realm:        mockKC.realm,
					ClientID:     mockKC.adminClient,
					ClientSecret: mockKC.adminSecret,
				},
			}

			phaseVal := phase
			event := watcher.Event[*pb.Gateway]{
				Type:       watcher.EventUpdated,
				ResourceID: gatewayID,
				Resource: &pb.Gateway{
					Metadata:  &pb.ObjectReference{Id: gatewayID},
					Name:      gatewayName,
					Namespace: "openshell-0011223344556677",
					Phase:     &phaseVal,
				},
			}

			if err := r.Handle(ctx, event); err != nil {
				t.Fatalf("Handle() returned unexpected error: %v", err)
			}

			// 1. Verify that Keycloak was updated with device authorization grant
			mockKC.mu.Lock()
			putCount := mockKC.putCalled
			var lastPut map[string]interface{}
			if len(mockKC.putBodies) > 0 {
				lastPut = mockKC.putBodies[len(mockKC.putBodies)-1]
			}
			mockKC.mu.Unlock()

			if putCount != 1 {
				t.Fatalf("expected exactly 1 PUT to Keycloak, got %d", putCount)
			}

			attrs, ok := lastPut["attributes"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected attributes map in updated representation, got %v", lastPut["attributes"])
			}
			if attrs["oauth2.device.authorization.grant.enabled"] != "true" {
				t.Errorf("oauth2.device.authorization.grant.enabled = %v, want 'true'", attrs["oauth2.device.authorization.grant.enabled"])
			}

			// 2. Verify unrelated attributes and top-level fields survived intact
			if attrs["custom_tenant"] != "tenant-abc" {
				t.Errorf("custom_tenant attribute = %v, want 'tenant-abc'", attrs["custom_tenant"])
			}
			if attrs["custom_env"] != "prod" {
				t.Errorf("custom_env attribute = %v, want 'prod'", attrs["custom_env"])
			}
			if lastPut["name"] != "Test Gateway Client" {
				t.Errorf("client name = %v, want 'Test Gateway Client'", lastPut["name"])
			}
			if lastPut["description"] != "Gateway client for testing" {
				t.Errorf("client description = %v, want 'Gateway client for testing'", lastPut["description"])
			}
			if lastPut["directAccessGrantsEnabled"] != true {
				t.Errorf("directAccessGrantsEnabled = %v, want true", lastPut["directAccessGrantsEnabled"])
			}
			expectedRedirects := []interface{}{"https://gateway.example.com/callback"}
			if !reflect.DeepEqual(lastPut["redirectUris"], expectedRedirects) {
				t.Errorf("redirectUris = %v, want %v", lastPut["redirectUris"], expectedRedirects)
			}

			// 3. Verify Kubernetes reprovisioning work was completely bypassed
			if dynamicActions := fakeDynamic.Actions(); len(dynamicActions) > 0 {
				t.Errorf("expected 0 dynamic actions on gated phase, got %d: %v", len(dynamicActions), dynamicActions)
			}
		})
	}
}

func TestGatewayReconciler_Handle_GatedPhase_IdempotentWhenAlreadyEnabled(t *testing.T) {
	ctx := context.Background()
	gatewayID := "gw-idempotent"
	gatewayName := "my-gateway"
	clientID := fmt.Sprintf("%s-%s", gatewayName, gatewayID)
	clientUUID := "uuid-idempotent"

	initialRep := map[string]interface{}{
		"id":       clientUUID,
		"clientId": clientID,
		"attributes": map[string]interface{}{
			"oauth2.device.authorization.grant.enabled": "true",
			"custom_attr": "preserved",
		},
	}

	mockKC := newMockKeycloakAdminServer(t, clientID, clientUUID, initialRep)

	kcClient := keycloak.NewClient(
		mockKC.server.URL,
		mockKC.realm,
		mockKC.adminClient,
		mockKC.adminSecret,
	)

	fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())

	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         fakeDynamic,
		clientset:             nil,
		controlPlaneNamespace: "hypershell-system",
		keycloakClient:        kcClient,
	}

	phase := "Running"
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource: &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: gatewayID},
			Name:      gatewayName,
			Namespace: "openshell-0011223344556677",
			Phase:     &phase,
		},
	}

	if err := r.Handle(ctx, event); err != nil {
		t.Fatalf("Handle() returned unexpected error: %v", err)
	}

	mockKC.mu.Lock()
	putCount := mockKC.putCalled
	mockKC.mu.Unlock()

	if putCount != 0 {
		t.Errorf("expected 0 PUT calls when already enabled, got %d", putCount)
	}
}

func TestGatewayReconciler_Handle_GatedPhase_KeycloakUnconfigured(t *testing.T) {
	ctx := context.Background()
	fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())

	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         fakeDynamic,
		clientset:             nil,
		controlPlaneNamespace: "hypershell-system",
		keycloakClient:        nil, // unconfigured Keycloak
	}

	phase := "Running"
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: "gw-no-keycloak",
		Resource: &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: "gw-no-keycloak"},
			Name:      "my-gateway",
			Namespace: "openshell-0011223344556677",
			Phase:     &phase,
		},
	}

	if err := r.Handle(ctx, event); err != nil {
		t.Fatalf("Handle() should succeed when Keycloak is unconfigured, got: %v", err)
	}

	if len(fakeDynamic.Actions()) > 0 {
		t.Errorf("expected 0 Kubernetes actions when Keycloak is unconfigured on gated gateway")
	}
}

func TestGatewayReconciler_Handle_GatedPhase_MissingClientLeavesToProvisioningPath(t *testing.T) {
	ctx := context.Background()
	gatewayID := "gw-missing"
	gatewayName := "my-gateway"
	clientID := fmt.Sprintf("%s-%s", gatewayName, gatewayID)

	// clientUUID is empty -> GetClientUUID returns ""
	mockKC := newMockKeycloakAdminServer(t, clientID, "", nil)

	kcClient := keycloak.NewClient(
		mockKC.server.URL,
		mockKC.realm,
		mockKC.adminClient,
		mockKC.adminSecret,
	)

	fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())

	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         fakeDynamic,
		clientset:             nil,
		controlPlaneNamespace: "hypershell-system",
		keycloakClient:        kcClient,
	}

	phase := "Running"
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource: &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: gatewayID},
			Name:      gatewayName,
			Namespace: "openshell-0011223344556677",
			Phase:     &phase,
		},
	}

	// Missing client on gated gateway logs warning and returns nil without error,
	// avoiding creating a duplicate client out of band.
	if err := r.Handle(ctx, event); err != nil {
		t.Fatalf("Handle() returned unexpected error for missing client: %v", err)
	}

	mockKC.mu.Lock()
	putCount := mockKC.putCalled
	mockKC.mu.Unlock()

	if putCount != 0 {
		t.Errorf("expected 0 PUT calls for missing client, got %d", putCount)
	}

	if len(fakeDynamic.Actions()) > 0 {
		t.Errorf("expected 0 Kubernetes actions on gated gateway with missing Keycloak client")
	}
}

func TestGatewayReconciler_Handle_GatedPhase_PropagatesKeycloakLookupError(t *testing.T) {
	ctx := context.Background()
	gatewayID := "gw-err-lookup"
	gatewayName := "my-gateway"
	clientID := fmt.Sprintf("%s-%s", gatewayName, gatewayID)
	clientUUID := "uuid-lookup-err"

	mockKC := newMockKeycloakAdminServer(t, clientID, clientUUID, map[string]interface{}{
		"id": clientUUID,
	})
	mockKC.mu.Lock()
	mockKC.forceUUIDErr = true
	mockKC.mu.Unlock()

	kcClient := keycloak.NewClient(
		mockKC.server.URL,
		mockKC.realm,
		mockKC.adminClient,
		mockKC.adminSecret,
	)

	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         fakedynamic.NewSimpleDynamicClient(runtime.NewScheme()),
		clientset:             nil,
		controlPlaneNamespace: "hypershell-system",
		keycloakClient:        kcClient,
	}

	phase := "Running"
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource: &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: gatewayID},
			Name:      gatewayName,
			Namespace: "openshell-0011223344556677",
			Phase:     &phase,
		},
	}

	err := r.Handle(ctx, event)
	if err == nil {
		t.Fatalf("expected error from Handle() when Keycloak lookup fails, got nil")
	}
	if !strings.Contains(err.Error(), "reconcile Keycloak client for gateway") {
		t.Errorf("error = %q, want wrapping with 'reconcile Keycloak client for gateway'", err.Error())
	}
}

func TestGatewayReconciler_Handle_GatedPhase_PropagatesKeycloakUpdateError(t *testing.T) {
	ctx := context.Background()
	gatewayID := "gw-err-update"
	gatewayName := "my-gateway"
	clientID := fmt.Sprintf("%s-%s", gatewayName, gatewayID)
	clientUUID := "uuid-update-err"

	mockKC := newMockKeycloakAdminServer(t, clientID, clientUUID, map[string]interface{}{
		"id":       clientUUID,
		"clientId": clientID,
		"attributes": map[string]interface{}{
			"oauth2.device.authorization.grant.enabled": "false",
		},
	})
	mockKC.mu.Lock()
	mockKC.forcePutErr = true
	mockKC.mu.Unlock()

	kcClient := keycloak.NewClient(
		mockKC.server.URL,
		mockKC.realm,
		mockKC.adminClient,
		mockKC.adminSecret,
	)

	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         fakedynamic.NewSimpleDynamicClient(runtime.NewScheme()),
		clientset:             nil,
		controlPlaneNamespace: "hypershell-system",
		keycloakClient:        kcClient,
	}

	phase := "Degraded"
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource: &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: gatewayID},
			Name:      gatewayName,
			Namespace: "openshell-0011223344556677",
			Phase:     &phase,
		},
	}

	err := r.Handle(ctx, event)
	if err == nil {
		t.Fatalf("expected error from Handle() when Keycloak PUT update fails, got nil")
	}
	if !strings.Contains(err.Error(), "reconcile Keycloak client for gateway") {
		t.Errorf("error = %q, want wrapping with 'reconcile Keycloak client for gateway'", err.Error())
	}
}
