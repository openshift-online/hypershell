package reconciler

import (
	"context"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// importFakeGatewayClient records the Gateway writes the import writer issues and serves a
// canned owned list. Only the methods the writer uses are implemented; the rest are
// satisfied by the embedded interface.
type importFakeGatewayClient struct {
	pb.GatewayServiceClient
	owned          []*pb.Gateway
	lastListRepo   string
	lastListMDRepo string
	created        []*pb.CreateGatewayRequest
	updated        []*pb.UpdateGatewayRequest
	deleted        []string
}

func (f *importFakeGatewayClient) ListGateways(ctx context.Context, in *pb.ListGatewaysRequest, _ ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
	f.lastListRepo = in.GetRepositoryId()
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if vals := md.Get(importRepositoryMetadataKey); len(vals) > 0 {
			f.lastListMDRepo = vals[0]
		}
	}
	return &pb.ListGatewaysResponse{Items: f.owned}, nil
}

func (f *importFakeGatewayClient) CreateGateway(_ context.Context, in *pb.CreateGatewayRequest, _ ...grpc.CallOption) (*pb.CreateGatewayResponse, error) {
	f.created = append(f.created, in)
	return &pb.CreateGatewayResponse{}, nil
}

func (f *importFakeGatewayClient) UpdateGateway(_ context.Context, in *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
	f.updated = append(f.updated, in)
	return &pb.UpdateGatewayResponse{}, nil
}

func (f *importFakeGatewayClient) DeleteGateway(_ context.Context, in *pb.DeleteGatewayRequest, _ ...grpc.CallOption) (*pb.DeleteGatewayResponse, error) {
	f.deleted = append(f.deleted, in.GetId())
	return &pb.DeleteGatewayResponse{}, nil
}

type fakeAgentRuntimeClient struct {
	pb.AgentRuntimeServiceClient
	owned        []*pb.AgentRuntime
	lastListRepo string
	created      []*pb.CreateAgentRuntimeRequest
	updated      []*pb.UpdateAgentRuntimeRequest
	deleted      []string
}

func (f *fakeAgentRuntimeClient) ListAgentRuntimes(_ context.Context, in *pb.ListAgentRuntimesRequest, _ ...grpc.CallOption) (*pb.ListAgentRuntimesResponse, error) {
	f.lastListRepo = in.GetRepositoryId()
	return &pb.ListAgentRuntimesResponse{Items: f.owned}, nil
}

func (f *fakeAgentRuntimeClient) CreateAgentRuntime(_ context.Context, in *pb.CreateAgentRuntimeRequest, _ ...grpc.CallOption) (*pb.CreateAgentRuntimeResponse, error) {
	f.created = append(f.created, in)
	return &pb.CreateAgentRuntimeResponse{}, nil
}

func (f *fakeAgentRuntimeClient) UpdateAgentRuntime(_ context.Context, in *pb.UpdateAgentRuntimeRequest, _ ...grpc.CallOption) (*pb.UpdateAgentRuntimeResponse, error) {
	f.updated = append(f.updated, in)
	return &pb.UpdateAgentRuntimeResponse{}, nil
}

func (f *fakeAgentRuntimeClient) DeleteAgentRuntime(_ context.Context, in *pb.DeleteAgentRuntimeRequest, _ ...grpc.CallOption) (*pb.DeleteAgentRuntimeResponse, error) {
	f.deleted = append(f.deleted, in.GetId())
	return &pb.DeleteAgentRuntimeResponse{}, nil
}

func importGw(id, name, repoID, clusterID string) *pb.Gateway {
	return &pb.Gateway{Metadata: &pb.ObjectReference{Id: id}, Name: name, RepositoryId: &repoID, ClusterId: clusterID}
}

