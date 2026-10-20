package inferenceRoutes

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// stubInferenceRouteService is a minimal InferenceRouteService for gRPC handler
// tests. InferenceRoute has no Update, so there is no Replace method.
type stubInferenceRouteService struct {
	got       *InferenceRoute
	ret       *InferenceRoute
	err       *errors.ServiceError
	deletedID string
}

func (s *stubInferenceRouteService) Get(ctx context.Context, id string) (*InferenceRoute, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubInferenceRouteService) Create(ctx context.Context, r *InferenceRoute) (*InferenceRoute, *errors.ServiceError) {
	s.got = r
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubInferenceRouteService) Delete(ctx context.Context, id string) *errors.ServiceError {
	s.deletedID = id
	return s.err
}
func (s *stubInferenceRouteService) All(ctx context.Context) (InferenceRouteList, *errors.ServiceError) {
	return InferenceRouteList{}, s.err
}
func (s *stubInferenceRouteService) FindByIDs(ctx context.Context, ids []string) (InferenceRouteList, *errors.ServiceError) {
	return InferenceRouteList{}, s.err
}

func sampleInferenceRoute() *InferenceRoute {
	r := &InferenceRoute{WorkspaceId: "ws-1", ProviderBindingId: "pb-1", Model: "claude"}
	r.ID = "ir-1"
	return r
}

func TestInferenceRouteToProto(t *testing.T) {
	got := inferenceRouteToProto(sampleInferenceRoute())
	if got.GetMetadata().GetId() != "ir-1" {
		t.Fatalf("id = %q, want ir-1", got.GetMetadata().GetId())
	}
	if got.GetMetadata().GetKind() != "InferenceRoute" {
		t.Fatalf("kind = %q, want InferenceRoute", got.GetMetadata().GetKind())
	}
	if got.GetMetadata().GetHref() != "/api/hypershell/ext/inference_routes/ir-1" {
		t.Fatalf("href = %q", got.GetMetadata().GetHref())
	}
	if got.GetWorkspaceId() != "ws-1" || got.GetProviderBindingId() != "pb-1" || got.GetModel() != "claude" {
		t.Fatalf("field mapping wrong: %+v", got)
	}
}

func TestCreateInferenceRoute_ValidatesRequiredFields(t *testing.T) {
	h := &inferenceRouteGRPCHandler{service: &stubInferenceRouteService{}}
	cases := []*pb.CreateInferenceRouteRequest{
		{ProviderBindingId: "pb-1", Model: "claude"},
		{WorkspaceId: "ws-1", Model: "claude"},
		{WorkspaceId: "ws-1", ProviderBindingId: "pb-1"},
	}
	for i, req := range cases {
		if _, err := h.CreateInferenceRoute(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("case %d: code = %s, want InvalidArgument", i, status.Code(err))
		}
	}
}

func TestCreateInferenceRoute_Success(t *testing.T) {
	svc := &stubInferenceRouteService{ret: sampleInferenceRoute()}
	h := &inferenceRouteGRPCHandler{service: svc}
	resp, err := h.CreateInferenceRoute(context.Background(), &pb.CreateInferenceRouteRequest{
		WorkspaceId: "ws-1", ProviderBindingId: "pb-1", Model: "claude",
	})
	if err != nil {
		t.Fatalf("CreateInferenceRoute error = %v", err)
	}
	if resp.GetInferenceRoute().GetMetadata().GetId() != "ir-1" {
		t.Fatalf("id = %q, want ir-1", resp.GetInferenceRoute().GetMetadata().GetId())
	}
	if svc.got.WorkspaceId != "ws-1" || svc.got.ProviderBindingId != "pb-1" || svc.got.Model != "claude" {
		t.Fatalf("service received wrong model: %+v", svc.got)
	}
}

func TestGetInferenceRoute_NotFound(t *testing.T) {
	h := &inferenceRouteGRPCHandler{service: &stubInferenceRouteService{err: errors.NotFound("missing")}}
	if _, err := h.GetInferenceRoute(context.Background(), &pb.GetInferenceRouteRequest{Id: "ir-x"}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", status.Code(err))
	}
}

func TestGetInferenceRoute_ValidatesID(t *testing.T) {
	h := &inferenceRouteGRPCHandler{service: &stubInferenceRouteService{}}
	if _, err := h.GetInferenceRoute(context.Background(), &pb.GetInferenceRouteRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %s, want InvalidArgument", status.Code(err))
	}
}

func TestDeleteInferenceRoute_Success(t *testing.T) {
	svc := &stubInferenceRouteService{ret: sampleInferenceRoute()}
	h := &inferenceRouteGRPCHandler{service: svc}
	if _, err := h.DeleteInferenceRoute(context.Background(), &pb.DeleteInferenceRouteRequest{Id: "ir-1"}); err != nil {
		t.Fatalf("DeleteInferenceRoute error = %v", err)
	}
	if svc.deletedID != "ir-1" {
		t.Fatalf("deleted id = %q, want ir-1", svc.deletedID)
	}
}
