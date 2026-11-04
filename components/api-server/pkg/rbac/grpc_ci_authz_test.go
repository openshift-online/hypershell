package rbac

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CI federation gRPC parity: a registered control plane may READ the CI kinds to
// drive reconciliation. CIProvider writes over gRPC stay denied - it is a
// platform-wide registration authored over HTTP (SEC-RBAC-09.08). CITrust is on the
// declarative-import write path (SEC-RBAC-09.09): an import-marked CITrust write
// backed by a real reconciled Repository is authorized; a CITrust write WITHOUT the
// import binding is still denied. For non-control-plane callers only platform:admin
// may read (the gRPC lists are unfiltered) and every write is denied.

func codeOf(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	return status.Code(err)
}

func TestControlPlaneGRPC_CIKindsReadAllowedWriteDenied(t *testing.T) {
	ctx := context.Background()
	reads := []string{
		"/hypershell.v1.CIProviderService/ListCIProviders",
		"/hypershell.v1.CIProviderService/GetCIProvider",
		"/hypershell.v1.CITrustService/WatchCITrusts",
		"/hypershell.v1.CITrustService/GetCITrust",
	}
	for _, m := range reads {
		if err := authorizeControlPlaneGRPC(ctx, "cluster-1", m, nil, nil, nil); err != nil {
			t.Errorf("control plane read %s should be allowed; got %v", m, err)
		}
	}
	// CIProvider writes are always denied over gRPC; CITrust writes WITHOUT an import
	// binding are denied too (authorizeImportWrite requires the reconciled-Repository
	// metadata).
	writes := []string{
		"/hypershell.v1.CIProviderService/CreateCIProvider",
		"/hypershell.v1.CIProviderService/UpdateCIProvider",
		"/hypershell.v1.CIProviderService/DeleteCIProvider",
		"/hypershell.v1.CITrustService/CreateCITrust",
		"/hypershell.v1.CITrustService/UpdateCITrust",
		"/hypershell.v1.CITrustService/DeleteCITrust",
	}
	for _, m := range writes {
		if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-1", m, nil, nil, nil)); code != codes.PermissionDenied {
			t.Errorf("control plane write %s without an import binding should be PermissionDenied; got %v", m, code)
		}
	}
}

// An import-marked CITrust write by a registered control plane, naming a real
// reconciled Repository, is authorized on the declarative-import write path. A
// CIProvider write on the same binding stays denied (not importable).
// Verifies: SEC-RBAC-09.09
func TestControlPlaneGRPC_CITrustImportWriteAuthorized(t *testing.T) {
	imports := fakeImportResolver{known: map[string]bool{"repo-1": true}}
	ctx := importCtx("repo-1")

	citrustWrites := []string{
		"/hypershell.v1.CITrustService/CreateCITrust",
		"/hypershell.v1.CITrustService/UpdateCITrust",
		"/hypershell.v1.CITrustService/DeleteCITrust",
	}
	for _, m := range citrustWrites {
		if err := authorizeControlPlaneGRPC(ctx, "cluster-1", m, nil, nil, imports); err != nil {
			t.Errorf("import-marked CITrust write %s should be authorized; got %v", m, err)
		}
	}

	// A CITrust write naming an UNKNOWN Repository is refused.
	if code := codeOf(authorizeControlPlaneGRPC(importCtx("ghost"), "cluster-1", "/hypershell.v1.CITrustService/CreateCITrust", nil, nil, imports)); code != codes.PermissionDenied {
		t.Errorf("CITrust write for an unknown repository should be PermissionDenied; got %v", code)
	}

	// CIProvider is not importable: an import binding does not authorize its write.
	if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-1", "/hypershell.v1.CIProviderService/CreateCIProvider", nil, nil, imports)); code != codes.PermissionDenied {
		t.Errorf("import-marked CIProvider write should stay PermissionDenied; got %v", code)
	}
}

func TestUserGRPC_CIKindsReadRequiresPlatformAdminWriteDenied(t *testing.T) {
	admin := []BindingSummary{{RoleName: "platform:admin", Scope: "global"}}
	gw := "gw-1"
	owner := []BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gw}}
	ciAdmin := []BindingSummary{{RoleName: "ci-federation:admin", Scope: "global"}}

	reads := []string{
		"/hypershell.v1.CIProviderService/ListCIProviders",
		"/hypershell.v1.CITrustService/ListCITrusts",
	}
	for _, m := range reads {
		if err := authorizeUserGRPC(m, nil, admin); err != nil {
			t.Errorf("platform:admin read %s should be allowed; got %v", m, err)
		}
		if code := codeOf(authorizeUserGRPC(m, nil, owner)); code != codes.PermissionDenied {
			t.Errorf("gateway owner read %s should be denied (unfiltered gRPC list); got %v", m, code)
		}
		if code := codeOf(authorizeUserGRPC(m, nil, ciAdmin)); code != codes.PermissionDenied {
			t.Errorf("ci-federation:admin read %s over gRPC should be denied; got %v", m, code)
		}
	}

	writes := []string{
		"/hypershell.v1.CIProviderService/CreateCIProvider",
		"/hypershell.v1.CITrustService/CreateCITrust",
	}
	for _, m := range writes {
		for _, b := range [][]BindingSummary{admin, owner, ciAdmin} {
			if code := codeOf(authorizeUserGRPC(m, nil, b)); code != codes.PermissionDenied {
				t.Errorf("user write %s should be denied over gRPC (authored over HTTP); got %v", m, code)
			}
		}
	}
}
