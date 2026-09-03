package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

func TestExistingGatewayKeycloakClientID(t *testing.T) {
	gatewayID := "gateway-01ABCDEF"
	oidcJSON := func(clientID, audience string) *string {
		t.Helper()
		data, err := json.Marshal(gateway.OIDCConfig{ClientID: clientID, Audience: audience})
		if err != nil {
			t.Fatalf("marshal OIDC config: %v", err)
		}
		value := string(data)
		return &value
	}
	malformedOIDC := "{"

	tests := []struct {
		name    string
		gateway *pb.Gateway
		want    string
		wantErr string
	}{
		{
			name:    "current identity falls back to current name",
			gateway: &pb.Gateway{Name: "current-name"},
			want:    "current-name-" + gatewayID,
		},
		{
			name:    "current persisted identity",
			gateway: &pb.Gateway{Name: "current-name", Oidc: oidcJSON("current-name-"+gatewayID, "current-name-"+gatewayID)},
			want:    "current-name-" + gatewayID,
		},
		{
			name:    "legacy ID-only audience",
			gateway: &pb.Gateway{Name: "renamed", Oidc: oidcJSON("", gatewayID)},
			want:    gatewayID,
		},
		{
			name:    "renamed gateway uses persisted identity",
			gateway: &pb.Gateway{Name: "new-name", Oidc: oidcJSON("original-name-"+gatewayID, "original-name-"+gatewayID)},
			want:    "original-name-" + gatewayID,
		},
		{
			name:    "historical raw name characters remain valid",
			gateway: &pb.Gateway{Name: "new-name", Oidc: oidcJSON("Original gateway_日本語-"+gatewayID, "Original gateway_日本語-"+gatewayID)},
			want:    "Original gateway_日本語-" + gatewayID,
		},
		{
			name:    "historical line separator remains valid",
			gateway: &pb.Gateway{Name: "new-name", Oidc: oidcJSON("Original\u2028gateway-"+gatewayID, "Original\u2028gateway-"+gatewayID)},
			want:    "Original\u2028gateway-" + gatewayID,
		},
		{
			name:    "historical paragraph separator remains valid",
			gateway: &pb.Gateway{Name: "new-name", Oidc: oidcJSON("Original\u2029gateway-"+gatewayID, "Original\u2029gateway-"+gatewayID)},
			want:    "Original\u2029gateway-" + gatewayID,
		},
		{
			name:    "historical bidi formatting control remains valid",
			gateway: &pb.Gateway{Name: "new-name", Oidc: oidcJSON("Original\u202egateway-"+gatewayID, "Original\u202egateway-"+gatewayID)},
			want:    "Original\u202egateway-" + gatewayID,
		},
		{
			name:    "malformed persisted OIDC config",
			gateway: &pb.Gateway{Name: "current-name", Oidc: &malformedOIDC},
			wantErr: "parse persisted OIDC config",
		},
		{
			name:    "foreign persisted identity",
			gateway: &pb.Gateway{Name: "current-name", Oidc: oidcJSON("foreign-gateway-OTHER", "foreign-gateway-OTHER")},
			wantErr: "not owned by the gateway",
		},
		{
			name:    "mismatched persisted identities",
			gateway: &pb.Gateway{Name: "current-name", Oidc: oidcJSON("one-"+gatewayID, "two-"+gatewayID)},
			wantErr: "do not match",
		},
		{
			name:    "persisted carriage return",
			gateway: &pb.Gateway{Name: "current-name", Oidc: oidcJSON("bad\r-"+gatewayID, "")},
			wantErr: "control characters",
		},
		{
			name:    "persisted newline",
			gateway: &pb.Gateway{Name: "current-name", Oidc: oidcJSON("bad\n-"+gatewayID, "")},
			wantErr: "control characters",
		},
		{
			name:    "persisted non-line control character",
			gateway: &pb.Gateway{Name: "current-name", Oidc: oidcJSON("bad\t-"+gatewayID, "")},
			wantErr: "control characters",
		},
		{
			name:    "fallback name with control character",
			gateway: &pb.Gateway{Name: "bad\nname"},
			wantErr: "control characters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := existingGatewayKeycloakClientID(gatewayID, tc.gateway)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("existingGatewayKeycloakClientID() error = %v, want containing %q", err, tc.wantErr)
				}
				var identityErr *gatewayKeycloakClientIdentityError
				if !errors.As(err, &identityErr) {
					t.Fatalf("existingGatewayKeycloakClientID() error type = %T, want terminal identity validation", err)
				}
				if got != "" {
					t.Fatalf("existingGatewayKeycloakClientID() = %q on error, want empty", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("existingGatewayKeycloakClientID() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("existingGatewayKeycloakClientID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReconcileExistingGatewayKeycloakClient_QuotesAcceptedUnicodeClientIDs(t *testing.T) {
	gatewayID := "gateway-01ABCDEF"
	for _, tc := range []struct {
		name      string
		separator string
	}{
		{name: "line separator", separator: "\u2028"},
		{name: "paragraph separator", separator: "\u2029"},
		{name: "bidi override", separator: "\u202e"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientID := "Historical" + tc.separator + "name-" + gatewayID
			data, err := json.Marshal(gateway.OIDCConfig{ClientID: clientID, Audience: clientID})
			if err != nil {
				t.Fatalf("marshal OIDC config: %v", err)
			}
			oidc := string(data)
			gw := &pb.Gateway{Name: "renamed", Oidc: &oidc}

			got, err := existingGatewayKeycloakClientID(gatewayID, gw)
			if err != nil || got != clientID {
				t.Fatalf("existingGatewayKeycloakClientID() = %q, %v; want accepted %q", got, err, clientID)
			}

			mockKC := newMockKeycloakAdminServer(t, clientID, "", nil)
			r := &GatewayReconciler{keycloakClient: keycloak.NewClient(
				mockKC.server.URL,
				mockKC.realm,
				mockKC.adminClient,
				mockKC.adminSecret,
			)}
			err = r.reconcileExistingGatewayKeycloakClient(t.Context(), gatewayID, gw)
			if err == nil {
				t.Fatal("reconcileExistingGatewayKeycloakClient() error = nil, want missing-client error")
			}
			if strings.Contains(err.Error(), tc.separator) {
				t.Fatalf("error contains raw %s and can inject log structure: %q", tc.name, err)
			}
			if quoted := fmt.Sprintf("%q", clientID); !strings.Contains(err.Error(), quoted) {
				t.Fatalf("error = %q, want escaped client ID %s", err, quoted)
			}
		})
	}
}

func TestReconcileExistingGatewayKeycloakClient_RejectsUntrustedIdentityBeforeKeycloakCall(t *testing.T) {
	gatewayID := "gateway-01ABCDEF"
	tests := []struct {
		name     string
		clientID string
		audience string
	}{
		{name: "foreign", clientID: "foreign-OTHER", audience: "foreign-OTHER"},
		{name: "mismatched", clientID: "one-" + gatewayID, audience: "two-" + gatewayID},
		{name: "control character", clientID: "bad\n-" + gatewayID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var called atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called.Store(true)
				http.Error(w, "unexpected Keycloak call", http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)

			data, err := json.Marshal(gateway.OIDCConfig{ClientID: tc.clientID, Audience: tc.audience})
			if err != nil {
				t.Fatalf("marshal OIDC config: %v", err)
			}
			oidc := string(data)
			r := &GatewayReconciler{
				keycloakClient: keycloak.NewClient(server.URL, "realm", "admin", "secret"),
			}
			err = r.reconcileExistingGatewayKeycloakClient(t.Context(), gatewayID, &pb.Gateway{
				Name: "current-name",
				Oidc: &oidc,
			})
			if err == nil {
				t.Fatal("reconcileExistingGatewayKeycloakClient() error = nil, want identity rejection")
			}
			if called.Load() {
				t.Fatal("untrusted identity reached the Keycloak API")
			}
		})
	}
}

type recordedGatewayUpdate struct {
	id        string
	phase     string
	status    string
	hasPhase  bool
	hasStatus bool
}

type recordingGatewayServer struct {
	pb.UnimplementedGatewayServiceServer

	mu        sync.Mutex
	gateway   *pb.Gateway
	updates   []recordedGatewayUpdate
	updateErr error
}

func (s *recordingGatewayServer) ListGateways(_ context.Context, _ *pb.ListGatewaysRequest) (*pb.ListGatewaysResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gateway == nil {
		return &pb.ListGatewaysResponse{Metadata: &pb.ListMeta{}}, nil
	}
	return &pb.ListGatewaysResponse{
		Items:    []*pb.Gateway{s.gateway},
		Metadata: &pb.ListMeta{Page: 1, Size: 500, Total: 1},
	}, nil
}

func (s *recordingGatewayServer) WatchGateways(_ *pb.WatchGatewaysRequest, stream pb.GatewayService_WatchGatewaysServer) error {
	if err := stream.SendHeader(metadata.Pairs("x-hypershell-test-watch", "ready")); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

func (s *recordingGatewayServer) setGateway(gw *pb.Gateway) {
	s.mu.Lock()
	s.gateway = gw
	s.mu.Unlock()
}

func (s *recordingGatewayServer) UpdateGateway(_ context.Context, req *pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
	update := recordedGatewayUpdate{id: req.GetId()}
	if req.Phase != nil {
		update.phase = req.GetPhase()
		update.hasPhase = true
	}
	if req.Status != nil {
		update.status = req.GetStatus()
		update.hasStatus = true
	}
	s.mu.Lock()
	s.updates = append(s.updates, update)
	updateErr := s.updateErr
	s.mu.Unlock()
	if updateErr != nil {
		return nil, updateErr
	}
	return &pb.UpdateGatewayResponse{}, nil
}

func (s *recordingGatewayServer) setUpdateError(err error) {
	s.mu.Lock()
	s.updateErr = err
	s.mu.Unlock()
}

func (s *recordingGatewayServer) snapshot() []recordedGatewayUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedGatewayUpdate(nil), s.updates...)
}

func newRecordingGatewayConn(t *testing.T) (*grpc.ClientConn, *recordingGatewayServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	recorder := &recordingGatewayServer{}
	pb.RegisterGatewayServiceServer(server, recorder)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial recording gateway server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, recorder
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

	mu                    sync.Mutex
	clientRep             map[string]interface{}
	putCalled             int
	putBodies             []map[string]interface{}
	forceUUIDErr          bool
	forceMissing          bool
	forcePutErr           bool
	uuidFailuresRemaining int
	uuidLookupCalls       int
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
		m.uuidLookupCalls++
		forceErr := m.forceUUIDErr
		if m.uuidFailuresRemaining > 0 {
			m.uuidFailuresRemaining--
			forceErr = true
		}
		forceMissing := m.forceMissing
		targetClientID := m.clientID
		targetUUID := m.clientUUID
		m.mu.Unlock()

		if forceErr {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		reqClientID := r.URL.Query().Get("clientId")
		w.Header().Set("Content-Type", "application/json")
		if reqClientID == targetClientID && targetUUID != "" && !forceMissing {
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

// Exercise the production WatchGateways queue and GatewayReconciler together.
// A transient Keycloak failure behind the Running phase gate must retry with the
// original payload; applying the queue's normal phase-clearing retry transform
// would expand the second attempt into full Kubernetes provisioning.
func TestWatchGateways_KeycloakRetryPreservesGatedPayload(t *testing.T) {
	gatewayID := "gw-keycloak-queue-retry"
	gatewayName := "my-gateway"
	clientID := gatewayName + "-" + gatewayID
	clientUUID := "uuid-keycloak-queue-retry"
	phase := "Running"
	gw := &pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: gatewayID},
		Name:      gatewayName,
		Namespace: "openshell-0011223344556677",
		Phase:     &phase,
	}

	mockKC := newMockKeycloakAdminServer(t, clientID, clientUUID, map[string]interface{}{
		"id":       clientUUID,
		"clientId": clientID,
		"attributes": map[string]interface{}{
			"oauth2.device.authorization.grant.enabled": "false",
		},
	})
	mockKC.mu.Lock()
	mockKC.uuidFailuresRemaining = 1
	mockKC.mu.Unlock()

	grpcConn, gatewayServer := newRecordingGatewayConn(t)
	gatewayServer.setGateway(gw)

	var kubernetesCalls atomic.Int32
	kubeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		kubernetesCalls.Add(1)
		http.Error(w, "unexpected Kubernetes provisioning request", http.StatusInternalServerError)
	}))
	t.Cleanup(kubeServer.Close)
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: kubeServer.URL})
	if err != nil {
		t.Fatalf("create recording Kubernetes client: %v", err)
	}

	fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())
	r := &GatewayReconciler{
		active:        make(map[string]struct{}),
		dynamicClient: fakeDynamic,
		clientset:     clientset,
		grpcConn:      grpcConn,
		keycloakClient: keycloak.NewClient(
			mockKC.server.URL,
			mockKC.realm,
			mockKC.adminClient,
			mockKC.adminSecret,
		),
	}

	ctx, cancel := context.WithCancel(t.Context())
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- watcher.WatchGateways(ctx, grpcConn, r, "")
	}()

	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		mockKC.mu.Lock()
		putCalls := mockKC.putCalled
		mockKC.mu.Unlock()
		if putCalls == 1 {
			break
		}
		select {
		case err := <-watchErr:
			cancel()
			t.Fatalf("WatchGateways returned before the Keycloak retry succeeded: %v", err)
		case <-deadline.C:
			cancel()
			<-watchErr
			mockKC.mu.Lock()
			lookupCalls := mockKC.uuidLookupCalls
			putCalls := mockKC.putCalled
			mockKC.mu.Unlock()
			t.Fatalf("timed out waiting for Keycloak retry: lookups=%d puts=%d", lookupCalls, putCalls)
		case <-ticker.C:
		}
	}

	cancel()
	select {
	case err := <-watchErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WatchGateways returned %v after cancellation, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WatchGateways did not stop after cancellation")
	}

	mockKC.mu.Lock()
	lookupCalls := mockKC.uuidLookupCalls
	putCalls := mockKC.putCalled
	mockKC.mu.Unlock()
	if lookupCalls != 2 || putCalls != 1 {
		t.Fatalf("Keycloak calls: lookups=%d puts=%d, want one failed lookup followed by one successful lookup and PUT", lookupCalls, putCalls)
	}
	if updates := gatewayServer.snapshot(); len(updates) != 0 {
		t.Fatalf("Keycloak-only retry performed provisioning gRPC updates: %v", updates)
	}
	if got := kubernetesCalls.Load(); got != 0 {
		t.Fatalf("Keycloak-only retry made %d typed Kubernetes request(s), want 0", got)
	}
	if actions := fakeDynamic.Actions(); len(actions) != 0 {
		t.Fatalf("Keycloak-only retry performed dynamic Kubernetes actions: %v", actions)
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

func TestGatewayReconciler_Handle_GatedPhase_ReportsMissingClientWithoutKubernetes(t *testing.T) {
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
	grpcConn, gatewayServer := newRecordingGatewayConn(t)

	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         fakeDynamic,
		clientset:             nil,
		grpcConn:              grpcConn,
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
		t.Fatal("expected missing desired client to remain non-converged")
	}
	if !watcher.PreservesPayloadForRetry(err) {
		t.Fatal("missing-client error must preserve the gated payload on retry")
	}
	if !strings.Contains(err.Error(), "desired Keycloak client "+fmt.Sprintf("%q", clientID)+" is missing") {
		t.Errorf("error = %q, want explicit quoted missing-client context", err)
	}

	// The status write generates a watch event. Reprocessing that payload must
	// retain the narrow retry marker without rewriting status on every attempt;
	// the queue's existing dirty-readd test covers enforcement of the backoff floor.
	missingStatus := gatewayKeycloakClientMissingStatus
	event.Resource.Status = &missingStatus
	if retryErr := r.Handle(ctx, event); retryErr == nil || !watcher.PreservesPayloadForRetry(retryErr) {
		t.Fatalf("self-generated status event error = %v, want preserved retry", retryErr)
	}

	updates := gatewayServer.snapshot()
	if len(updates) != 1 {
		t.Fatalf("gateway status updates = %v, want exactly one idempotent marker write", updates)
	}
	if !updates[0].hasStatus || updates[0].status != gatewayKeycloakClientMissingStatus {
		t.Errorf("status update = %#v, want fixed missing-client marker", updates[0])
	}
	if updates[0].hasPhase {
		t.Errorf("missing-client update changed phase to %q; phase must remain unchanged", updates[0].phase)
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

func TestGatewayReconciler_Handle_GatedPhase_InvalidIdentityIsTerminal(t *testing.T) {
	gatewayID := "gw-invalid-identity"
	foreignClientID := "foreign-client"
	oidcData, err := json.Marshal(gateway.OIDCConfig{ClientID: foreignClientID, Audience: foreignClientID})
	if err != nil {
		t.Fatalf("marshal OIDC config: %v", err)
	}
	oidc := string(oidcData)

	var keycloakCalled atomic.Bool
	keycloakServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		keycloakCalled.Store(true)
		http.Error(w, "unexpected Keycloak call", http.StatusInternalServerError)
	}))
	t.Cleanup(keycloakServer.Close)

	grpcConn, gatewayServer := newRecordingGatewayConn(t)
	fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())
	r := &GatewayReconciler{
		active:        make(map[string]struct{}),
		dynamicClient: fakeDynamic,
		grpcConn:      grpcConn,
		keycloakClient: keycloak.NewClient(
			keycloakServer.URL,
			"realm",
			"admin",
			"secret",
		),
	}
	phase := "Running"
	gatewayResource := &pb.Gateway{
		Metadata: &pb.ObjectReference{Id: gatewayID},
		Name:     "current-name",
		Phase:    &phase,
		Oidc:     &oidc,
	}
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource:   gatewayResource,
	}

	if err := r.Handle(t.Context(), event); err != nil {
		t.Fatalf("Handle() error = %v, want terminal validation success after status publication", err)
	}
	updates := gatewayServer.snapshot()
	if len(updates) != 1 {
		t.Fatalf("gateway updates = %v, want one invalid-configuration status", updates)
	}
	if !updates[0].hasStatus || updates[0].status != gatewayKeycloakClientInvalidStatus {
		t.Errorf("status update = %#v, want fixed invalid-configuration marker", updates[0])
	}
	if updates[0].hasPhase {
		t.Errorf("invalid-configuration update changed phase to %q", updates[0].phase)
	}
	if strings.Contains(updates[0].status, foreignClientID) {
		t.Errorf("fixed status %q contains untrusted client identity", updates[0].status)
	}
	if keycloakCalled.Load() {
		t.Fatal("invalid persisted identity reached the Keycloak API")
	}
	if len(fakeDynamic.Actions()) != 0 {
		t.Fatalf("invalid persisted identity triggered Kubernetes actions: %v", fakeDynamic.Actions())
	}

	// The status write emits another watch event. Once the fixed marker is present,
	// terminal validation must remain an idempotent success rather than retrying or
	// rewriting status forever.
	invalidStatus := gatewayKeycloakClientInvalidStatus
	gatewayResource.Status = &invalidStatus
	if err := r.Handle(t.Context(), event); err != nil {
		t.Fatalf("Handle() with existing invalid marker error = %v, want nil", err)
	}
	if got := len(gatewayServer.snapshot()); got != 1 {
		t.Fatalf("status update count = %d, want one idempotent write", got)
	}
}

