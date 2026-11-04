package reconciler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// fakeGatewayReReadClient records the unscoped re-read request and returns a
// scripted response, so the nil-payload delete path can be exercised without a
// gRPC server.
type fakeGatewayReReadClient struct {
	pb.GatewayServiceClient
	resp              *pb.GetGatewayResponse
	err               error
	gotIncludeDeleted bool
	gotID             string
}

func (f *fakeGatewayReReadClient) GetGateway(_ context.Context, in *pb.GetGatewayRequest, _ ...grpc.CallOption) (*pb.GetGatewayResponse, error) {
	f.gotID = in.GetId()
	f.gotIncludeDeleted = in.GetIncludeDeleted()
	return f.resp, f.err
}

// eventRecordingKube returns an httptest server that records Event creations in
// the control-plane namespace and refuses every other request, so the durable
// orphan signal can be asserted without standing up unrelated namespaced
// resources. The returned typed clientset is wired to it.
func eventRecordingKube(t *testing.T, cpNamespace string) (*kubernetes.Clientset, func() []corev1.Event) {
	t.Helper()
	eventsPath := "/api/v1/namespaces/" + cpNamespace + "/events"
	var mu sync.Mutex
	var events []corev1.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != eventsPath {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var ev corev1.Event
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			t.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(ev); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	typed, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() []corev1.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]corev1.Event(nil), events...)
	}
	return typed, snapshot
}

func orphanedKinds(events []corev1.Event) map[string]bool {
	kinds := map[string]bool{}
	for _, ev := range events {
		for _, kind := range []string{"PostgreSQLDatabase", "KeycloakClient"} {
			if strings.Contains(ev.Message, kind) {
				kinds[kind] = true
			}
		}
	}
	return kinds
}

// TestNilPayloadDeleteUnloadableRecordDefersAndRecordsOrphan verifies
// CTRL-WDEL-04.02: a nil-payload delete whose record cannot be re-read returns a
// retryable error and records the no-silent-orphan signal for the
// out-of-namespace owned resources, rather than returning success.
// Verifies: CTRL-WDEL-04.02, GTW-DEL-07
func TestNilPayloadDeleteUnloadableRecordDefersAndRecordsOrphan(t *testing.T) {
	const cpNamespace = "hypershell-control-plane"
	typed, events := eventRecordingKube(t, cpNamespace)
	fake := &fakeGatewayReReadClient{err: status.Error(codes.Unavailable, "api down")}
	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		clientset:             typed,
		dynamicClient:         dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		controlPlaneNamespace: cpNamespace,
		gatewayClientOverride: fake,
	}

	err := r.Handle(t.Context(), watcher.Event[*pb.Gateway]{Type: watcher.EventDeleted, ResourceID: "gw-1", Resource: nil})
	if err == nil {
		t.Fatal("nil-payload delete returned success despite an unloadable record")
	}
	if !fake.gotIncludeDeleted {
		t.Fatal("re-read did not request the unscoped (include_deleted) query")
	}
	if fake.gotID != "gw-1" {
		t.Fatalf("re-read id = %q, want gw-1", fake.gotID)
	}
	kinds := orphanedKinds(events())
	if !kinds["PostgreSQLDatabase"] || !kinds["KeycloakClient"] {
		t.Fatalf("missing no-silent-orphan signal for out-of-namespace resources: recorded kinds = %v", kinds)
	}
}

// TestNilPayloadDeleteReconcilesFromReReadRecord verifies CTRL-WDEL-04.01: a
// nil-payload delete whose record is loadable with the unscoped query reconciles
// teardown from that record (here the re-read supplies the namespace, which the
// teardown then acts on).
// Verifies: CTRL-WDEL-04.01
func TestNilPayloadDeleteReconcilesFromReReadRecord(t *testing.T) {
	const cpNamespace = "hypershell-control-plane"
	var mu sync.Mutex
	var sawNamespacePath bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "gw-ns") {
			mu.Lock()
			sawNamespacePath = true
			mu.Unlock()
		}
		// Fail every teardown request so Handle surfaces an error; the point under
		// test is that teardown is driven from the re-read record's namespace, not
		// that it completes.
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	typed, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json"}})
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeGatewayReReadClient{resp: &pb.GetGatewayResponse{Gateway: &pb.Gateway{Name: "gw", Namespace: "gw-ns"}}}
	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		clientset:             typed,
		dynamicClient:         dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		controlPlaneNamespace: cpNamespace,
		gatewayClientOverride: fake,
	}

	// Teardown will fail against the stub server, so an error is expected; the
	// assertion is that the re-read ran unscoped and teardown targeted the
	// re-read record's namespace.
	_ = r.Handle(t.Context(), watcher.Event[*pb.Gateway]{Type: watcher.EventDeleted, ResourceID: "gw-1", Resource: nil})
	if !fake.gotIncludeDeleted {
		t.Fatal("re-read did not request the unscoped (include_deleted) query")
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawNamespacePath {
		t.Fatal("teardown was not reconciled from the re-read record's namespace (gw-ns)")
	}
}

// TestDeleteWithUnresolvedNamespaceDefersAndRecordsOrphan verifies the
// missing-namespace fix (GTW-DEL-01/04/07): a delete whose record carries no
// namespace returns a retryable error and records the no-silent-orphan signal,
// rather than skipping cleanup and reporting success. Because the record is
// loaded here, the signal names only the classes the gateway actually owns: every
// gateway owns a database, but a gateway with no OIDC never had Keycloak clients.
// Verifies: GTW-DEL-01, GTW-DEL-04, GTW-DEL-07
func TestDeleteWithUnresolvedNamespaceDefersAndRecordsOrphan(t *testing.T) {
	const cpNamespace = "hypershell-control-plane"
	oidc := `{"client_id":"gw-1","audience":"gw-1"}`
	for _, tc := range []struct {
		name         string
		oidc         *string
		wantKeycloak bool
	}{
		{name: "with OIDC records database and keycloak", oidc: &oidc, wantKeycloak: true},
		{name: "without OIDC records only database", oidc: nil, wantKeycloak: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typed, events := eventRecordingKube(t, cpNamespace)
			r := &GatewayReconciler{
				active:                make(map[string]struct{}),
				clientset:             typed,
				dynamicClient:         dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
				controlPlaneNamespace: cpNamespace,
			}

			err := r.Handle(t.Context(), watcher.Event[*pb.Gateway]{Type: watcher.EventDeleted, ResourceID: "gw-1", Resource: &pb.Gateway{Name: "gw", Oidc: tc.oidc}})
			if err == nil {
				t.Fatal("delete with unresolved namespace returned success")
			}
			if !strings.Contains(err.Error(), "namespace") {
				t.Fatalf("error does not identify the unresolved namespace precondition: %v", err)
			}
			kinds := orphanedKinds(events())
			if !kinds["PostgreSQLDatabase"] {
				t.Fatalf("missing database no-silent-orphan signal: recorded kinds = %v", kinds)
			}
			if kinds["KeycloakClient"] != tc.wantKeycloak {
				t.Fatalf("KeycloakClient orphan recorded = %v, want %v: recorded kinds = %v", kinds["KeycloakClient"], tc.wantKeycloak, kinds)
			}
		})
	}
}
