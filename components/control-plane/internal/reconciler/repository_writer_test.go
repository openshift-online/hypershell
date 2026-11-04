package reconciler

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"sigs.k8s.io/kustomize/kyaml/filesys"
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
	// createRepoID is stamped onto the gateways CreateGateway reflects into owned so a
	// same-pass by-name resolution (which filters by repository_id) can see them.
	createRepoID string
	createCount  int
	// listCalls counts ListGateways invocations so a test can assert a pass reads the
	// owned-gateway set once, not once per CITrust (FINDING 4).
	listCalls int
}

func (f *importFakeGatewayClient) ListGateways(ctx context.Context, in *pb.ListGatewaysRequest, _ ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
	f.listCalls++
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
	// Reflect the create in the owned set the way the server would, so a later read in
	// the SAME pass (for example a CITrust resolving this gateway by name) can see it.
	f.createCount++
	id := fmt.Sprintf("created-gw-%d", f.createCount)
	repoID := f.createRepoID
	f.owned = append(f.owned, &pb.Gateway{
		Metadata:     &pb.ObjectReference{Id: id},
		Name:         in.GetName(),
		RepositoryId: &repoID,
		ClusterId:    in.GetClusterId(),
	})
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

// Verifies: CTRL-CORE-14.02
// (the Kind local-development shape is the same capability-selector mapping;
// the retired local-kind mode scenario is recorded in specs/id-rename-map.md).
func TestUpsertGateway_CreatesLateBoundWithKindSelector(t *testing.T) {
	gwc := &importFakeGatewayClient{}
	w := &grpcImportWriter{gateways: gwc, pageSize: 100}
	res := RenderedResource{Kind: "Gateway", Name: "dev-gateway", Spec: map[string]interface{}{
		"placement": map[string]interface{}{
			"selector": []interface{}{"provider/kind"},
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
	if got := sel.GetSelector(); len(got) != 1 || got[0] != "provider/kind" {
		t.Errorf("selector = %v, want [provider/kind]", got)
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
	// A placement block that is present but declares no non-empty selector list must
	// fail with a clear error, not silently create a gateway with neither cluster_id
	// nor placement.
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

// ---- CITrust ----

// fakeCITrustClient records CITrust writes and serves a canned list, so the import
// writer's by-name resolution, idempotency, and per-Repository ownership are
// observable without a server.
type fakeCITrustClient struct {
	pb.CITrustServiceClient
	owned     []*pb.CITrust
	created   []*pb.CreateCITrustRequest
	updated   []*pb.UpdateCITrustRequest
	deleted   []string
	listCalls int
}

func (f *fakeCITrustClient) ListCITrusts(_ context.Context, _ *pb.ListCITrustsRequest, _ ...grpc.CallOption) (*pb.ListCITrustsResponse, error) {
	f.listCalls++
	return &pb.ListCITrustsResponse{Items: f.owned}, nil
}

func (f *fakeCITrustClient) CreateCITrust(_ context.Context, in *pb.CreateCITrustRequest, _ ...grpc.CallOption) (*pb.CreateCITrustResponse, error) {
	f.created = append(f.created, in)
	return &pb.CreateCITrustResponse{}, nil
}

func (f *fakeCITrustClient) UpdateCITrust(_ context.Context, in *pb.UpdateCITrustRequest, _ ...grpc.CallOption) (*pb.UpdateCITrustResponse, error) {
	f.updated = append(f.updated, in)
	return &pb.UpdateCITrustResponse{}, nil
}

func (f *fakeCITrustClient) DeleteCITrust(_ context.Context, in *pb.DeleteCITrustRequest, _ ...grpc.CallOption) (*pb.DeleteCITrustResponse, error) {
	f.deleted = append(f.deleted, in.GetId())
	return &pb.DeleteCITrustResponse{}, nil
}

// fakeCIProviderClient serves a canned CIProvider list for find-by-name resolution.
type fakeCIProviderClient struct {
	pb.CIProviderServiceClient
	items     []*pb.CIProvider
	listCalls int
}

func (f *fakeCIProviderClient) ListCIProviders(_ context.Context, _ *pb.ListCIProvidersRequest, _ ...grpc.CallOption) (*pb.ListCIProvidersResponse, error) {
	f.listCalls++
	return &pb.ListCIProvidersResponse{Items: f.items}, nil
}

func ciProvider(id, name string) *pb.CIProvider {
	return &pb.CIProvider{Metadata: &pb.ObjectReference{Id: id}, Name: name}
}

func importCITrust(id, name, repoID string) *pb.CITrust {
	rid := repoID
	return &pb.CITrust{Metadata: &pb.ObjectReference{Id: id}, Name: name, RepositoryId: &rid}
}

func citrustSpec() map[string]interface{} {
	return map[string]interface{}{
		"ci_provider_id": "github-actions",
		"gateway_id":     "agents-gateway",
		"role":           "openshell-user",
		"subject": map[string]interface{}{
			"repository":    "acme/repo",
			"ref_protected": true,
		},
	}
}

// Verifies: PLAT-CITR-10, CTRL-CORE-06
func TestUpsertCITrust_CreatesResolvingByName(t *testing.T) {
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(ctc.created) != 1 {
		t.Fatalf("created = %+v, want one create", ctc.created)
	}
	c := ctc.created[0]
	if c.GetCiProviderId() != "cp-id" {
		t.Errorf("ci_provider_id = %q, want resolved id cp-id", c.GetCiProviderId())
	}
	if c.GetGatewayId() != "gw-id" {
		t.Errorf("gateway_id = %q, want resolved id gw-id", c.GetGatewayId())
	}
	if c.GetName() != "ct-1" {
		t.Errorf("name = %q, want manifest metadata.name ct-1", c.GetName())
	}
	if c.GetRole() != "openshell-user" || c.GetSubjectRepository() != "acme/repo" || !c.GetSubjectRefProtected() {
		t.Errorf("subject/role not mapped: %+v", c)
	}
}

// A re-import of an unchanged CITrust issues no write (idempotent).
// Verifies: CTRL-CORE-06
func TestUpsertCITrust_SkipsWhenUnchanged(t *testing.T) {
	existing := importCITrust("ct-id", "ct-1", "repo-1")
	existing.CiProviderId = "cp-id"
	existing.GatewayId = "gw-id"
	existing.Role = "openshell-user"
	sr := "acme/repo"
	existing.SubjectRepository = &sr
	rp := true
	existing.SubjectRefProtected = &rp
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{owned: []*pb.CITrust{existing}}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(ctc.created) != 0 || len(ctc.updated) != 0 {
		t.Fatalf("unchanged CITrust caused writes: created=%+v updated=%+v", ctc.created, ctc.updated)
	}
}

func TestUpsertCITrust_UpdatesWhenRoleChanges(t *testing.T) {
	existing := importCITrust("ct-id", "ct-1", "repo-1")
	existing.CiProviderId = "cp-id"
	existing.GatewayId = "gw-id"
	existing.Role = "openshell-admin" // differs from the manifest's openshell-user
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{owned: []*pb.CITrust{existing}}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(ctc.updated) != 1 || ctc.updated[0].GetId() != "ct-id" || ctc.updated[0].GetRole() != "openshell-user" {
		t.Fatalf("updated = %+v, want one update of ct-id -> openshell-user", ctc.updated)
	}
}

// An import may not author a CITrust on a gateway the Repository does not own; name
// resolution is scoped to owned gateways, so the control plane never attempts a
// cross-tenant write (the api-server enforces the same gate server-side).
// Verifies: SEC-RBAC-25.02
func TestUpsertCITrust_RejectsGatewayNotOwned(t *testing.T) {
	// The Repository owns no gateway named agents-gateway (owned list empty).
	gwc := &importFakeGatewayClient{}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	err := w.Upsert(context.Background(), "repo-1", res)
	if err == nil {
		t.Fatal("Upsert expected error for a gateway the repository does not own, got nil")
	}
	if !strings.Contains(err.Error(), "agents-gateway") {
		t.Errorf("error = %v, want it to name the unresolved gateway", err)
	}
	if len(ctc.created) != 0 {
		t.Fatalf("a cross-tenant create was issued: %+v", ctc.created)
	}
}

func TestUpsertCITrust_RejectsUnknownCIProvider(t *testing.T) {
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{} // no providers registered
	ctc := &fakeCITrustClient{}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	err := w.Upsert(context.Background(), "repo-1", res)
	if err == nil {
		t.Fatal("Upsert expected error for an unknown ci_provider_id, got nil")
	}
	if len(ctc.created) != 0 {
		t.Fatalf("a create was issued despite unresolved ci_provider_id: %+v", ctc.created)
	}
}

// ci_provider_id and gateway_id are immutable. A same-named owned trust whose manifest
// now resolves to a DIFFERENT gateway must not be silently reconciled (subject/role
// updated against a stale FK, or a false no-op); it is reported as an immutable-field
// error that requires recreating the trust under a new name (FINDING 2). Before the fix
// the FKs were omitted from both the drift comparison and the update request, so a
// changed FK on an otherwise-matching trust was a silent no-op.
func TestUpsertCITrust_RejectsChangedImmutableGateway(t *testing.T) {
	// The owned trust already matches the manifest's role and subject, so before the fix
	// the upsert returned a silent no-op; its stored gateway_id, however, differs from
	// what the manifest's gateway name now resolves to.
	existing := importCITrust("ct-id", "ct-1", "repo-1")
	existing.CiProviderId = "cp-id"
	existing.GatewayId = "stale-gw-id" // manifest's agents-gateway now resolves to gw-id
	existing.Role = "openshell-user"
	sr := "acme/repo"
	existing.SubjectRepository = &sr
	rp := true
	existing.SubjectRefProtected = &rp
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{owned: []*pb.CITrust{existing}}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	err := w.Upsert(context.Background(), "repo-1", res)
	if err == nil {
		t.Fatal("expected an immutable-FK error for a changed gateway_id, got nil")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Errorf("error = %v, want it to explain the FK is immutable", err)
	}
	if len(ctc.updated) != 0 || len(ctc.created) != 0 {
		t.Fatalf("a write was issued despite a changed immutable FK: updated=%+v created=%+v", ctc.updated, ctc.created)
	}
}

// A desired name already taken by a CITrust this Repository does NOT own (another
// Repository, or a human/REST create) cannot be adopted: the create would 409 on every
// pass. The import reports a clear, non-retryable error instead of looping forever
// (FINDING 4). Before the fix only the repo's own owned set was consulted, so a foreign
// name fell through to the create branch and re-issued a doomed create each pass.
func TestUpsertCITrust_RejectsNameOwnedByAnotherOwner(t *testing.T) {
	foreign := importCITrust("other-id", "ct-1", "repo-2") // same name, different owner
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{owned: []*pb.CITrust{foreign}}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	err := w.Upsert(context.Background(), "repo-1", res)
	if err == nil {
		t.Fatal("expected an error importing a name owned by another owner, got nil")
	}
	if !strings.Contains(err.Error(), "not owned by this repository") {
		t.Errorf("error = %v, want it to explain the name cannot be adopted", err)
	}
	if len(ctc.created) != 0 {
		t.Fatalf("a create was issued despite a cross-owner name collision: %+v", ctc.created)
	}
}

// ownedCITrusts filters by repository_id, so prune is scoped per-Repository: a CITrust
// owned by another Repository or created directly over REST (no ownership mark) is not
// enumerated and therefore never pruned.
// Verifies: CTRL-CORE-06.01
func TestOwnedCITrusts_FilterByRepository(t *testing.T) {
	ctc := &fakeCITrustClient{owned: []*pb.CITrust{
		importCITrust("mine", "ct-mine", "repo-1"),
		importCITrust("other", "ct-other", "repo-2"),
		{Metadata: &pb.ObjectReference{Id: "rest"}, Name: "ct-rest"}, // REST-created, no ownership
	}}
	// Exercise the owned-list reader directly, mirroring the gateway/agent-runtime
	// focused test: ListOwned additionally touches the other kinds' clients this test
	// does not wire.
	w := &grpcImportWriter{ciTrusts: ctc, pageSize: 100}
	got, err := w.ownedCITrusts(context.Background(), "repo-1")
	if err != nil {
		t.Fatalf("ownedCITrusts error = %v", err)
	}
	if len(got) != 1 || got[0].GetName() != "ct-mine" {
		t.Fatalf("owned CITrusts = %+v, want only ct-mine (repo-2's and the REST-created trust must be excluded)", got)
	}
}

// TestDelete_CITrust confirms the Delete dispatch routes a CITrust prune to the
// DeleteCITrust RPC.
func TestDelete_CITrust(t *testing.T) {
	ctc := &fakeCITrustClient{}
	w := &grpcImportWriter{ciTrusts: ctc, pageSize: 100}
	if err := w.Delete(context.Background(), "repo-1", ResourceRef{Kind: "CITrust", Name: "ct-1", ID: "ct-id"}); err != nil {
		t.Fatalf("delete CITrust error = %v", err)
	}
	if len(ctc.deleted) != 1 || ctc.deleted[0] != "ct-id" {
		t.Fatalf("CITrust deletes = %v, want [ct-id]", ctc.deleted)
	}
}

// gatewayAndCITrustFS declares a fresh repository that owns a Gateway AND a CITrust
// referencing it by name - exactly the shape of .hypershell/base/platform.
func gatewayAndCITrustFS(t *testing.T) filesys.FileSystem {
	t.Helper()
	return inMemFS(t, map[string]string{
		"/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - r.yaml
`,
		"/r.yaml": `apiVersion: hypershell.redhat.io/v1
kind: CITrust
metadata:
  name: ct-1
spec:
  ci_provider_id: github-actions
  gateway_id: agents-gateway
  role: openshell-user
  subject:
    repository: acme/repo
    ref_protected: true
---
apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: agents-gateway
spec:
  placement:
    selector:
      - network/public
`,
	})
}

// TestUpsertAll_SinglePassImportsGatewayThenCITrust proves a fresh repo declaring a
// Gateway AND a CITrust referencing it imports in a SINGLE pass (FINDING 1):
// renderImportables orders the Gateway (a foreign-key target) before the CITrust even
// though the manifest lists the CITrust first and "CITrust" sorts before "Gateway"
// alphabetically, and the CITrust resolves the gateway created earlier in the same
// pass. Before the ordering fix the CITrust was upserted first and failed to resolve
// the not-yet-created gateway, needing a second reconcile.
func TestUpsertAll_SinglePassImportsGatewayThenCITrust(t *testing.T) {
	desired, err := renderImportables(gatewayAndCITrustFS(t), "/")
	if err != nil {
		t.Fatalf("renderImportables error = %v", err)
	}

	gwc := &importFakeGatewayClient{createRepoID: "repo-1"} // owns no gateway yet
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}

	errs := w.UpsertAll(context.Background(), "repo-1", desired)
	if len(errs) != 0 {
		t.Fatalf("single-pass import reported errors: %v", errs)
	}
	if len(gwc.created) != 1 || gwc.created[0].GetName() != "agents-gateway" {
		t.Fatalf("gateway creates = %+v, want one agents-gateway", gwc.created)
	}
	if len(ctc.created) != 1 {
		t.Fatalf("CITrust creates = %+v, want one (CITrust must be created in the same pass)", ctc.created)
	}
	if got := ctc.created[0].GetGatewayId(); got != "created-gw-1" {
		t.Errorf("CITrust gateway_id = %q, want it resolved to the gateway created earlier in the pass", got)
	}
}

// TestUpsertCITrust_UpdatesWhenSubjectRepositoryChanges proves a changed
// subject.repository reconciles via an update instead of being silently skipped
// (FINDING 2). Before the fix subject_repository was absent from both the drift
// comparison and UpdateCITrustRequest, so a changed value compared equal and no update
// was issued.
func TestUpsertCITrust_UpdatesWhenSubjectRepositoryChanges(t *testing.T) {
	existing := importCITrust("ct-id", "ct-1", "repo-1")
	existing.CiProviderId = "cp-id"
	existing.GatewayId = "gw-id"
	existing.Role = "openshell-user"
	oldRepo := "acme/old" // the manifest declares acme/repo
	existing.SubjectRepository = &oldRepo
	rp := true
	existing.SubjectRefProtected = &rp
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{owned: []*pb.CITrust{existing}}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if len(ctc.updated) != 1 {
		t.Fatalf("updated = %+v, want one update reconciling the changed subject.repository", ctc.updated)
	}
	if got := ctc.updated[0].GetSubjectRepository(); got != "acme/repo" {
		t.Errorf("updated subject_repository = %q, want acme/repo", got)
	}
}

// TestUpsertCITrust_UpdateSucceedsWhenReferencesResolveToSameIDs proves an update-only
// change (role) succeeds as long as the manifest's by-name references still resolve to
// the SAME immutable ci_provider_id/gateway_id the trust was created with. The FK
// immutability check compares ids, not names, so a CIProvider/Gateway rename that the
// manifest tracks is NOT treated as a changed FK and does not block the update
// (FINDING 2). Only a reference that resolves to a DIFFERENT id is rejected (see
// TestUpsertCITrust_RejectsChangedImmutableGateway).
func TestUpsertCITrust_UpdateSucceedsWhenReferencesResolveToSameIDs(t *testing.T) {
	existing := importCITrust("ct-id", "ct-1", "repo-1")
	existing.CiProviderId = "cp-id"
	existing.GatewayId = "gw-id"
	existing.Role = "openshell-admin" // differs from the manifest's openshell-user -> forces an update
	// The manifest's ci_provider_id ("github-actions") and gateway_id ("agents-gateway")
	// still resolve to the same immutable ids the trust was created with.
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{owned: []*pb.CITrust{existing}}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	res := RenderedResource{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()}

	if err := w.Upsert(context.Background(), "repo-1", res); err != nil {
		t.Fatalf("Upsert error = %v, want the update to succeed when references resolve to the same ids", err)
	}
	if len(ctc.updated) != 1 || ctc.updated[0].GetRole() != "openshell-user" {
		t.Fatalf("updated = %+v, want one update of ct-id -> openshell-user", ctc.updated)
	}
	if len(ctc.created) != 0 {
		t.Fatalf("a create was issued on an update-only change: %+v", ctc.created)
	}
}

// TestUpsertAll_SharesCITrustReadsAcrossMultipleTrusts proves a pass importing several
// CITrusts is O(all) not O(N*all): the owned-CITrust set, owned-Gateway set, and
// CIProvider registry are each read ONCE for the whole pass and shared across every
// CITrust upsert (FINDING 4), and the import remains correct (both trusts created).
// Before the fix each upsertCITrust re-ran all three scans.
func TestUpsertAll_SharesCITrustReadsAcrossMultipleTrusts(t *testing.T) {
	gwc := &importFakeGatewayClient{owned: []*pb.Gateway{importGw("gw-id", "agents-gateway", "repo-1", "c1")}}
	cpc := &fakeCIProviderClient{items: []*pb.CIProvider{ciProvider("cp-id", "github-actions")}}
	ctc := &fakeCITrustClient{}
	w := &grpcImportWriter{gateways: gwc, ciProviders: cpc, ciTrusts: ctc, pageSize: 100}
	desired := []RenderedResource{
		{Kind: "CITrust", Name: "ct-1", Spec: citrustSpec()},
		{Kind: "CITrust", Name: "ct-2", Spec: citrustSpec()},
	}

	errs := w.UpsertAll(context.Background(), "repo-1", desired)
	if len(errs) != 0 {
		t.Fatalf("UpsertAll reported errors: %v", errs)
	}
	if len(ctc.created) != 2 {
		t.Fatalf("CITrust creates = %d, want 2 (both trusts imported)", len(ctc.created))
	}
	if ctc.listCalls != 1 {
		t.Errorf("ListCITrusts calls = %d, want 1 per pass", ctc.listCalls)
	}
	if gwc.listCalls != 1 {
		t.Errorf("ListGateways calls = %d, want 1 per pass", gwc.listCalls)
	}
	if cpc.listCalls != 1 {
		t.Errorf("ListCIProviders calls = %d, want 1 per pass", cpc.listCalls)
	}
}