func TestGatewayReconciler_Handle_GatedPhase_InvalidIdentityStatusFailureRetriesNarrowly(t *testing.T) {
	gatewayID := "gw-invalid-status-failure"
	foreignClientID := "foreign-client"
	oidcData, err := json.Marshal(gateway.OIDCConfig{ClientID: foreignClientID, Audience: foreignClientID})
	if err != nil {
		t.Fatalf("marshal OIDC config: %v", err)
	}
	oidc := string(oidcData)

	var keycloakCalled atomic.Bool
	keycloakServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		keycloakCalled.Store(true)
		http.Error(w, "unexpected Keycloak call", http.StatusInternalServerError)
	}))
	t.Cleanup(keycloakServer.Close)

	grpcConn, gatewayServer := newRecordingGatewayConn(t)
	gatewayServer.setUpdateError(errors.New("status unavailable"))
	fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())
	r := &GatewayReconciler{
		active:        make(map[string]struct{}),
		dynamicClient: fakeDynamic,
		grpcConn:      grpcConn,
		keycloakClient: keycloak.NewClient(
			keycloakServer.URL,
			"realm",
			"admin",
			"secret",
		),
	}
	phase := "Running"
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource: &pb.Gateway{
			Metadata: &pb.ObjectReference{Id: gatewayID},
			Name:     "current-name",
			Phase:    &phase,
			Oidc:     &oidc,
		},
	}

	err = r.Handle(t.Context(), event)
	if err == nil || !watcher.PreservesPayloadForRetry(err) {
		t.Fatalf("Handle() error = %v, want preserved retry for failed status publication", err)
	}
	updates := gatewayServer.snapshot()
	if len(updates) != 1 || !updates[0].hasStatus || updates[0].status != gatewayKeycloakClientInvalidStatus {
		t.Fatalf("attempted update = %v, want fixed invalid-configuration status", updates)
	}
	if updates[0].hasPhase {
		t.Errorf("failed invalid-configuration update attempted phase %q", updates[0].phase)
	}
	if keycloakCalled.Load() {
		t.Fatal("invalid persisted identity reached the Keycloak API")
	}
	if len(fakeDynamic.Actions()) != 0 {
		t.Fatalf("status publication failure triggered Kubernetes actions: %v", fakeDynamic.Actions())
	}
}