func TestUpsertGateway_CreatesWhenAbsent(t *testing.T) {
	gwc := &importFakeGatewayClient{}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	res := RenderedResource{Kind: "Gateway", Name: "gw-1", Spec: map[string]interface{}{"cluster_id": "c1"}}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(gwc.created) != 1 || gwc.created[0].GetName() != "gw-1" || gwc.created[0].GetClusterId() != "c1" {
		t.Fatalf("created = %+v, want one create of gw-1/c1", gwc.created)
	}
	// The owned-list read carries both the repository_id filter and the import
	// ownership binding, which is what exempts it from the cluster caller binding.
	if gwc.lastListRepo != "repo-1" {
		t.Errorf("list repository_id filter = %q, want repo-1", gwc.lastListRepo)
	}
	if gwc.lastListMDRepo != "repo-1" {
		t.Errorf("list import binding metadata = %q, want repo-1", gwc.lastListMDRepo)
	}
}

func TestUpsertGateway_SkipsWhenUnchanged(t *testing.T) {
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("g1", "gw-1", "repo-1", "c1")}}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	res := RenderedResource{Kind: "Gateway", Name: "gw-1", Spec: map[string]interface{}{"cluster_id": "c1"}}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(gwc.created) != 0 || len(gwc.updated) != 0 {
		t.Fatalf("unchanged gateway caused writes: created=%+v updated=%+v", gwc.created, gwc.updated)
	}
}

// An import is a non-privileged writer whose only writable surface is name + placement,
// both create-only (PLAT-DATA-23 / CTRL-CORE-15). Re-importing an existing gateway with a
// changed cluster_id does not reassign it: name is the match key and cluster_id is not
// updatable by an import, so the pass is a no-op.
func TestUpsertGateway_ReimportWithChangedClusterIsNoOp(t *testing.T) {
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("g1", "gw-1", "repo-1", "c1")}}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	res := RenderedResource{Kind: "Gateway", Name: "gw-1", Spec: map[string]interface{}{"cluster_id": "c2"}}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(gwc.created) != 0 || len(gwc.updated) != 0 {
		t.Fatalf("re-import of an existing gateway wrote: created=%+v updated=%+v", gwc.created, gwc.updated)
	}
}

// A .hypershell Gateway manifest may declare only name + placement; a platform-managed
// field fails that gateway's import with a clear error naming the field, and no create or
// update is sent (CTRL-CORE-15.01; PLAT-DATA-23).
func TestUpsertGateway_RejectsPlatformManagedField(t *testing.T) {
	for _, field := range []string{"image", "oidc", "route", "tls_mode", "service_type", "external_dns", "supervisor_image", "server_dns_names", "credential_driver"} {
		gwc := &importFakeGatewayClient{}
		w := &grpcImportWriter{gateways: gwc, pageSize: 100}
		res := RenderedResource{Kind: "Gateway", Name: "gw-platform", Spec: map[string]interface{}{
			"cluster_id": "c1",
			field:        "some-value",
		}}

		err := w.Upsert(context.Background(), "repo-1", res)
		if err == nil {
			t.Fatalf("field %q: Upsert returned nil error; want a platform-managed rejection", field)
		}
		if !strings.Contains(err.Error(), field) || !strings.Contains(err.Error(), "platform-managed") {
			t.Errorf("field %q: error = %q, want it to name the field and 'platform-managed'", field, err)
		}
		if len(gwc.created) != 0 || len(gwc.updated) != 0 {
			t.Errorf("field %q: a rejected manifest still wrote: created=%+v updated=%+v", field, gwc.created, gwc.updated)
		}
	}
}

// Verifies: CTRL-CORE-14.04
func TestUpsertGateway_CreatesPinnedWithoutPlacement(t *testing.T) {
	gwc := &importFakeGatewayClient{}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	res := RenderedResource{Kind: "Gateway", Name: "gw-1", Spec: map[string]interface{}{"cluster_id": "c1"}}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(gwc.created) != 1 {
		t.Fatalf("created = %+v, want one create", gwc.created)
	}
	// A pinned gateway sends cluster_id and NO placement (exactly-one-of).
	if gwc.created[0].GetClusterId() != "c1" {
		t.Errorf("cluster_id = %q, want c1", gwc.created[0].GetClusterId())
	}
	if gwc.created[0].GetPlacement() != nil {
		t.Errorf("placement = %+v, want nil for a pinned gateway", gwc.created[0].GetPlacement())
	}
}

