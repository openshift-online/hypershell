package reconciler

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Verifies: OSG-KCLK-11.01, OSG-KCLK-11.02, OSG-KCLK-11.03, GTW-DEL-07.04
func TestGatewayDeletionUsesValidatedStoredIdentity(t *testing.T) {
	const cpNamespace = "hypershell-control-plane"
	eventsPath := "/api/v1/namespaces/" + cpNamespace + "/events"
	for _, tc := range []struct {
		name, oidc, wantClient string
		noKeycloak             bool
		invalid                bool
	}{
		{name: "renamed", oidc: `{"client_id":"original-gateway-id","audience":"original-gateway-id"}`, wantClient: "original-gateway-id"},
		{name: "legacy audience", oidc: `{"audience":"original-gateway-id"}`, wantClient: "original-gateway-id"},
		{name: "legacy name fallback", wantClient: "renamed-gateway-id"},
		{name: "another gateway", oidc: `{"client_id":"other-gateway"}`, invalid: true},
		{name: "conflicting identity", oidc: `{"client_id":"original-gateway-id","audience":"another-gateway-id"}`, invalid: true},
		{name: "invalid JSON", oidc: `{`, invalid: true},
		{name: "no identity configured", noKeycloak: true},
		{name: "missing provisioner", oidc: `{"client_id":"original-gateway-id"}`, noKeycloak: true},
		{name: "missing provisioner with invalid identity", oidc: `{`, noKeycloak: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			prior := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(prior) })
			var lookups []string
			requests := 0
			identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/realms/test/protocol/openid-connect/token":
					if err := json.NewEncoder(w).Encode(map[string]interface{}{"access_token": t.Name(), "expires_in": 300}); err != nil {
						t.Error(err)
					}
				case "/admin/realms/test/clients":
					if id := r.URL.Query().Get("clientId"); id != "" {
						lookups = append(lookups, id)
					}
					if _, err := w.Write([]byte(`[]`)); err != nil {
						t.Error(err)
					}
				default:
					t.Errorf("unexpected identity request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer identity.Close()
			// Keep namespace deletion pending to observe identity cleanup without
			// requiring unrelated namespaced Kubernetes resources. Only Event
			// creation in the control-plane namespace succeeds, so the durable
			// orphan signal can be asserted.
			var eventsMu sync.Mutex
			var events []corev1.Event
			kube := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != eventsPath {
					http.Error(w, "namespace service unavailable", http.StatusServiceUnavailable)
					return
				}
				var ev corev1.Event
				if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
					t.Error(err)
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				eventsMu.Lock()
				events = append(events, ev)
				eventsMu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				if err := json.NewEncoder(w).Encode(ev); err != nil {
					t.Error(err)
				}
			}))
			defer kube.Close()
			typed, err := kubernetes.NewForConfig(&rest.Config{Host: kube.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json"}})
			if err != nil {
				t.Fatal(err)
			}
			r := &GatewayReconciler{
				active: make(map[string]struct{}), clientset: typed,
				dynamicClient:         dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
				keycloakClient:        keycloak.NewClient(identity.URL, "test", "provisioner", t.Name()),
				controlPlaneNamespace: cpNamespace,
			}
			if tc.noKeycloak {
				r.keycloakClient = nil
			}
			err = r.Handle(t.Context(), watcher.Event[*pb.Gateway]{Type: watcher.EventDeleted, ResourceID: "gateway-id", Resource: &pb.Gateway{Name: "renamed", Namespace: "gateway-ns", Oidc: &tc.oidc}})
			if err == nil {
				t.Fatal("namespace failure was lost")
			}
			// Other teardown steps (for example the Vault mount) may record their
			// own orphan Events; only the Keycloak client signal is in scope here.
			var recorded []corev1.Event
			eventsMu.Lock()
			for _, ev := range events {
				if strings.Contains(ev.Message, "KeycloakClient") {
					recorded = append(recorded, ev)
				}
			}
			eventsMu.Unlock()
			// assertOrphanEvents checks the durable IncompleteFinalization signal:
			// one Warning Event per unreclaimed Keycloak client, in the
			// control-plane namespace, naming the gateway.
			assertOrphanEvents := func(want int) {
				t.Helper()
				if len(recorded) != want {
					t.Fatalf("recorded %d IncompleteFinalization Events, want %d: %+v (logs=%q)", len(recorded), want, recorded, logs.String())
				}
				for _, ev := range recorded {
					if ev.Reason != "IncompleteFinalization" || ev.Type != corev1.EventTypeWarning {
						t.Fatalf("event reason/type = %q/%q, want IncompleteFinalization/Warning", ev.Reason, ev.Type)
					}
					if ev.Namespace != cpNamespace || ev.InvolvedObject.Name != "gateway-id" {
						t.Fatalf("event namespace/object = %q/%q, want %q/gateway-id", ev.Namespace, ev.InvolvedObject.Name, cpNamespace)
					}
					if !strings.Contains(ev.Message, "unreclaimed") {
						t.Fatalf("event message does not name the unreclaimed Keycloak client: %q", ev.Message)
					}
				}
			}
			if tc.noKeycloak {
				if requests != 0 {
					t.Fatal("unconfigured Keycloak received a request")
				}
				reported := strings.Contains(logs.String(), "identity cleanup requires the Keycloak client")
				if reported != (tc.oidc != "") {
					t.Fatalf("unexpected missing provisioner report: logs=%q err=%v", logs.String(), err)
				}
				if strings.Contains(err.Error(), "identity cleanup requires the Keycloak client") {
					t.Fatalf("missing provisioner blocked finalization: %v", err)
				}
				switch {
				case tc.oidc == "":
					assertOrphanEvents(0)
				case tc.invalid:
					// No client identifier is known, so one Event names the gateway.
					assertOrphanEvents(1)
				default:
					if !strings.Contains(logs.String(), "original-gateway-id") {
						t.Fatalf("missing provisioner log omitted client identity: %q", logs.String())
					}
					// One Event each for the gateway and console clients.
					assertOrphanEvents(2)
				}
			} else if tc.invalid {
				var identityErr *gatewayKeycloakClientIdentityError
				if errors.As(err, &identityErr) {
					t.Fatalf("invalid identity blocked finalization: %v", err)
				}
				if requests != 0 {
					t.Fatal("invalid identity reached Keycloak")
				}
				if !strings.Contains(logs.String(), "stored identity cannot be resolved") {
					t.Fatalf("invalid identity was not reported for operator recovery: %q", logs.String())
				}
				assertOrphanEvents(1)
			} else if want := []string{tc.wantClient + "-console", tc.wantClient}; !reflect.DeepEqual(lookups, want) {
				t.Fatalf("client lookups = %v, want %v", lookups, want)
			}
		})
	}
}