func TestGatewayReconciler_Handle_GatedPhase_ClearsMissingStatusAfterKeycloakRecovers(t *testing.T) {
	ctx := context.Background()
	gatewayID := "gw-recovers"
	gatewayName := "my-gateway"
	clientID := gatewayName + "-" + gatewayID
	clientUUID := "uuid-recovers"

	mockKC := newMockKeycloakAdminServer(t, clientID, clientUUID, map[string]interface{}{
		"id":       clientUUID,
		"clientId": clientID,
		"attributes": map[string]interface{}{
			"oauth2.device.authorization.grant.enabled": "false",
		},
	})
	mockKC.mu.Lock()
	mockKC.forceMissing = true
	mockKC.mu.Unlock()

	grpcConn, gatewayServer := newRecordingGatewayConn(t)
	fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())
	r := &GatewayReconciler{
		active:        make(map[string]struct{}),
		dynamicClient: fakeDynamic,
		clientset:     nil,
		grpcConn:      grpcConn,
		keycloakClient: keycloak.NewClient(
			mockKC.server.URL,
			mockKC.realm,
			mockKC.adminClient,
			mockKC.adminSecret,
		),
	}
	phase := "Running"
	gatewayResource := &pb.Gateway{
		Metadata: &pb.ObjectReference{Id: gatewayID},
		Name:     gatewayName,
		Phase:    &phase,
	}
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource:   gatewayResource,
	}

	if err := r.Handle(ctx, event); err == nil || !watcher.PreservesPayloadForRetry(err) {
		t.Fatalf("initial missing-client Handle() error = %v, want preserved retry", err)
	}

	mockKC.mu.Lock()
	mockKC.forceMissing = false
	mockKC.mu.Unlock()
	missingStatus := gatewayKeycloakClientMissingStatus
	gatewayResource.Status = &missingStatus

	if err := r.Handle(ctx, event); err != nil {
		t.Fatalf("Handle() after Keycloak recovery returned error: %v", err)
	}

	updates := gatewayServer.snapshot()
	if len(updates) != 2 {
		t.Fatalf("gateway updates = %v, want missing marker then clear", updates)
	}
	clear := updates[1]
	if !clear.hasStatus || clear.status != "" {
		t.Errorf("recovery update = %#v, want empty status", clear)
	}
	if clear.hasPhase {
		t.Errorf("recovery update changed phase to %q; workload phase must remain unchanged", clear.phase)
	}
	if len(fakeDynamic.Actions()) != 0 {
		t.Errorf("expected no Kubernetes actions during Keycloak recovery, got %v", fakeDynamic.Actions())
	}
}