// Verifies: CTRL-CORE-14.02
func TestUpsertGateway_CreatesLateBoundWithCapabilitySelector(t *testing.T) {
	gwc := &importFakeGatewayClient{}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	// A declarative late-bound gateway: a capability selector and NO cluster_id.
	res := RenderedResource{Kind: "Gateway", Name: "agents-gateway", Spec: map[string]interface{}{
		"placement": map[string]interface{}{
			"selector": []interface{}{"network/public", "provider/ibm"},
		},
	}}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(gwc.created) != 1 {
		t.Fatalf("created = %+v, want one create", gwc.created)
	}
	created := gwc.created[0]
	if created.GetClusterId() != "" {
		t.Errorf("cluster_id = %q, want empty for a late-bound gateway", created.GetClusterId())
	}
	sel := created.GetPlacement().GetCapabilitySelector()
	if sel == nil {
		t.Fatalf("placement = %+v, want a capability_selector", created.GetPlacement())
	}
	if got := sel.GetSelector(); len(got) != 2 || got[0] != "network/public" || got[1] != "provider/ibm" {
		t.Errorf("selector = %v, want [network/public provider/ibm]", got)
	}
}

// Verifies: CTRL-CORE-14.03
func TestUpsertGateway_CreatesLateBoundWithLocalKind(t *testing.T) {
	gwc := &importFakeGatewayClient{}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	res := RenderedResource{Kind: "Gateway", Name: "dev-gateway", Spec: map[string]interface{}{
		"placement": map[string]interface{}{"mode": "local-kind"},
	}}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(gwc.created) != 1 {
		t.Fatalf("created = %+v, want one create", gwc.created)
	}
	created := gwc.created[0]
	if created.GetClusterId() != "" {
		t.Errorf("cluster_id = %q, want empty for a local-kind gateway", created.GetClusterId())
	}
	if created.GetPlacement().GetLocalKind() == nil {
		t.Errorf("placement = %+v, want local_kind set", created.GetPlacement())
	}
}

// Verifies: CTRL-CORE-14.05
func TestUpsertGateway_LateBoundNoOpAfterServerAssignsCluster(t *testing.T) {
	// The server resolved the late-bound gateway to cluster c9 after the first
	// import. On re-import the rendered manifest still declares only a placement and
	// no cluster_id; the writer must NOT treat the server-assigned cluster_id as
	// drift, so the pass is a no-op.
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("g1", "agents-gateway", "repo-1", "c9")}}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	res := RenderedResource{Kind: "Gateway", Name: "agents-gateway", Spec: map[string]interface{}{
		"placement": map[string]interface{}{
			"selector": []interface{}{"network/public", "provider/ibm"},
		},
	}}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(gwc.created) != 0 || len(gwc.updated) != 0 {
		t.Fatalf("late-bound re-import churned: created=%+v updated=%+v", gwc.created, gwc.updated)
	}
}

// Verifies: CTRL-CORE-14.01
func TestUpsertGateway_MalformedPlacementReportsError(t *testing.T) {
	gwc := &importFakeGatewayClient{}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	// A placement block that is present but declares neither a non-empty selector nor
	// mode: local-kind must fail with a clear error, not silently create a gateway
	// with neither cluster_id nor placement.
	res := RenderedResource{Kind: "Gateway", Name: "gw-bad", Spec: map[string]interface{}{
		"placement": map[string]interface{}{"mode": "nonsense"},
	}}

	err := w.Upsert(context.Background(), "repo-1", res)
	if err == nil {
		t.Fatalf("Upsert of a malformed placement returned nil error; want a diagnostic")
	}
	if len(gwc.created) != 0 || len(gwc.updated) != 0 {
		t.Fatalf("malformed placement still wrote: created=%+v updated=%+v", gwc.created, gwc.updated)
	}
}

