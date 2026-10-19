package providerSpecs

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// stubProviderSpecService is a minimal ProviderSpecService for gRPC handler tests.
type stubProviderSpecService struct {
	got       *ProviderSpec
	ret       *ProviderSpec
	err       *errors.ServiceError
	deletedID string
}

func (s *stubProviderSpecService) Get(ctx context.Context, id string) (*ProviderSpec, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubProviderSpecService) Create(ctx context.Context, ps *ProviderSpec) (*ProviderSpec, *errors.ServiceError) {
	s.got = ps
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubProviderSpecService) Replace(ctx context.Context, ps *ProviderSpec) (*ProviderSpec, *errors.ServiceError) {
	s.got = ps
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubProviderSpecService) Delete(ctx context.Context, id string) *errors.ServiceError {
	s.deletedID = id
	return s.err
}
func (s *stubProviderSpecService) All(ctx context.Context) (ProviderSpecList, *errors.ServiceError) {
	return ProviderSpecList{}, s.err
}
func (s *stubProviderSpecService) FindByIDs(ctx context.Context, ids []string) (ProviderSpecList, *errors.ServiceError) {
	return ProviderSpecList{}, s.err
}

func sampleProviderSpec() *ProviderSpec {
	ps := &ProviderSpec{Name: "gpu-pool", Category: "compute"}
	ps.ID = "ps-1"
	return ps
}

func TestProviderSpecToProto(t *testing.T) {
	ps := sampleProviderSpec()
	capValue := `{"gpu":true}`
	ps.Capability = &capValue
	got := providerSpecToProto(ps)
	if got.GetMetadata().GetId() != "ps-1" {
		t.Fatalf("id = %q, want ps-1", got.GetMetadata().GetId())
	}
	if got.GetMetadata().GetKind() != "ProviderSpec" {
		t.Fatalf("kind = %q, want ProviderSpec", got.GetMetadata().GetKind())
	}
	if got.GetMetadata().GetHref() != "/api/hypershell/ext/provider_specs/ps-1" {
		t.Fatalf("href = %q", got.GetMetadata().GetHref())
	}
	if got.GetName() != "gpu-pool" || got.GetCategory() != "compute" || got.GetCapability() != `{"gpu":true}` {
		t.Fatalf("field mapping wrong: %+v", got)
	}
}

func TestCreateProviderSpec_ValidatesRequiredFields(t *testing.T) {
	h := &providerSpecGRPCHandler{service: &stubProviderSpecService{}}
	cases := []*pb.CreateProviderSpecRequest{
		{Category: "compute"},
		{Name: "gpu-pool"},
	}
	for i, req := range cases {
		if _, err := h.CreateProviderSpec(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("case %d: code = %s, want InvalidArgument", i, status.Code(err))
		}
	}
}

func TestCreateProviderSpec_Success(t *testing.T) {
	svc := &stubProviderSpecService{ret: sampleProviderSpec()}
	h := &providerSpecGRPCHandler{service: svc}
	resp, err := h.CreateProviderSpec(context.Background(), &pb.CreateProviderSpecRequest{
		Name: "gpu-pool", Category: "compute",
	})
	if err != nil {
		t.Fatalf("CreateProviderSpec error = %v", err)
	}
	if resp.GetProviderSpec().GetMetadata().GetId() != "ps-1" {
		t.Fatalf("id = %q, want ps-1", resp.GetProviderSpec().GetMetadata().GetId())
	}
	if svc.got.Name != "gpu-pool" || svc.got.Category != "compute" {
		t.Fatalf("service received wrong model: %+v", svc.got)
	}
}

func TestGetProviderSpec_NotFound(t *testing.T) {
	h := &providerSpecGRPCHandler{service: &stubProviderSpecService{err: errors.NotFound("missing")}}
	if _, err := h.GetProviderSpec(context.Background(), &pb.GetProviderSpecRequest{Id: "ps-x"}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", status.Code(err))
	}
}

func TestGetProviderSpec_ValidatesID(t *testing.T) {
	h := &providerSpecGRPCHandler{service: &stubProviderSpecService{}}
	if _, err := h.GetProviderSpec(context.Background(), &pb.GetProviderSpecRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %s, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateProviderSpec_AppliesFields(t *testing.T) {
	svc := &stubProviderSpecService{ret: sampleProviderSpec()}
	h := &providerSpecGRPCHandler{service: svc}
	newCap := `{"gpu":false}`
	if _, err := h.UpdateProviderSpec(context.Background(), &pb.UpdateProviderSpecRequest{Id: "ps-1", Capability: &newCap}); err != nil {
		t.Fatalf("UpdateProviderSpec error = %v", err)
	}
	if svc.got.Capability == nil || *svc.got.Capability != `{"gpu":false}` {
		t.Fatalf("capability not applied: %+v", svc.got)
	}
}

func TestDeleteProviderSpec_Success(t *testing.T) {
	svc := &stubProviderSpecService{ret: sampleProviderSpec()}
	h := &providerSpecGRPCHandler{service: svc}
	if _, err := h.DeleteProviderSpec(context.Background(), &pb.DeleteProviderSpecRequest{Id: "ps-1"}); err != nil {
		t.Fatalf("DeleteProviderSpec error = %v", err)
	}
	if svc.deletedID != "ps-1" {
		t.Fatalf("deleted id = %q, want ps-1", svc.deletedID)
	}
}