func TestGatewayReconciler_Handle_GatedPhase_ClearsInvalidStatusAfterIdentityRecovers(t *testing.T) {
	gatewayID := "gw-valid-again"
	gatewayName := "my-gateway"
	clientID := gatewayName + "-" + gatewayID
	clientUUID := "uuid-valid-again"
	mockKC := newMockKeycloakAdminServer(t, clientID, clientUUID, map[string]interface{}{
		"id":       clientUUID,
		"clientId": clientID,
		"attributes": map[string]interface{}{
			"oauth2.device.authorization.grant.enabled": "true",
		},
	})

	grpcConn, gatewayServer := newRecordingGatewayConn(t)
	fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())
	r := &GatewayReconciler{
		active:        make(map[string]struct{}),
		dynamicClient: fakeDynamic,
		grpcConn:      grpcConn,
		keycloakClient: keycloak.NewClient(
			mockKC.server.URL,
			mockKC.realm,
			mockKC.adminClient,
			mockKC.adminSecret,
		),
	}
	phase := "Running"
	invalidStatus := gatewayKeycloakClientInvalidStatus
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource: &pb.Gateway{
			Metadata: &pb.ObjectReference{Id: gatewayID},
			Name:     gatewayName,
			Phase:    &phase,
			Status:   &invalidStatus,
		},
	}

	if err := r.Handle(t.Context(), event); err != nil {
		t.Fatalf("Handle() after identity recovery error = %v", err)
	}
	updates := gatewayServer.snapshot()
	if len(updates) != 1 || !updates[0].hasStatus || updates[0].status != "" {
		t.Fatalf("gateway updates = %v, want one status clear", updates)
	}
	if updates[0].hasPhase {
		t.Errorf("identity recovery changed phase to %q", updates[0].phase)
	}
	if len(fakeDynamic.Actions()) != 0 {
		t.Fatalf("identity recovery triggered Kubernetes actions: %v", fakeDynamic.Actions())
	}
}

