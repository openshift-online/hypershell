package secretSources

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// stubSecretSourceService is a minimal SecretSourceService for gRPC handler tests.
type stubSecretSourceService struct {
	got       *SecretSource
	ret       *SecretSource
	err       *errors.ServiceError
	deletedID string
}

func (s *stubSecretSourceService) Get(ctx context.Context, id string) (*SecretSource, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubSecretSourceService) Create(ctx context.Context, item *SecretSource) (*SecretSource, *errors.ServiceError) {
	s.got = item
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubSecretSourceService) Replace(ctx context.Context, item *SecretSource) (*SecretSource, *errors.ServiceError) {
	s.got = item
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubSecretSourceService) Delete(ctx context.Context, id string) *errors.ServiceError {
	s.deletedID = id
	return s.err
}
func (s *stubSecretSourceService) All(ctx context.Context) (SecretSourceList, *errors.ServiceError) {
	return SecretSourceList{}, s.err
}
func (s *stubSecretSourceService) FindByIDs(ctx context.Context, ids []string) (SecretSourceList, *errors.ServiceError) {
	return SecretSourceList{}, s.err
}
func (s *stubSecretSourceService) FindByAgentRuntimeID(ctx context.Context, agentRuntimeID string) (SecretSourceList, *errors.ServiceError) {
	return SecretSourceList{}, s.err
}

func sampleSecretSource() *SecretSource {
	item := &SecretSource{
		Name:           "gateway-oidc",
		AgentRuntimeId: "ar-1",
		Purpose:        "gateway-identity",
		Backend:        "vault",
		Path:           "secret/gw",
	}
	item.ID = "ss-1"
	return item
}

func TestSecretSourceToProto(t *testing.T) {
	item := sampleSecretSource()
	got := secretSourceToProto(item)
	if got.GetMetadata().GetId() != "ss-1" {
		t.Fatalf("id = %q, want ss-1", got.GetMetadata().GetId())
	}
	if got.GetMetadata().GetKind() != "SecretSource" {
		t.Fatalf("kind = %q, want SecretSource", got.GetMetadata().GetKind())
	}
	if got.GetMetadata().GetHref() != "/api/hypershell/ext/secret_sources/ss-1" {
		t.Fatalf("href = %q", got.GetMetadata().GetHref())
	}
	if got.GetName() != "gateway-oidc" || got.GetAgentRuntimeId() != "ar-1" || got.GetBackend() != "vault" {
		t.Fatalf("field mapping wrong: %+v", got)
	}
}

func TestCreateSecretSource_ValidatesRequiredFields(t *testing.T) {
	h := &secretSourceGRPCHandler{service: &stubSecretSourceService{}}
	cases := []*pb.CreateSecretSourceRequest{
		{AgentRuntimeId: "ar-1", Purpose: "gateway-identity", Backend: "vault", Path: "secret/gw"},
		{Name: "n", Purpose: "gateway-identity", Backend: "vault", Path: "secret/gw"},
		{Name: "n", AgentRuntimeId: "ar-1", Backend: "vault", Path: "secret/gw"},
		{Name: "n", AgentRuntimeId: "ar-1", Purpose: "gateway-identity", Path: "secret/gw"},
		{Name: "n", AgentRuntimeId: "ar-1", Purpose: "gateway-identity", Backend: "vault"},
	}
	for i, req := range cases {
		if _, err := h.CreateSecretSource(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("case %d: code = %s, want InvalidArgument", i, status.Code(err))
		}
	}
}

func TestCreateSecretSource_Success(t *testing.T) {
	svc := &stubSecretSourceService{ret: sampleSecretSource()}
	h := &secretSourceGRPCHandler{service: svc}
	resp, err := h.CreateSecretSource(context.Background(), &pb.CreateSecretSourceRequest{
		Name: "gateway-oidc", AgentRuntimeId: "ar-1", Purpose: "gateway-identity", Backend: "vault", Path: "secret/gw",
	})
	if err != nil {
		t.Fatalf("CreateSecretSource error = %v", err)
	}
	if resp.GetSecretSource().GetMetadata().GetId() != "ss-1" {
		t.Fatalf("id = %q, want ss-1", resp.GetSecretSource().GetMetadata().GetId())
	}
	if svc.got.Name != "gateway-oidc" || svc.got.AgentRuntimeId != "ar-1" || svc.got.Backend != "vault" {
		t.Fatalf("service received wrong model: %+v", svc.got)
	}
}

func TestGetSecretSource_NotFound(t *testing.T) {
	h := &secretSourceGRPCHandler{service: &stubSecretSourceService{err: errors.NotFound("missing")}}
	if _, err := h.GetSecretSource(context.Background(), &pb.GetSecretSourceRequest{Id: "ss-x"}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", status.Code(err))
	}
}

func TestGetSecretSource_ValidatesID(t *testing.T) {
	h := &secretSourceGRPCHandler{service: &stubSecretSourceService{}}
	if _, err := h.GetSecretSource(context.Background(), &pb.GetSecretSourceRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %s, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateSecretSource_AppliesFields(t *testing.T) {
	svc := &stubSecretSourceService{ret: sampleSecretSource()}
	h := &secretSourceGRPCHandler{service: svc}
	newPurpose := "api-key"
	if _, err := h.UpdateSecretSource(context.Background(), &pb.UpdateSecretSourceRequest{Id: "ss-1", Purpose: &newPurpose}); err != nil {
		t.Fatalf("UpdateSecretSource error = %v", err)
	}
	if svc.got.Purpose != "api-key" {
		t.Fatalf("purpose not applied: %+v", svc.got)
	}
}

func TestDeleteSecretSource_Success(t *testing.T) {
	svc := &stubSecretSourceService{ret: sampleSecretSource()}
	h := &secretSourceGRPCHandler{service: svc}
	if _, err := h.DeleteSecretSource(context.Background(), &pb.DeleteSecretSourceRequest{Id: "ss-1"}); err != nil {
		t.Fatalf("DeleteSecretSource error = %v", err)
	}
	if svc.deletedID != "ss-1" {
		t.Fatalf("deleted id = %q, want ss-1", svc.deletedID)
	}
}
