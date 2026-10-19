package workspaceMemberships

import (
	"context"
	"testing"
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

type stubDao struct {
	requiresAdmin bool
	adminCount    int64
	byID          map[string]*WorkspaceMembership
	created       *WorkspaceMembership
	deletedID     string
}

func (s *stubDao) Get(ctx context.Context, id string) (*WorkspaceMembership, error) {
	return s.byID[id], nil
}
func (s *stubDao) Create(ctx context.Context, wm *WorkspaceMembership) (*WorkspaceMembership, error) {
	s.created = wm
	return wm, nil
}
func (s *stubDao) Replace(ctx context.Context, wm *WorkspaceMembership) (*WorkspaceMembership, error) {
	return wm, nil
}
func (s *stubDao) Delete(ctx context.Context, id string) error { s.deletedID = id; return nil }
func (s *stubDao) FindByIDs(ctx context.Context, ids []string) (WorkspaceMembershipList, error) {
	return nil, nil
}
func (s *stubDao) All(ctx context.Context) (WorkspaceMembershipList, error) { return nil, nil }
func (s *stubDao) CountAdmins(ctx context.Context, workspaceID, excludeID string) (int64, error) {
	return s.adminCount, nil
}
func (s *stubDao) WorkspaceRequiresAdmin(ctx context.Context, workspaceID string) (bool, error) {
	return s.requiresAdmin, nil
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

func TestCreate_InvalidRoleIsUnprocessable(t *testing.T) {
	svc := NewWorkspaceMembershipService(&stubDao{}, noopEvents{})
	_, err := svc.Create(context.Background(), &WorkspaceMembership{WorkspaceId: "ws-1", Subject: "s", Role: "owner"})
	if err == nil || err.Code != errors.ErrorUnprocessableEntity {
		t.Fatalf("expected 422 for invalid role, got %v", err)
	}
}

func TestCreate_MemberWithoutAdminWhenRefreshBearingIsUnprocessable(t *testing.T) {
	dao := &stubDao{requiresAdmin: true, adminCount: 0}
	svc := NewWorkspaceMembershipService(dao, noopEvents{})
	_, err := svc.Create(context.Background(), &WorkspaceMembership{WorkspaceId: "ws-1", Subject: "s", Role: RoleMember})
	if err == nil || err.Code != errors.ErrorUnprocessableEntity {
		t.Fatalf("expected 422 admin-required, got %v", err)
	}
}

func TestCreate_InvalidSubjectIsValidationError(t *testing.T) {
	svc := NewWorkspaceMembershipService(&stubDao{}, noopEvents{})
	for _, bad := range []string{"", "has space", "bad\tchar"} {
		_, err := svc.Create(context.Background(), &WorkspaceMembership{WorkspaceId: "ws-1", Subject: bad, Role: RoleAdmin})
		if err == nil || err.Code != errors.ErrorValidation {
			t.Fatalf("subject %q: expected 400 validation, got %v", bad, err)
		}
	}
}

func TestCreate_AdminMembershipAlwaysAllowed(t *testing.T) {
	dao := &stubDao{requiresAdmin: true, adminCount: 0}
	svc := NewWorkspaceMembershipService(dao, noopEvents{})
	_, err := svc.Create(context.Background(), &WorkspaceMembership{WorkspaceId: "ws-1", Subject: "s", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("admin create should succeed, got %v", err)
	}
	if dao.created == nil {
		t.Fatal("membership not created")
	}
}

func TestCreate_MemberAllowedWhenNoRefreshBearingBinding(t *testing.T) {
	dao := &stubDao{requiresAdmin: false, adminCount: 0}
	svc := NewWorkspaceMembershipService(dao, noopEvents{})
	_, err := svc.Create(context.Background(), &WorkspaceMembership{WorkspaceId: "ws-1", Subject: "s", Role: RoleMember})
	if err != nil {
		t.Fatalf("member create should succeed when no refresh-bearing binding, got %v", err)
	}
}

func TestDelete_LastAdminWhenRefreshBearingIsUnprocessable(t *testing.T) {
	adminMem := &WorkspaceMembership{WorkspaceId: "ws-1", Subject: "s", Role: RoleAdmin}
	adminMem.ID = "wm-admin"
	dao := &stubDao{requiresAdmin: true, adminCount: 0, byID: map[string]*WorkspaceMembership{"wm-admin": adminMem}}
	svc := NewWorkspaceMembershipService(dao, noopEvents{})
	err := svc.Delete(context.Background(), "wm-admin")
	if err == nil || err.Code != errors.ErrorUnprocessableEntity {
		t.Fatalf("expected 422 deleting last admin, got %v", err)
	}
	if dao.deletedID != "" {
		t.Fatal("membership should not have been deleted")
	}
}

func TestDelete_AdminAllowedWhenAnotherAdminRemains(t *testing.T) {
	adminMem := &WorkspaceMembership{WorkspaceId: "ws-1", Subject: "s", Role: RoleAdmin}
	adminMem.ID = "wm-admin"
	dao := &stubDao{requiresAdmin: true, adminCount: 1, byID: map[string]*WorkspaceMembership{"wm-admin": adminMem}}
	svc := NewWorkspaceMembershipService(dao, noopEvents{})
	if err := svc.Delete(context.Background(), "wm-admin"); err != nil {
		t.Fatalf("delete should succeed when another admin remains, got %v", err)
	}
	if dao.deletedID != "wm-admin" {
		t.Fatalf("expected deletion of wm-admin, got %q", dao.deletedID)
	}
}
