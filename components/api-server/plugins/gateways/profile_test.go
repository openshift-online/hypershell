package gateways

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	rherrors "github.com/openshift-online/rh-trex-ai/pkg/errors"
)

// noopGatewayEventService satisfies services.EventService for unit tests.
type noopGatewayEventService struct{}

func (noopGatewayEventService) Get(_ context.Context, _ string) (*api.Event, *rherrors.ServiceError) {
	return nil, nil
}
func (noopGatewayEventService) Create(_ context.Context, ev *api.Event) (*api.Event, *rherrors.ServiceError) {
	return ev, nil
}
func (noopGatewayEventService) Replace(_ context.Context, ev *api.Event) (*api.Event, *rherrors.ServiceError) {
	return ev, nil
}
func (noopGatewayEventService) Delete(_ context.Context, _ string) *rherrors.ServiceError { return nil }
func (noopGatewayEventService) All(_ context.Context) (api.EventList, *rherrors.ServiceError) {
	return nil, nil
}
func (noopGatewayEventService) FindByIDs(_ context.Context, _ []string) (api.EventList, *rherrors.ServiceError) {
	return nil, nil
}
func (noopGatewayEventService) FindUnreconciled(_ context.Context, _ time.Duration) (api.EventList, *rherrors.ServiceError) {
	return nil, nil
}
func (noopGatewayEventService) FindBySourceAndType(_ context.Context, _ string, _ api.EventType) (api.EventList, *rherrors.ServiceError) {
	return nil, nil
}

// settingPlacement is a PlacementResolver that assigns a fixed database_id so
// Create reaches the profile-resolution logic under test.
type settingPlacement struct{ databaseID string }

func (p settingPlacement) Resolve(_ context.Context, gw *Gateway) error {
	gw.DatabaseId = p.databaseID
	return nil
}

// fakeProfileResolver is a configurable ProfileResolver double.
type fakeProfileResolver struct {
	clusterDefault    string
	clusterDefaultErr error
	exists            bool
	existsErr         error

	clusterDefaultCalls int
}

func (f *fakeProfileResolver) ClusterDefaultProfileID(_ context.Context, _ string) (string, error) {
	f.clusterDefaultCalls++
	return f.clusterDefault, f.clusterDefaultErr
}

func (f *fakeProfileResolver) ProfileExists(_ context.Context, _ string) (bool, error) {
	return f.exists, f.existsErr
}

func (f *fakeProfileResolver) ClusterExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func newProfileTestService(profiles ProfileResolver) *sqlGatewayService {
	return &sqlGatewayService{
		gatewayDao: NewMockGatewayDao(),
		events:     noopGatewayEventService{},
		placement:  settingPlacement{databaseID: "db-assigned"},
		profiles:   profiles,
	}
}

// TestGatewayServiceProfileResolution covers the service-level profile
// resolution in Create: a client value wins; otherwise the cluster default is
// used; an empty result is rejected; and a nonexistent profile is rejected.
func TestGatewayServiceProfileResolution(t *testing.T) {
	t.Run("client-supplied profile wins over cluster default", func(t *testing.T) {
		resolver := &fakeProfileResolver{clusterDefault: "cluster-default", exists: true}
		svc := newProfileTestService(resolver)

		gw := &Gateway{Name: "gw", ClusterId: "c-1", ProfileId: "client-profile"}
		created, svcErr := svc.Create(context.Background(), gw)
		if svcErr != nil {
			t.Fatalf("Create() unexpected error: %v", svcErr)
		}
		if created.ProfileId != "client-profile" {
			t.Fatalf("ProfileId = %q, want client-supplied %q", created.ProfileId, "client-profile")
		}
		if resolver.clusterDefaultCalls != 0 {
			t.Fatalf("cluster default consulted %d times, want 0 when client supplies a profile", resolver.clusterDefaultCalls)
		}
	})

	t.Run("falls back to cluster default when none supplied", func(t *testing.T) {
		resolver := &fakeProfileResolver{clusterDefault: "cluster-default", exists: true}
		svc := newProfileTestService(resolver)

		gw := &Gateway{Name: "gw", ClusterId: "c-1"}
		created, svcErr := svc.Create(context.Background(), gw)
		if svcErr != nil {
			t.Fatalf("Create() unexpected error: %v", svcErr)
		}
		if created.ProfileId != "cluster-default" {
			t.Fatalf("ProfileId = %q, want cluster default %q", created.ProfileId, "cluster-default")
		}
		if resolver.clusterDefaultCalls != 1 {
			t.Fatalf("cluster default consulted %d times, want 1", resolver.clusterDefaultCalls)
		}
	})

	t.Run("rejects when neither client nor cluster supplies a profile", func(t *testing.T) {
		resolver := &fakeProfileResolver{clusterDefault: "", exists: true}
		svc := newProfileTestService(resolver)

		_, svcErr := svc.Create(context.Background(), &Gateway{Name: "gw", ClusterId: "c-1"})
		if svcErr == nil {
			t.Fatal("Create() = nil error, want HTTP 400 when no profile is resolvable")
		}
		if svcErr.HttpCode != http.StatusBadRequest {
			t.Fatalf("Create() HttpCode = %d, want 400", svcErr.HttpCode)
		}
	})

	t.Run("rejects a nonexistent profile on create", func(t *testing.T) {
		resolver := &fakeProfileResolver{exists: false}
		svc := newProfileTestService(resolver)

		_, svcErr := svc.Create(context.Background(), &Gateway{Name: "gw", ClusterId: "c-1", ProfileId: "ghost"})
		if svcErr == nil {
			t.Fatal("Create() = nil error, want HTTP 400 for a nonexistent profile")
		}
		if svcErr.HttpCode != http.StatusBadRequest {
			t.Fatalf("Create() HttpCode = %d, want 400", svcErr.HttpCode)
		}
	})
}

