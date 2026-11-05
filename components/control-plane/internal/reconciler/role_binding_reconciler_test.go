package reconciler

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

func roleBinding(id, roleName string) *pb.RoleBinding {
	return &pb.RoleBinding{
		Metadata: &pb.ObjectReference{Id: id},
		RoleName: roleName,
	}
}

// Deleting an owner binding while a viewer binding survives must NOT revoke
// openshell-user: both roles grant it, so it is still desired. Only
// openshell-admin (owner-exclusive) should end up revocable.
func TestUnionKcRoles_OverlapKeepsSharedRole(t *testing.T) {
	const deletedID = "rb-owner"
	remaining := []*pb.RoleBinding{
		roleBinding(deletedID, "gateway:owner"), // the one being deleted -- excluded
		roleBinding("rb-viewer", "gateway:viewer"),
	}

	stillDesired := unionKcRoles(remaining, deletedID)

	if !stillDesired["openshell-user"] {
		t.Error("openshell-user must remain desired: the viewer binding still grants it")
	}
	if stillDesired["openshell-admin"] {
		t.Error("openshell-admin must not be desired: only the deleted owner binding granted it")
	}

	// Confirm the revoke decision for the deleted owner binding: keep
	// openshell-user, revoke openshell-admin.
	var revoked []string
	for _, kcRole := range keycloakRoleMap["gateway:owner"] {
		if !stillDesired[kcRole] {
			revoked = append(revoked, kcRole)
		}
	}
	if len(revoked) != 1 || revoked[0] != "openshell-admin" {
		t.Errorf("revoked = %v, want [openshell-admin]", revoked)
	}
}

// With no surviving bindings, every role the deleted binding granted becomes
// revocable.
// Verifies: OSG-KCLK-05.05
func TestUnionKcRoles_NoRemainingBindingsRevokesAll(t *testing.T) {
	const deletedID = "rb-owner"
	stillDesired := unionKcRoles([]*pb.RoleBinding{roleBinding(deletedID, "gateway:owner")}, deletedID)
	if len(stillDesired) != 0 {
		t.Errorf("stillDesired = %v, want empty", stillDesired)
	}
}

// A gateway owner must receive BOTH openshell-admin and openshell-user on the
// per-gateway console client. The gateway admin API enforces the two roles
// independently (admin does not imply user), so an owner missing openshell-user
// can read gateway info but is refused "list workspaces" with
// "role 'openshell-user' required". A viewer receives only openshell-user.
// Verifies: OSG-KCLK-05.01, OSG-KCLK-05.02, OSG-KCLK-05.03
func TestKeycloakRoleMap_OwnerGetsAdminAndUser(t *testing.T) {
	cases := []struct {
		roleBinding string
		want        []string
	}{
		{"gateway:owner", []string{"openshell-admin", "openshell-user"}},
		{"gateway:admin", []string{"openshell-admin", "openshell-user"}},
		{"gateway:viewer", []string{"openshell-user"}},
	}
	for _, tc := range cases {
		got, ok := keycloakRoleMap[tc.roleBinding]
		if !ok {
			t.Fatalf("keycloakRoleMap missing mapping for %q", tc.roleBinding)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("keycloakRoleMap[%q] = %v, want %v", tc.roleBinding, got, tc.want)
		}
		for i, role := range tc.want {
			if got[i] != role {
				t.Errorf("keycloakRoleMap[%q][%d] = %q, want %q", tc.roleBinding, i, got[i], role)
			}
		}
	}
}

// After an in-place demotion from gateway:admin to gateway:viewer (a single
// UPDATED event, one surviving binding), union reconcile must assign
// openshell-user and remove openshell-admin (GTW-ACC-02).
// Verifies: OSG-KCLK-05.04
func TestUnionReconcile_DemotionToViewerStripsAdminKeepsUser(t *testing.T) {
	surviving := []*pb.RoleBinding{roleBinding("rb-1", "gateway:viewer")}
	desired := unionKcRoles(surviving, "")

	var assigned, removed []string
	for _, kc := range allGatewayKcRoles {
		if desired[kc] {
			assigned = append(assigned, kc)
		} else {
			removed = append(removed, kc)
		}
	}
	if len(assigned) != 1 || assigned[0] != "openshell-user" {
		t.Errorf("assigned = %v, want [openshell-user]", assigned)
	}
	if len(removed) != 1 || removed[0] != "openshell-admin" {
		t.Errorf("removed = %v, want [openshell-admin]", removed)
	}
}