func TestGatewayReconciler_Handle_GatedPhase_UsesPersistedClientIdentityAfterRename(t *testing.T) {
	ctx := context.Background()
	gatewayID := "gw-renamed"
	originalClientID := "original-name-" + gatewayID
	clientUUID := "uuid-renamed"

	for _, tc := range []struct {
		name string
		oidc gateway.OIDCConfig
	}{
		{name: "client_id", oidc: gateway.OIDCConfig{ClientID: originalClientID, Audience: originalClientID}},
		{name: "legacy audience", oidc: gateway.OIDCConfig{Audience: originalClientID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oidcJSON, err := json.Marshal(tc.oidc)
			if err != nil {
				t.Fatalf("marshal OIDC config: %v", err)
			}
			mockKC := newMockKeycloakAdminServer(t, originalClientID, clientUUID, map[string]interface{}{
				"id":       clientUUID,
				"clientId": originalClientID,
				"attributes": map[string]interface{}{
					"oauth2.device.authorization.grant.enabled": "false",
				},
			})
			fakeDynamic := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())
			r := &GatewayReconciler{
				active:        make(map[string]struct{}),
				dynamicClient: fakeDynamic,
				clientset:     nil,
				keycloakClient: keycloak.NewClient(
					mockKC.server.URL,
					mockKC.realm,
					mockKC.adminClient,
					mockKC.adminSecret,
				),
			}
			phase := "Running"
			oidcValue := string(oidcJSON)
			event := watcher.Event[*pb.Gateway]{
				Type:       watcher.EventUpdated,
				ResourceID: gatewayID,
				Resource: &pb.Gateway{
					Metadata: &pb.ObjectReference{Id: gatewayID},
					Name:     "renamed-gateway",
					Phase:    &phase,
					Oidc:     &oidcValue,
				},
			}

			if err := r.Handle(ctx, event); err != nil {
				t.Fatalf("Handle() returned unexpected error: %v", err)
			}
			mockKC.mu.Lock()
			putCount := mockKC.putCalled
			mockKC.mu.Unlock()
			if putCount != 1 {
				t.Errorf("expected persisted client to be updated once, got %d PUTs", putCount)
			}
			if len(fakeDynamic.Actions()) > 0 {
				t.Errorf("expected 0 Kubernetes actions for renamed gated gateway")
			}
		})
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
	if !watcher.PreservesPayloadForRetry(err) {
		t.Fatal("Keycloak lookup error must preserve the gated payload on retry")
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
	if !watcher.PreservesPayloadForRetry(err) {
		t.Fatal("Keycloak update error must preserve the gated payload on retry")
	}
	if !strings.Contains(err.Error(), "reconcile Keycloak client for gateway") {
		t.Errorf("error = %q, want wrapping with 'reconcile Keycloak client for gateway'", err.Error())
	}
}
