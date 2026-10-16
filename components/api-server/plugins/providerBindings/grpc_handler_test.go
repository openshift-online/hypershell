package providerBindings

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// stubProviderBindingService is a minimal ProviderBindingService for gRPC handler tests.
type stubProviderBindingService struct {
	got       *ProviderBinding
	ret       *ProviderBinding
	err       *errors.ServiceError
	deletedID string
}

func (s *stubProviderBindingService) Get(ctx context.Context, id string) (*ProviderBinding, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubProviderBindingService) Create(ctx context.Context, b *ProviderBinding) (*ProviderBinding, *errors.ServiceError) {
	s.got = b
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubProviderBindingService) Replace(ctx context.Context, b *ProviderBinding) (*ProviderBinding, *errors.ServiceError) {
	s.got = b
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubProviderBindingService) Delete(ctx context.Context, id string) *errors.ServiceError {
	s.deletedID = id
	return s.err
}
func (s *stubProviderBindingService) All(ctx context.Context) (ProviderBindingList, *errors.ServiceError) {
	return ProviderBindingList{}, s.err
}
func (s *stubProviderBindingService) FindByIDs(ctx context.Context, ids []string) (ProviderBindingList, *errors.ServiceError) {
	return ProviderBindingList{}, s.err
}

func sampleProviderBinding() *ProviderBinding {
	b := &ProviderBinding{Name: "binding", WorkspaceId: "ws-1", ProviderSpecId: "ps-1"}
	b.ID = "pb-1"
	return b
}

func TestProviderBindingToProto(t *testing.T) {
	b := sampleProviderBinding()
	b.SecretSourceId = "ss-1"
	got := providerBindingToProto(b)
	if got.GetMetadata().GetId() != "pb-1" {
		t.Fatalf("id = %q, want pb-1", got.GetMetadata().GetId())
	}
	if got.GetMetadata().GetKind() != "ProviderBinding" {
		t.Fatalf("kind = %q, want ProviderBinding", got.GetMetadata().GetKind())
	}
	if got.GetMetadata().GetHref() != "/api/hypershell/ext/provider_bindings/pb-1" {
		t.Fatalf("href = %q", got.GetMetadata().GetHref())
	}
	if got.GetName() != "binding" || got.GetWorkspaceId() != "ws-1" || got.GetProviderSpecId() != "ps-1" {
		t.Fatalf("field mapping wrong: %+v", got)
	}
	if got.GetSecretSourceId() != "ss-1" {
		t.Fatalf("secret_source_id = %q, want ss-1", got.GetSecretSourceId())
	}
}

func TestCreateProviderBinding_ValidatesRequiredFields(t *testing.T) {
	h := &providerBindingGRPCHandler{service: &stubProviderBindingService{}}
	cases := []*pb.CreateProviderBindingRequest{
		{WorkspaceId: "ws-1", ProviderSpecId: "ps-1"},
		{Name: "n", ProviderSpecId: "ps-1"},
		{Name: "n", WorkspaceId: "ws-1"},
	}
	for i, req := range cases {
		if _, err := h.CreateProviderBinding(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("case %d: code = %s, want InvalidArgument", i, status.Code(err))
		}
	}
}

func TestCreateProviderBinding_Success(t *testing.T) {
	svc := &stubProviderBindingService{ret: sampleProviderBinding()}
	h := &providerBindingGRPCHandler{service: svc}
	secretSourceID := "ss-1"
	resp, err := h.CreateProviderBinding(context.Background(), &pb.CreateProviderBindingRequest{
		Name: "binding", WorkspaceId: "ws-1", ProviderSpecId: "ps-1", SecretSourceId: &secretSourceID,
	})
	if err != nil {
		t.Fatalf("CreateProviderBinding error = %v", err)
	}
	if resp.GetProviderBinding().GetMetadata().GetId() != "pb-1" {
		t.Fatalf("id = %q, want pb-1", resp.GetProviderBinding().GetMetadata().GetId())
	}
	if svc.got.Name != "binding" || svc.got.WorkspaceId != "ws-1" {
		t.Fatalf("service received wrong model: %+v", svc.got)
	}
	if svc.got.SecretSourceId != "ss-1" {
		t.Fatalf("secret_source_id not applied: %+v", svc.got)
	}
}

func TestGetProviderBinding_NotFound(t *testing.T) {
	h := &providerBindingGRPCHandler{service: &stubProviderBindingService{err: errors.NotFound("missing")}}
	if _, err := h.GetProviderBinding(context.Background(), &pb.GetProviderBindingRequest{Id: "pb-x"}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", status.Code(err))
	}
}

func TestGetProviderBinding_ValidatesID(t *testing.T) {
	h := &providerBindingGRPCHandler{service: &stubProviderBindingService{}}
	if _, err := h.GetProviderBinding(context.Background(), &pb.GetProviderBindingRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %s, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateProviderBinding_AppliesFields(t *testing.T) {
	svc := &stubProviderBindingService{ret: sampleProviderBinding()}
	h := &providerBindingGRPCHandler{service: svc}
	newName := "renamed"
	newSecret := "ss-2"
	strategy := `{"interval":"1h"}`
	if _, err := h.UpdateProviderBinding(context.Background(), &pb.UpdateProviderBindingRequest{
		Id: "pb-1", Name: &newName, SecretSourceId: &newSecret, RefreshStrategy: &strategy,
	}); err != nil {
		t.Fatalf("UpdateProviderBinding error = %v", err)
	}
	if svc.got.Name != "renamed" {
		t.Fatalf("name not applied: %+v", svc.got)
	}
	if svc.got.SecretSourceId != "ss-2" {
		t.Fatalf("secret_source_id not applied: %+v", svc.got)
	}
	if svc.got.RefreshStrategy == nil || *svc.got.RefreshStrategy != strategy {
		t.Fatalf("refresh_strategy not applied: %+v", svc.got)
	}
}

func TestDeleteProviderBinding_Success(t *testing.T) {
	svc := &stubProviderBindingService{}
	h := &providerBindingGRPCHandler{service: svc}
	if _, err := h.DeleteProviderBinding(context.Background(), &pb.DeleteProviderBindingRequest{Id: "pb-1"}); err != nil {
		t.Fatalf("DeleteProviderBinding error = %v", err)
	}
	if svc.deletedID != "pb-1" {
		t.Fatalf("deleted id = %q, want pb-1", svc.deletedID)
	}
}