// A granted admin (gateway:admin) must receive both client roles, like an owner.
// Verifies: OSG-KCLK-05.02
func TestUnionReconcile_AdminGetsBothRoles(t *testing.T) {
	desired := unionKcRoles([]*pb.RoleBinding{roleBinding("rb-1", "gateway:admin")}, "")
	if !desired["openshell-admin"] || !desired["openshell-user"] {
		t.Errorf("gateway:admin union = %v, want both openshell-admin and openshell-user", desired)
	}
}

func TestHandle_EmptyUsernameIsError(t *testing.T) {
	r := NewRoleBindingReconciler(keycloak.NewClient("http://keycloak", "hypershell", "id", "secret"), nil, "cluster-1")
	gatewayID := "gw-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			Username:  "",
		},
	})
	if err == nil {
		t.Fatal("empty username must be an error so the queue retries")
	}
	if !strings.Contains(err.Error(), "username not yet resolved") {
		t.Fatalf("error = %v, want username not yet resolved", err)
	}
}

func TestHandle_EmptyRoleNameIsError(t *testing.T) {
	r := NewRoleBindingReconciler(keycloak.NewClient("http://keycloak", "hypershell", "id", "secret"), nil, "cluster-1")
	gatewayID := "gw-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "",
			GatewayId: &gatewayID,
			Username:  "service-account-hypershell-e2e",
		},
	})
	if err == nil {
		t.Fatal("empty role name must be an error so the queue retries")
	}
	if !strings.Contains(err.Error(), "role name not yet resolved") {
		t.Fatalf("error = %v, want role name not yet resolved", err)
	}
}

// A binding for a gateway hosted by another cluster is not this control
// plane's to sync. The api-server's cluster-scoped watch should never deliver
// one; should one arrive anyway (defense in depth), the event is dropped
// without touching Keycloak and without an error, so the queue does not retry
// it.
// Verifies: CTRL-CORE-01.07
func TestHandle_SkipsGatewayOfAnotherCluster(t *testing.T) {
	conn, recorder := newRecordingGatewayConn(t)
	recorder.setGateway(&pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "team-gateway",
		ClusterId: "cluster-2",
	})
	// An unroutable Keycloak: any attempt to assign a role fails loudly.
	r := NewRoleBindingReconciler(keycloak.NewClient("http://127.0.0.1:1", "hypershell", "id", "secret"), conn, "cluster-1")
	gatewayID := "gw-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			Username:  "alice",
		},
	})
	if err != nil {
		t.Fatalf("binding for another cluster's gateway must be skipped without error, got %v", err)
	}
}

// The same binding on the control plane that owns the gateway proceeds to the
// Keycloak sync (which here fails because Keycloak is unroutable), proving the
// cluster check is what gates the skip above.
func TestHandle_OwnClusterGatewayReachesKeycloak(t *testing.T) {
	// Union reconcile lists the user's surviving bindings before touching
	// Keycloak, so the stub must serve RoleBindingService (empty is fine; the
	// event binding's own roles are still included in the desired union).
	conn, recorder, _ := newRecordingGatewayAndRoleBindingConn(t)
	recorder.setGateway(&pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "team-gateway",
		ClusterId: "cluster-1",
	})
	r := NewRoleBindingReconciler(keycloak.NewClient("http://127.0.0.1:1", "hypershell", "id", "secret"), conn, "cluster-1")
	gatewayID := "gw-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			Username:  "alice",
		},
	})
	if err == nil {
		t.Fatal("binding for this cluster's gateway must reach Keycloak and surface its error")
	}
	if !strings.Contains(err.Error(), "assign keycloak role") {
		t.Fatalf("error = %v, want a keycloak assignment error", err)
	}
}

// A binding whose gateway no longer exists (deleted while the stream was down, or
// surfaced by a resync that lists bindings outliving their gateway) is a benign
// no-op: there is no per-gateway Keycloak client left to reconcile. Handle must
// return nil -- not a (terminal) error -- so the reconcile queue does not log at
// ERROR on every reconnect and resync interval, forever, for every such binding.
// An unroutable Keycloak proves the no-op happens before any Keycloak call: if
// the reconciler proceeded past the missing gateway it would fail loudly instead.
// Verifies: OSG-KCLK-10.03
func TestHandle_MissingGatewayIsBenignNoOp(t *testing.T) {
	// No gateway is set on the recorder, so GetGateway returns codes.NotFound for
	// any id -- exactly the missing-gateway condition under test.
	conn, _ := newRecordingGatewayConn(t)
	r := NewRoleBindingReconciler(keycloak.NewClient("http://127.0.0.1:1", "hypershell", "id", "secret"), conn, "cluster-1")
	gatewayID := "gw-gone"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			Username:  "alice",
		},
	})
	if err != nil {
		t.Fatalf("a binding whose gateway is gone must be a no-op, got %v", err)
	}
}

