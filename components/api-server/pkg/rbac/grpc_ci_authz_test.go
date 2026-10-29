package rbac

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CI federation gRPC parity (SEC-RBAC-09.08): a registered control plane may READ
// the CI kinds to drive reconciliation; all writes over gRPC are denied, and for
// non-control-plane callers only platform:admin may read (the gRPC lists are
// unfiltered), while every write is denied because both kinds are authored over
// HTTP.

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
			t.Errorf("control plane write %s should be PermissionDenied; got %v", m, code)
		}
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