// fakeGatewayService implements GatewayService for handler-level PATCH tests.
type fakeGatewayService struct {
	gateway       *Gateway
	getErr        *rherrors.ServiceError
	profileExists bool
	profileErr    *rherrors.ServiceError
	clusterExists bool
	clusterErr    *rherrors.ServiceError
	replaceErr    *rherrors.ServiceError
	replaced      *Gateway
}

func (f *fakeGatewayService) Get(_ context.Context, _ string) (*Gateway, *rherrors.ServiceError) {
	return f.gateway, f.getErr
}
func (f *fakeGatewayService) GetUnscoped(_ context.Context, _ string) (*Gateway, *rherrors.ServiceError) {
	return f.gateway, f.getErr
}
func (f *fakeGatewayService) Create(_ context.Context, g *Gateway) (*Gateway, *rherrors.ServiceError) {
	return g, nil
}
func (f *fakeGatewayService) Replace(_ context.Context, g *Gateway) (*Gateway, *rherrors.ServiceError) {
	f.replaced = g
	return g, f.replaceErr
}
func (f *fakeGatewayService) Delete(_ context.Context, _ string) *rherrors.ServiceError { return nil }
func (f *fakeGatewayService) All(_ context.Context) (GatewayList, *rherrors.ServiceError) {
	return nil, nil
}
func (f *fakeGatewayService) AdjustActiveSandboxCount(_ context.Context, _ string, _ int) (int, *rherrors.ServiceError) {
	return 0, nil
}
func (f *fakeGatewayService) SetActiveSandboxCount(_ context.Context, _ string, _ int) (int, *rherrors.ServiceError) {
	return 0, nil
}
func (f *fakeGatewayService) FindByIDs(_ context.Context, _ []string) (GatewayList, *rherrors.ServiceError) {
	return nil, nil
}
func (f *fakeGatewayService) ProfileExists(_ context.Context, _ string) (bool, *rherrors.ServiceError) {
	return f.profileExists, f.profileErr
}
func (f *fakeGatewayService) ClusterExists(_ context.Context, _ string) (bool, *rherrors.ServiceError) {
	return f.clusterExists, f.clusterErr
}
func (f *fakeGatewayService) OnUpsert(_ context.Context, _ string) error { return nil }
func (f *fakeGatewayService) OnDelete(_ context.Context, _ string) error { return nil }

// doProfilePatch drives the PATCH handler through a gorilla mux router so
// mux.Vars are populated, and returns the recorded response.
func doProfilePatch(h *gatewayHandler, gatewayID string, body interface{}) *http.Response {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/gateways/"+gatewayID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")

	router := mux.NewRouter()
	router.HandleFunc("/gateways/{id}", h.Patch).Methods(http.MethodPatch)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr.Result()
}

func gatewayWithProfileID(id, profileID string) *Gateway {
	gw := &Gateway{Name: "gw-" + id, ProfileId: profileID}
	gw.ID = id
	return gw
}

func TestGatewayPatchProfileID_ClearRejected(t *testing.T) {
	svc := &fakeGatewayService{gateway: gatewayWithProfileID("gw-1", "current-profile"), profileExists: true, clusterExists: true}
	h := &gatewayHandler{gateway: svc}

	resp := doProfilePatch(h, "gw-1", map[string]interface{}{"profile_id": ""})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH profile_id=%q returned %d, want 400", "", resp.StatusCode)
	}
	if svc.replaced != nil {
		t.Fatal("Replace() must not be called when clearing the profile is rejected")
	}
}

