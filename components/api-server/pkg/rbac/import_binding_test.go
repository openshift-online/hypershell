package rbac

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fakeImportResolver reports whether a Repository id is in its known set.
type fakeImportResolver struct {
	known map[string]bool
	err   error
}

func (f fakeImportResolver) RepositoryExists(_ context.Context, id string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.known[id], nil
}

// importCtx returns a context carrying the reconciled-Repository binding metadata.
func importCtx(repositoryID string) context.Context {
	md := metadata.Pairs(ImportRepositoryMetadataKey, repositoryID)
	return metadata.NewIncomingContext(context.Background(), md)
}

// TestAuthorizeControlPlaneGRPC_ImportWriteBinding pins the declarative-import write
// authorization: an owned-kind write by a registered control plane is allowed only
// when the reconciled Repository is named in trusted metadata AND exists, and a
// Repository write stays denied regardless.
func TestAuthorizeControlPlaneGRPC_ImportWriteBinding(t *testing.T) {
	const clusterID = "cluster-1"
	imports := fakeImportResolver{known: map[string]bool{"repo-1": true}}

	t.Run("owned-kind write with a valid repository binding is allowed", func(t *testing.T) {
		err := authorizeControlPlaneGRPC(importCtx("repo-1"), clusterID,
			"/hypershell.v1.SandboxTemplateService/CreateSandboxTemplate", nil, nil, imports)
		if err != nil {
			t.Fatalf("want allowed, got %v", err)
		}
	})

	t.Run("owned-kind write without a repository binding is denied", func(t *testing.T) {
		err := authorizeControlPlaneGRPC(context.Background(), clusterID,
			"/hypershell.v1.SandboxTemplateService/CreateSandboxTemplate", nil, nil, imports)
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %s", status.Code(err))
		}
	})

	t.Run("owned-kind write naming an unknown repository is denied", func(t *testing.T) {
		err := authorizeControlPlaneGRPC(importCtx("ghost"), clusterID,
			"/hypershell.v1.SecretSourceService/DeleteSecretSource", nil, nil, imports)
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %s", status.Code(err))
		}
	})

	t.Run("a resolver failure is Unavailable, never a grant", func(t *testing.T) {
		failing := fakeImportResolver{err: context.DeadlineExceeded}
		err := authorizeControlPlaneGRPC(importCtx("repo-1"), clusterID,
			"/hypershell.v1.SandboxTemplateService/CreateSandboxTemplate", nil, nil, failing)
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("want Unavailable, got %s", status.Code(err))
		}
	})

	t.Run("nil resolver is Unavailable when a binding is present", func(t *testing.T) {
		err := authorizeControlPlaneGRPC(importCtx("repo-1"), clusterID,
			"/hypershell.v1.SandboxTemplateService/CreateSandboxTemplate", nil, nil, nil)
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("want Unavailable, got %s", status.Code(err))
		}
	})

	t.Run("Repository writes stay denied even with a binding", func(t *testing.T) {
		err := authorizeControlPlaneGRPC(importCtx("repo-1"), clusterID,
			"/hypershell.v1.RepositoryService/CreateRepository", nil, nil, imports)
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %s", status.Code(err))
		}
	})
}

// TestImportOwnershipMark confirms the ownership mark is taken from trusted metadata
// and never from a client-supplied value, and that cross-Repository writes are
// refused.
func TestImportOwnershipMark(t *testing.T) {
	t.Run("set from reconciled repository metadata", func(t *testing.T) {
		got := ImportOwnershipMark(importCtx("repo-1"))
		if got == nil || *got != "repo-1" {
			t.Fatalf("mark = %v, want repo-1", got)
		}
	})
	t.Run("nil without an import context", func(t *testing.T) {
		if got := ImportOwnershipMark(context.Background()); got != nil {
			t.Fatalf("mark = %v, want nil", got)
		}
	})
}

func TestEnforceImportOwnership(t *testing.T) {
	repo1 := "repo-1"
	repo2 := "repo-2"
	t.Run("same owner passes", func(t *testing.T) {
		if err := EnforceImportOwnership(importCtx("repo-1"), &repo1); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
	})
	// Verifies: SEC-RBAC-09.07
	t.Run("different owner refused", func(t *testing.T) {
		if status.Code(EnforceImportOwnership(importCtx("repo-1"), &repo2)) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied")
		}
	})
	// Verifies: SEC-RBAC-09.07
	t.Run("unowned resource refused in import context", func(t *testing.T) {
		if status.Code(EnforceImportOwnership(importCtx("repo-1"), nil)) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied")
		}
	})
	t.Run("no import context is a no-op", func(t *testing.T) {
		if err := EnforceImportOwnership(context.Background(), nil); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
	})
}

// TestIsImportExtKindWrite confirms only owned-kind writes are treated as import
// writes (so the bootstrap allowlist is kept off them), and reads and Repository
// writes are not.
func TestIsImportExtKindWrite(t *testing.T) {
	cases := map[string]bool{
		"/hypershell.v1.SandboxTemplateService/CreateSandboxTemplate": true,
		"/hypershell.v1.SecretSourceService/DeleteSecretSource":       true,
		"/hypershell.v1.SandboxTemplateService/ListSandboxTemplates":  false,
		"/hypershell.v1.RepositoryService/CreateRepository":           false,
		"/hypershell.v1.GatewayService/CreateGateway":                 false,
	}
	for method, want := range cases {
		if got := IsImportExtKindWrite(method); got != want {
			t.Fatalf("IsImportExtKindWrite(%s) = %v, want %v", method, got, want)
		}
	}
}