// listResult is one scripted ListRoleBindings outcome: either a set of surviving
// bindings (items) or an RPC error. A recordingRoleBindingServer serves these in
// order so a test can model the confirmed-revocation guard's two reads (GTW-ACC-15):
// the first empty-success list followed by a confirming read that disagrees.
type listResult struct {
	items []*pb.RoleBinding
	err   error
}

// recordingRoleBindingServer records each ListRoleBindings request so tests can
// assert the delete path scopes its recompute to this control plane's cluster and
// re-confirms absence before revoking. When results are scripted they are served
// in order; once exhausted (or when none are scripted) it returns an empty-success
// list, preserving the default used by the pre-GTW-ACC-15 tests.
type recordingRoleBindingServer struct {
	pb.UnimplementedRoleBindingServiceServer

	mu      sync.Mutex
	lists   []*pb.ListRoleBindingsRequest
	results []listResult
}

func (s *recordingRoleBindingServer) ListRoleBindings(_ context.Context, req *pb.ListRoleBindingsRequest) (*pb.ListRoleBindingsResponse, error) {
	s.mu.Lock()
	s.lists = append(s.lists, req)
	var res listResult
	if len(s.results) > 0 {
		res = s.results[0]
		s.results = s.results[1:]
	}
	s.mu.Unlock()
	if res.err != nil {
		return nil, res.err
	}
	return &pb.ListRoleBindingsResponse{Items: res.items}, nil
}

// setResults scripts the ordered outcomes ListRoleBindings returns.
func (s *recordingRoleBindingServer) setResults(results ...listResult) {
	s.mu.Lock()
	s.results = results
	s.mu.Unlock()
}

func (s *recordingRoleBindingServer) snapshot() []*pb.ListRoleBindingsRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*pb.ListRoleBindingsRequest(nil), s.lists...)
}

// newRecordingGatewayAndRoleBindingConn serves both a recording gateway server
// and a recording role binding server on one in-memory connection.
func newRecordingGatewayAndRoleBindingConn(t *testing.T) (*grpc.ClientConn, *recordingGatewayServer, *recordingRoleBindingServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	gateways := &recordingGatewayServer{}
	bindings := &recordingRoleBindingServer{}
	pb.RegisterGatewayServiceServer(server, gateways)
	pb.RegisterRoleBindingServiceServer(server, bindings)
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
		t.Fatalf("dial recording servers: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, gateways, bindings
}

// On delete the reconciler recomputes the user's surviving roles with
// ListRoleBindings. The api-server requires a registered control plane to scope
// that list to its own cluster, so the request must carry this cluster's id. When
// the first read is empty the confirmed-revocation guard (GTW-ACC-15) issues a
// second, identically scoped confirming read before revoking; both must carry the
// cluster id and the same (user, gateway).
func TestHandle_DeleteListsSurvivingBindingsScopedToCluster(t *testing.T) {
	conn, gateways, bindings := newRecordingGatewayAndRoleBindingConn(t)
	gateways.setGateway(&pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "team-gateway",
		ClusterId: "cluster-1",
	})
	// An unroutable Keycloak: the revoke after the list fails loudly.
	r := NewRoleBindingReconciler(keycloak.NewClient("http://127.0.0.1:1", "hypershell", "id", "secret"), conn, "cluster-1")
	gatewayID := "gw-1"
	userID := "user-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventDeleted,
		Resource: &pb.RoleBinding{
			Metadata:  &pb.ObjectReference{Id: "rb-1"},
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			UserId:    &userID,
			Username:  "alice",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "remove keycloak role") {
		t.Fatalf("error = %v, want a keycloak revoke error after the list", err)
	}

	lists := bindings.snapshot()
	if len(lists) != 2 {
		t.Fatalf("ListRoleBindings calls = %d, want 2 (first read + confirming read)", len(lists))
	}
	for i, req := range lists {
		if req.ClusterId == nil || *req.ClusterId != "cluster-1" {
			t.Fatalf("ListRoleBindings[%d] cluster_id = %v, want %q", i, req.ClusterId, "cluster-1")
		}
		if req.GetUserId() != userID || req.GetGatewayId() != gatewayID {
			t.Fatalf("ListRoleBindings[%d] user/gateway = %q/%q, want %q/%q", i, req.GetUserId(), req.GetGatewayId(), userID, gatewayID)
		}
	}
}

