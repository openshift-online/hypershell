package agentWorkspaces

import (
	"context"
	"testing"
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// stubDao is an in-memory AgentWorkspaceDao for service validation tests.
type stubDao struct {
	byGatewayName map[string]AgentWorkspaceList
	byRuntime     map[string]AgentWorkspaceList
	created       *AgentWorkspace
}

func (s *stubDao) Get(ctx context.Context, id string) (*AgentWorkspace, error) { return nil, nil }
func (s *stubDao) Create(ctx context.Context, ws *AgentWorkspace) (*AgentWorkspace, error) {
	s.created = ws
	return ws, nil
}
func (s *stubDao) Replace(ctx context.Context, ws *AgentWorkspace) (*AgentWorkspace, error) {
	return ws, nil
}
func (s *stubDao) Delete(ctx context.Context, id string) error { return nil }
func (s *stubDao) FindByIDs(ctx context.Context, ids []string) (AgentWorkspaceList, error) {
	return nil, nil
}
func (s *stubDao) All(ctx context.Context) (AgentWorkspaceList, error) { return nil, nil }
func (s *stubDao) FindByGatewayName(ctx context.Context, gatewayID, name string) (AgentWorkspaceList, error) {
	return s.byGatewayName[gatewayID+"/"+name], nil
}
func (s *stubDao) FindByAgentRuntime(ctx context.Context, agentRuntimeID string) (AgentWorkspaceList, error) {
	return s.byRuntime[agentRuntimeID], nil
}
func (s *stubDao) ProviderBindingsForWorkspace(ctx context.Context, workspaceID string) ([]ProviderBindingRow, error) {
	return nil, nil
}
func (s *stubDao) MembershipsForWorkspace(ctx context.Context, workspaceID string) ([]MembershipRow, error) {
	return nil, nil
}
func (s *stubDao) InferenceRouteForWorkspace(ctx context.Context, workspaceID string) (*InferenceRouteRow, error) {
	return nil, nil
}

type noopEvents struct{}

func (noopEvents) Create(ctx context.Context, e *api.Event) (*api.Event, *errors.ServiceError) {
	return e, nil
}
func (noopEvents) Get(ctx context.Context, id string) (*api.Event, *errors.ServiceError) {
	return nil, nil
}
func (noopEvents) Replace(ctx context.Context, e *api.Event) (*api.Event, *errors.ServiceError) {
	return e, nil
}
func (noopEvents) Delete(ctx context.Context, id string) *errors.ServiceError { return nil }
func (noopEvents) All(ctx context.Context) (api.EventList, *errors.ServiceError) {
	return nil, nil
}
func (noopEvents) FindByIDs(ctx context.Context, ids []string) (api.EventList, *errors.ServiceError) {
	return nil, nil
}
func (noopEvents) FindUnreconciled(ctx context.Context, olderThan time.Duration) (api.EventList, *errors.ServiceError) {
	return nil, nil
}
func (noopEvents) FindBySourceAndType(ctx context.Context, source string, eventType api.EventType) (api.EventList, *errors.ServiceError) {
	return nil, nil
}

func TestCreate_NameCollisionDifferentRuntimeIsConflict(t *testing.T) {
	existing := &AgentWorkspace{AgentRuntimeId: "r-other", GatewayId: "gw-1", Name: "amber-reviews"}
	existing.ID = "ws-existing"
	dao := &stubDao{byGatewayName: map[string]AgentWorkspaceList{"gw-1/amber-reviews": {existing}}}
	svc := NewAgentWorkspaceService(dao, noopEvents{})

	_, err := svc.Create(context.Background(), &AgentWorkspace{AgentRuntimeId: "r-mine", GatewayId: "gw-1", Name: "amber-reviews"})
	if err == nil || !err.IsConflict() {
		t.Fatalf("expected 409 conflict, got %v", err)
	}
}

func TestCreate_SameRuntimeAndNameIsNotCollision(t *testing.T) {
	dao := &stubDao{byGatewayName: map[string]AgentWorkspaceList{}}
	svc := NewAgentWorkspaceService(dao, noopEvents{})

	_, err := svc.Create(context.Background(), &AgentWorkspace{AgentRuntimeId: "r-mine", GatewayId: "gw-1", Name: "amber-reviews"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if dao.created == nil {
		t.Fatal("workspace was not created")
	}
}

func TestCreate_InvalidNameFormatIsValidationError(t *testing.T) {
	svc := NewAgentWorkspaceService(&stubDao{byGatewayName: map[string]AgentWorkspaceList{}}, noopEvents{})
	for _, bad := range []string{"", "Amber_Reviews", "-leading", "trailing-", "UPPER", "has space"} {
		_, err := svc.Create(context.Background(), &AgentWorkspace{AgentRuntimeId: "r", GatewayId: "gw-1", Name: bad})
		if err == nil || err.Code != errors.ErrorValidation {
			t.Fatalf("name %q: expected 400 validation, got %v", bad, err)
		}
	}
}

func TestCreate_RuntimeAlreadyOwnsWorkspaceIsConflict(t *testing.T) {
	owned := &AgentWorkspace{AgentRuntimeId: "r-mine", GatewayId: "gw-1", Name: "other"}
	owned.ID = "ws-owned"
	dao := &stubDao{byRuntime: map[string]AgentWorkspaceList{"r-mine": {owned}}}
	svc := NewAgentWorkspaceService(dao, noopEvents{})

	_, err := svc.Create(context.Background(), &AgentWorkspace{AgentRuntimeId: "r-mine", GatewayId: "gw-1", Name: "amber-reviews"})
	if err == nil || !err.IsConflict() {
		t.Fatalf("expected 409 conflict for second workspace, got %v", err)
	}
}