func TestListOwned_FiltersGatewaysAndAgentRuntimesByRepository(t *testing.T) {
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{
		importGw("g1", "gw-1", "repo-1", "c1"),
		importGw("g2", "gw-2", "repo-2", "c1"), // another repository; must be excluded
	}}
	arRepo1 := "repo-1"
	arRepo2 := "repo-2"
	arc := &fakeAgentRuntimeClient{owned: []*pb.AgentRuntime{
		{Metadata: &pb.ObjectReference{Id: "a1"}, Name: "ar-1", RepositoryId: &arRepo1},
		{Metadata: &pb.ObjectReference{Id: "a2"}, Name: "ar-2", RepositoryId: &arRepo2},
	}}
	w := &grpcImportWriter{gateways: gwc, agentRuntimes: arc, pageSize: 100}

	// Exercise the two new owned-list readers directly: ListOwned additionally touches
	// the five other kinds' clients, which this focused test does not wire.
	gws, err := w.ownedGateways(context.Background(), "repo-1")
	if err != nil {
		t.Fatalf("ownedGateways error = %v", err)
	}
	if len(gws) != 1 || gws[0].GetName() != "gw-1" {
		t.Fatalf("ownedGateways = %+v, want only gw-1 (repo-2's gateway must be excluded)", gws)
	}
	if gwc.lastListRepo != "repo-1" {
		t.Errorf("gateway list repository_id filter = %q, want repo-1", gwc.lastListRepo)
	}

	ars, err := w.ownedAgentRuntimes(context.Background(), "repo-1")
	if err != nil {
		t.Fatalf("ownedAgentRuntimes error = %v", err)
	}
	if len(ars) != 1 || ars[0].GetName() != "ar-1" {
		t.Fatalf("ownedAgentRuntimes = %+v, want only ar-1 (repo-2's runtime must be excluded)", ars)
	}
	if arc.lastListRepo != "repo-1" {
		t.Errorf("agent runtime list repository_id filter = %q, want repo-1", arc.lastListRepo)
	}
}

func TestDelete_GatewayAndAgentRuntime(t *testing.T) {
	gwc := &importFakeGatewayClient{}
	arc := &fakeAgentRuntimeClient{}
	w := &grpcImportWriter{gateways: gwc, agentRuntimes: arc, pageSize: 100}

	if err := w.Delete(context.Background(), "repo-1", ResourceRef{Kind: "Gateway", Name: "gw-1", ID: "g1"}); err != nil {
		t.Fatalf("delete gateway error = %v", err)
	}
	if err := w.Delete(context.Background(), "repo-1", ResourceRef{Kind: "AgentRuntime", Name: "ar-1", ID: "a1"}); err != nil {
		t.Fatalf("delete agent runtime error = %v", err)
	}
	if len(gwc.deleted) != 1 || gwc.deleted[0] != "g1" {
		t.Fatalf("gateway deletes = %v, want [g1]", gwc.deleted)
	}
	if len(arc.deleted) != 1 || arc.deleted[0] != "a1" {
		t.Fatalf("agent runtime deletes = %v, want [a1]", arc.deleted)
	}
}

func TestUpsertAgentRuntime_CreatesAndUpdates(t *testing.T) {
	arc := &fakeAgentRuntimeClient{}
	w := &grpcImportWriter{agentRuntimes: arc, pageSize: 100}
	res := RenderedResource{Kind: "AgentRuntime", Name: "ar-1", Spec: map[string]interface{}{
		"gateway_id":            "gw-1",
		"sandbox_template_id":   "tmpl-1",
		"login_refresh_seconds": 900,
	}}
	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(arc.created) != 1 || arc.created[0].GetGatewayId() != "gw-1" || arc.created[0].GetLoginRefreshSeconds() != 900 {
		t.Fatalf("created = %+v, want one create of ar-1 with login_refresh_seconds=900", arc.created)
	}

	// Now with an existing runtime that differs, it updates.
	repo1 := "repo-1"
	arc2 := &fakeAgentRuntimeClient{owned: []*pb.AgentRuntime{
		{Metadata: &pb.ObjectReference{Id: "a1"}, Name: "ar-1", RepositoryId: &repo1, GatewayId: "gw-OLD", SandboxTemplateId: "tmpl-1"},
	}}
	w2 := &grpcImportWriter{agentRuntimes: arc2, pageSize: 100}
	if err := w2.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert (update) error = %v", err)
	}
	if len(arc2.updated) != 1 || arc2.updated[0].GetId() != "a1" || arc2.updated[0].GetGatewayId() != "gw-1" {
		t.Fatalf("updated = %+v, want one update of a1 -> gw-1", arc2.updated)
	}
}