// deleteRoleBindingEvent builds an EventDeleted for the owner binding under test.
func deleteRoleBindingEvent(gatewayID, userID string) watcher.Event[*pb.RoleBinding] {
	return watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventDeleted,
		Resource: &pb.RoleBinding{
			Metadata:  &pb.ObjectReference{Id: "rb-1"},
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			UserId:    &userID,
			Username:  "bob",
		},
	}
}

// deleteGuardReconciler wires a recording gateway + role-binding connection with
// gw-1 on this cluster and an unroutable Keycloak, so any revoke that is reached
// fails loudly with "remove keycloak role". A guard that requeues before revoking
// therefore surfaces its own error, never a Keycloak one.
func deleteGuardReconciler(t *testing.T) (*RoleBindingReconciler, *recordingRoleBindingServer) {
	t.Helper()
	conn, gateways, bindings := newRecordingGatewayAndRoleBindingConn(t)
	gateways.setGateway(&pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "team-gateway",
		ClusterId: "cluster-1",
	})
	r := NewRoleBindingReconciler(keycloak.NewClient("http://127.0.0.1:1", "hypershell", "id", "secret"), conn, "cluster-1")
	return r, bindings
}

// GTW-ACC-15.01: a delete whose first ListRoleBindings is empty but whose
// confirming read returns a still-backed binding must NOT revoke; it must requeue.
// Verifies: GTW-ACC-15.01
func TestHandle_DeleteRequeuesWhenConfirmingReadFindsSurvivingBinding(t *testing.T) {
	r, bindings := deleteGuardReconciler(t)
	// First read: transient empty. Confirming read: a surviving viewer binding
	// (a different id from the deleted owner) still backing openshell-user.
	bindings.setResults(
		listResult{},
		listResult{items: []*pb.RoleBinding{roleBinding("rb-viewer", "gateway:viewer")}},
	)

	err := r.Handle(context.Background(), deleteRoleBindingEvent("gw-1", "user-1"))
	if err == nil {
		t.Fatal("an unconfirmed empty read must requeue with an error, got nil")
	}
	if strings.Contains(err.Error(), "remove keycloak role") {
		t.Fatalf("guard must requeue before any revoke, but a Keycloak removal was attempted: %v", err)
	}
	if !strings.Contains(err.Error(), "confirming read found") {
		t.Fatalf("error = %v, want a requeue citing the disagreeing confirming read", err)
	}
	if got := len(bindings.snapshot()); got != 2 {
		t.Fatalf("ListRoleBindings calls = %d, want 2 (first + confirming read)", got)
	}
}

// recordingKeycloak is a minimal in-memory Keycloak that records which client
// roles RemoveClientRole deletes, so a test can assert a confirmed last-binding
// delete revokes BOTH openshell-admin and openshell-user (not merely that the
// revoke path was entered). It serves only the endpoints RemoveClientRole walks:
// token, client lookup, client-role lookup, user lookup, and the role-mapping
// delete whose body carries the role names.
type recordingKeycloak struct {
	mu      sync.Mutex
	removed []string
}

func (k *recordingKeycloak) removedRoles() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := append([]string(nil), k.removed...)
	sort.Strings(out)
	return out
}

func newRecordingKeycloak(t *testing.T) (*httptest.Server, *recordingKeycloak) {
	t.Helper()
	k := &recordingKeycloak{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 300})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/clients"):
			cid := r.URL.Query().Get("clientId")
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "uuid-" + cid, "clientId": cid}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/roles/")+len("/roles/"):]
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "role-" + name, "name": name})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users"):
			uname := r.URL.Query().Get("username")
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "user-" + uname, "username": uname}})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/role-mappings/clients/"):
			body, _ := io.ReadAll(r.Body)
			var roles []struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(body, &roles)
			k.mu.Lock()
			for _, role := range roles {
				k.removed = append(k.removed, role.Name)
			}
			k.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, k
}

