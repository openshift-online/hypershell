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
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestGatewayDeletionUsesValidatedStoredIdentity(t *testing.T) {
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
			// requiring unrelated namespaced Kubernetes resources.
			kube := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "namespace service unavailable", http.StatusServiceUnavailable)
			}))
			defer kube.Close()
			typed, err := kubernetes.NewForConfig(&rest.Config{Host: kube.URL})
			if err != nil {
				t.Fatal(err)
			}
			r := &GatewayReconciler{
				active: make(map[string]struct{}), clientset: typed,
				dynamicClient:  dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
				keycloakClient: keycloak.NewClient(identity.URL, "test", "provisioner", t.Name()),
			}
			if tc.noKeycloak {
				r.keycloakClient = nil
			}
			err = r.Handle(t.Context(), watcher.Event[*pb.Gateway]{Type: watcher.EventDeleted, ResourceID: "gateway-id", Resource: &pb.Gateway{Name: "renamed", Namespace: "gateway-ns", Oidc: &tc.oidc}})
			if err == nil {
				t.Fatal("namespace failure was lost")
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
				if tc.oidc != "" && !strings.Contains(logs.String(), "original-gateway-id") {
					t.Fatalf("missing provisioner log omitted client identity: %q", logs.String())
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
			} else if want := []string{tc.wantClient + "-console", tc.wantClient}; !reflect.DeepEqual(lookups, want) {
				t.Fatalf("client lookups = %v, want %v", lookups, want)
			}
		})
	}
}
