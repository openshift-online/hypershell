package openshellclient

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	openshellv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/openshellv1"
)

// fakeOpenShell implements openshellv1.OpenShellClient; only the workspace methods
// the wrapper calls are implemented, the rest are embedded (nil) and never invoked.
type fakeOpenShell struct {
	openshellv1.OpenShellClient

	existingWorkspaces map[string]bool
	created            []string
	createErr          error

	members     []*openshellv1.WorkspaceMember
	added       []*openshellv1.AddWorkspaceMemberRequest
	addErr      error
	addConflict bool
}

func (f *fakeOpenShell) GetWorkspace(_ context.Context, in *openshellv1.GetWorkspaceRequest, _ ...grpc.CallOption) (*openshellv1.GetWorkspaceResponse, error) {
	if f.existingWorkspaces[in.GetName()] {
		return &openshellv1.GetWorkspaceResponse{}, nil
	}
	return nil, status.Error(codes.NotFound, "workspace not found")
}

func (f *fakeOpenShell) CreateWorkspace(_ context.Context, in *openshellv1.CreateWorkspaceRequest, _ ...grpc.CallOption) (*openshellv1.CreateWorkspaceResponse, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, in.GetName())
	return &openshellv1.CreateWorkspaceResponse{}, nil
}

func (f *fakeOpenShell) ListWorkspaceMembers(_ context.Context, _ *openshellv1.ListWorkspaceMembersRequest, _ ...grpc.CallOption) (*openshellv1.ListWorkspaceMembersResponse, error) {
	return &openshellv1.ListWorkspaceMembersResponse{Members: f.members}, nil
}

func (f *fakeOpenShell) AddWorkspaceMember(_ context.Context, in *openshellv1.AddWorkspaceMemberRequest, _ ...grpc.CallOption) (*openshellv1.AddWorkspaceMemberResponse, error) {
	if f.addConflict {
		return nil, status.Error(codes.AlreadyExists, "already a member")
	}
	if f.addErr != nil {
		return nil, f.addErr
	}
	f.added = append(f.added, in)
	return &openshellv1.AddWorkspaceMemberResponse{}, nil
}

func TestEnsureWorkspace_CreatesWhenAbsent(t *testing.T) {
	f := &fakeOpenShell{existingWorkspaces: map[string]bool{}}
	s := &Session{cl: f, tok: "t"}
	if err := s.EnsureWorkspace(context.Background(), "gh-acme"); err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if len(f.created) != 1 || f.created[0] != "gh-acme" {
		t.Fatalf("want create gh-acme, got %v", f.created)
	}
}

func TestEnsureWorkspace_NoopWhenPresent(t *testing.T) {
	f := &fakeOpenShell{existingWorkspaces: map[string]bool{"gh-acme": true}}
	s := &Session{cl: f, tok: "t"}
	if err := s.EnsureWorkspace(context.Background(), "gh-acme"); err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if len(f.created) != 0 {
		t.Fatalf("expected no create, got %v", f.created)
	}
}

func TestEnsureWorkspace_CreateAlreadyExistsIsSuccess(t *testing.T) {
	f := &fakeOpenShell{existingWorkspaces: map[string]bool{}, createErr: status.Error(codes.AlreadyExists, "raced")}
	s := &Session{cl: f, tok: "t"}
	if err := s.EnsureWorkspace(context.Background(), "gh-acme"); err != nil {
		t.Fatalf("AlreadyExists on create should be success, got %v", err)
	}
}

func TestEnsureMembers_AddsMissingSkipsPresentAdditiveOnly(t *testing.T) {
	f := &fakeOpenShell{
		members: []*openshellv1.WorkspaceMember{
			{PrincipalSubject: "present-user", Role: openshellv1.WorkspaceRole_WORKSPACE_ROLE_USER},
			{PrincipalSubject: "stranger", Role: openshellv1.WorkspaceRole_WORKSPACE_ROLE_ADMIN},
		},
	}
	s := &Session{cl: f, tok: "t"}
	want := map[string]Role{
		"present-user": RoleUser, // already present with same role -> skip
		"new-user":     RoleUser, // missing -> add
	}
	if err := s.EnsureMembers(context.Background(), "gh-acme", want); err != nil {
		t.Fatalf("EnsureMembers: %v", err)
	}
	if len(f.added) != 1 || f.added[0].GetPrincipalSubject() != "new-user" {
		t.Fatalf("want add only new-user, got %v", f.added)
	}
	// Additive only: "stranger" (not in want) must NOT be removed; there is no
	// remove call in EnsureMembers at all.
}

func TestEnsureMembers_WrongRoleIsReApplied(t *testing.T) {
	f := &fakeOpenShell{
		members: []*openshellv1.WorkspaceMember{
			{PrincipalSubject: "u", Role: openshellv1.WorkspaceRole_WORKSPACE_ROLE_USER},
		},
	}
	s := &Session{cl: f, tok: "t"}
	if err := s.EnsureMembers(context.Background(), "gh-acme", map[string]Role{"u": RoleAdmin}); err != nil {
		t.Fatalf("EnsureMembers: %v", err)
	}
	if len(f.added) != 1 || f.added[0].GetRole() != openshellv1.WorkspaceRole_WORKSPACE_ROLE_ADMIN {
		t.Fatalf("want re-add u as admin, got %v", f.added)
	}
}

func TestEnsureMembers_AddAlreadyExistsTolerated(t *testing.T) {
	f := &fakeOpenShell{addConflict: true}
	s := &Session{cl: f, tok: "t"}
	if err := s.EnsureMembers(context.Background(), "gh-acme", map[string]Role{"u": RoleUser}); err != nil {
		t.Fatalf("AlreadyExists on add should be tolerated, got %v", err)
	}
}

func TestRoleFromGatewayRole(t *testing.T) {
	if RoleFromGatewayRole("openshell-admin") != RoleAdmin {
		t.Fatal("openshell-admin should map to RoleAdmin")
	}
	for _, r := range []string{"openshell-user", "", "whatever"} {
		if RoleFromGatewayRole(r) != RoleUser {
			t.Fatalf("%q should map to RoleUser", r)
		}
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	cases := map[string]string{
		"grpcs://gw-ns.example.com:443":  "gw-ns.example.com:443",
		"https://gw-ns.example.com/path": "gw-ns.example.com:443",
		"gw-ns.example.com":              "gw-ns.example.com:443",
		"gw-ns.example.com:8443":         "gw-ns.example.com:8443",
		"  ":                             "",
	}
	for in, want := range cases {
		if got := normalizeEndpoint(in); got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}