// GTW-ACC-15.02: a genuine last-binding delete whose reads are consistently empty
// must revoke BOTH client roles after the confirming read. A recording Keycloak
// proves both openshell-admin and openshell-user are removed (GTW-ACC-14), not
// merely that the revoke path was entered.
// Verifies: GTW-ACC-15.02
func TestHandle_DeleteRevokesAfterConsistentEmptyConfirmation(t *testing.T) {
	conn, gateways, bindings := newRecordingGatewayAndRoleBindingConn(t)
	gateways.setGateway(&pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "team-gateway",
		ClusterId: "cluster-1",
	})
	kcServer, kc := newRecordingKeycloak(t)
	r := NewRoleBindingReconciler(keycloak.NewClient(kcServer.URL, "hypershell", "id", "secret"), conn, "cluster-1")
	bindings.setResults(listResult{}, listResult{}) // both reads empty

	err := r.Handle(context.Background(), deleteRoleBindingEvent("gw-1", "user-1"))
	if err != nil {
		t.Fatalf("a confirmed last-binding delete must revoke without error, got %v", err)
	}
	if got, want := kc.removedRoles(), []string{"openshell-admin", "openshell-user"}; !equalStrings(got, want) {
		t.Fatalf("removed roles = %v, want %v (both client roles revoked)", got, want)
	}
	if got := len(bindings.snapshot()); got != 2 {
		t.Fatalf("ListRoleBindings calls = %d, want 2 (first + confirming read)", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// GTW-ACC-15.03: when the computed desired set is non-empty on a delete (a
// surviving viewer binding still backs openshell-user), the guard does not fire:
// no confirming read is issued and the reconcile proceeds straight to Keycloak.
// Verifies: GTW-ACC-15.03
func TestHandle_DeleteWithSurvivingBindingSkipsConfirmation(t *testing.T) {
	r, bindings := deleteGuardReconciler(t)
	// The deleted owner binding is excluded; a surviving viewer binding keeps
	// openshell-user desired, so desired is non-empty.
	bindings.setResults(listResult{items: []*pb.RoleBinding{roleBinding("rb-viewer", "gateway:viewer")}})

	err := r.Handle(context.Background(), deleteRoleBindingEvent("gw-1", "user-1"))
	if err == nil || !strings.Contains(err.Error(), "remove keycloak role") {
		t.Fatalf("error = %v, want the reconcile to reach Keycloak (non-empty desired, no confirmation)", err)
	}
	if got := len(bindings.snapshot()); got != 1 {
		t.Fatalf("ListRoleBindings calls = %d, want 1 (no confirming read for a non-empty desired set)", got)
	}
}

// GTW-ACC-15.04: a confirming read that errors blocks the revoke and requeues.
// Verifies: GTW-ACC-15.04
func TestHandle_DeleteRequeuesWhenConfirmingReadErrors(t *testing.T) {
	r, bindings := deleteGuardReconciler(t)
	bindings.setResults(
		listResult{}, // first read: empty success
		listResult{err: status.Error(codes.Unavailable, "transient")}, // confirming read errors
	)

	err := r.Handle(context.Background(), deleteRoleBindingEvent("gw-1", "user-1"))
	if err == nil {
		t.Fatal("a confirming-read error must requeue with an error, got nil")
	}
	if strings.Contains(err.Error(), "remove keycloak role") {
		t.Fatalf("guard must requeue before any revoke, but a Keycloak removal was attempted: %v", err)
	}
	if !strings.Contains(err.Error(), "confirm absence") {
		t.Fatalf("error = %v, want a requeue citing the failed confirming read", err)
	}
	if got := len(bindings.snapshot()); got != 2 {
		t.Fatalf("ListRoleBindings calls = %d, want 2 (first + confirming read)", got)
	}
}

// The confirming read MUST apply the same deleted-binding exclusion as the first
// read (spec GTW-ACC-15 edge case: the deleted binding may still be visible due to
// a soft-delete race). Here the confirming read returns only the binding being
// deleted; excluded, it is a consistent empty set, so the revoke proceeds. Were
// excludeID dropped from the confirming read, the owner binding would be seen as
// surviving and the delete would wrongly requeue instead of revoking.
// Verifies: GTW-ACC-15.02
func TestHandle_ConfirmingReadExcludesDeletedBinding(t *testing.T) {
	r, bindings := deleteGuardReconciler(t)
	// First read empty; confirming read still shows the soft-deleted owner binding
	// (same id, rb-1) that the event is deleting.
	bindings.setResults(
		listResult{},
		listResult{items: []*pb.RoleBinding{roleBinding("rb-1", "gateway:owner")}},
	)

	err := r.Handle(context.Background(), deleteRoleBindingEvent("gw-1", "user-1"))
	if err == nil || !strings.Contains(err.Error(), "remove keycloak role") {
		t.Fatalf("error = %v, want the revoke to proceed once the deleted binding is excluded from the confirming read", err)
	}
	if strings.Contains(err.Error(), "confirming read found") {
		t.Fatalf("confirming read must exclude the deleted binding, but it was counted as surviving: %v", err)
	}
	if got := len(bindings.snapshot()); got != 2 {
		t.Fatalf("ListRoleBindings calls = %d, want 2 (first + confirming read)", got)
	}
}