func TestGatewayPatchProfileID_NonexistentRejected(t *testing.T) {
	svc := &fakeGatewayService{gateway: gatewayWithProfileID("gw-1", "current-profile"), profileExists: false, clusterExists: true}
	h := &gatewayHandler{gateway: svc}

	resp := doProfilePatch(h, "gw-1", map[string]interface{}{"profile_id": "nonexistent-id"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH profile_id=nonexistent returned %d, want 400", resp.StatusCode)
	}
	if svc.replaced != nil {
		t.Fatal("Replace() must not be called when the target profile does not exist")
	}
}

func TestGatewayPatchProfileID_ReassignSucceeds(t *testing.T) {
	svc := &fakeGatewayService{gateway: gatewayWithProfileID("gw-1", "old-profile"), profileExists: true, clusterExists: true}
	h := &gatewayHandler{gateway: svc}

	resp := doProfilePatch(h, "gw-1", map[string]interface{}{"profile_id": "new-profile"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH profile_id=new-profile returned %d, want 200", resp.StatusCode)
	}
	if svc.replaced == nil {
		t.Fatal("Replace() was not called")
	}
	if svc.replaced.ProfileId != "new-profile" {
		t.Fatalf("replaced gateway ProfileId = %q, want %q", svc.replaced.ProfileId, "new-profile")
	}
}

// TestGatewayServiceClusterValidation covers the service-level cluster_id
// validation in Create: a nonexistent cluster_id is rejected with 400, and
// an existing cluster_id passes.
func TestGatewayServiceClusterValidation(t *testing.T) {
	t.Run("rejects a nonexistent cluster_id on create", func(t *testing.T) {
		resolver := &fakeProfileResolver{exists: true}
		svc := newProfileTestService(&clusterRejectingResolver{fakeProfileResolver: resolver})

		_, svcErr := svc.Create(context.Background(), &Gateway{Name: "gw", ClusterId: "ghost-cluster", ProfileId: "p"})
		if svcErr == nil {
			t.Fatal("Create() = nil error, want HTTP 400 for a nonexistent cluster_id")
		}
		if svcErr.HttpCode != http.StatusBadRequest {
			t.Fatalf("Create() HttpCode = %d, want 400", svcErr.HttpCode)
		}
	})

	t.Run("accepts a valid cluster_id on create", func(t *testing.T) {
		resolver := &fakeProfileResolver{exists: true}
		svc := newProfileTestService(resolver)

		gw := &Gateway{Name: "gw", ClusterId: "real-cluster-id", ProfileId: "p"}
		created, svcErr := svc.Create(context.Background(), gw)
		if svcErr != nil {
			t.Fatalf("Create() unexpected error: %v", svcErr)
		}
		if created.ClusterId != "real-cluster-id" {
			t.Fatalf("ClusterId = %q, want %q", created.ClusterId, "real-cluster-id")
		}
	})
}

// clusterRejectingResolver wraps fakeProfileResolver and rejects all cluster lookups.
type clusterRejectingResolver struct {
	*fakeProfileResolver
}

func (r *clusterRejectingResolver) ClusterExists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// TestGatewayPatchClusterID_NonexistentRejected verifies the PATCH handler rejects
// a cluster_id that does not exist.
func TestGatewayPatchClusterID_NonexistentRejected(t *testing.T) {
	svc := &fakeGatewayService{
		gateway:       gatewayWithProfileID("gw-1", "current-profile"),
		profileExists: true,
		clusterExists: false,
	}
	h := &gatewayHandler{gateway: svc}

	resp := doProfilePatch(h, "gw-1", map[string]interface{}{"cluster_id": "ghost-cluster"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH cluster_id=ghost returned %d, want 400", resp.StatusCode)
	}
	if svc.replaced != nil {
		t.Fatal("Replace() must not be called when the cluster does not exist")
	}
}

// TestGatewayPatchClusterID_ClearRejected verifies the PATCH handler rejects
// clearing cluster_id with an empty string.
func TestGatewayPatchClusterID_ClearRejected(t *testing.T) {
	svc := &fakeGatewayService{
		gateway:       gatewayWithProfileID("gw-1", "current-profile"),
		profileExists: true,
		clusterExists: true,
	}
	h := &gatewayHandler{gateway: svc}

	resp := doProfilePatch(h, "gw-1", map[string]interface{}{"cluster_id": ""})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH cluster_id=%q returned %d, want 400", "", resp.StatusCode)
	}
	if svc.replaced != nil {
		t.Fatal("Replace() must not be called when clearing cluster_id is rejected")
	}
}

// TestGatewayPatchClusterID_ReassignSucceeds verifies the PATCH handler accepts
// updating cluster_id to a cluster that exists.
func TestGatewayPatchClusterID_ReassignSucceeds(t *testing.T) {
	svc := &fakeGatewayService{
		gateway:       gatewayWithProfileID("gw-1", "current-profile"),
		profileExists: true,
		clusterExists: true,
	}
	h := &gatewayHandler{gateway: svc}

	resp := doProfilePatch(h, "gw-1", map[string]interface{}{"cluster_id": "new-cluster-id"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH cluster_id=new-cluster-id returned %d, want 200", resp.StatusCode)
	}
	if svc.replaced == nil {
		t.Fatal("Replace() was not called")
	}
	if svc.replaced.ClusterId != "new-cluster-id" {
		t.Fatalf("replaced gateway ClusterId = %q, want %q", svc.replaced.ClusterId, "new-cluster-id")
	}
}
