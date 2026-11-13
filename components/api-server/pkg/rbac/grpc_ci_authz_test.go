package rbac

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
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
	// CIProvider create/delete are always denied over gRPC; CITrust create/delete
	// WITHOUT an import binding are denied too (authorizeImportWrite requires the
	// reconciled-Repository metadata). The status-only UpdateCIProvider/UpdateCITrust
	// write-backs are a distinct authorized path, covered by the SEC-RBAC-27 tests.
	writes := []string{
		"/hypershell.v1.CIProviderService/CreateCIProvider",
		"/hypershell.v1.CIProviderService/DeleteCIProvider",
		"/hypershell.v1.CITrustService/CreateCITrust",
		"/hypershell.v1.CITrustService/DeleteCITrust",
	}
	for _, m := range writes {
		if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-1", m, nil, nil, nil)); code != codes.PermissionDenied {
			t.Errorf("control plane write %s without an import binding should be PermissionDenied; got %v", m, code)
		}
	}
}

// SEC-RBAC-27: a registered control plane may write back CIProvider status
// (status-only, platform-wide), but a write that changes a spec field is denied, and
// Create/Delete stay denied.
// Verifies: SEC-RBAC-27.01, SEC-RBAC-27.02
func TestControlPlaneGRPC_CIProviderStatusWriteBack(t *testing.T) {
	ctx := context.Background()
	const m = "/hypershell.v1.CIProviderService/UpdateCIProvider"

	// Status-only: jwks_url (the only gRPC-settable spec field) unset, only
	// status_idp_alias changed.
	alias := "github-actions"
	statusOnly := &pb.UpdateCIProviderRequest{Id: "cp-1", StatusIdpAlias: &alias}
	if err := authorizeControlPlaneGRPC(ctx, "cluster-1", m, statusOnly, nil, nil); err != nil {
		t.Errorf("status-only UpdateCIProvider should be allowed; got %v", err)
	}

	// A spec-field change (jwks_url) is denied: the control plane never authors spec.
	jwks := "https://example.test/jwks"
	specChange := &pb.UpdateCIProviderRequest{Id: "cp-1", JwksUrl: &jwks}
	if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-1", m, specChange, nil, nil)); code != codes.PermissionDenied {
		t.Errorf("spec-field UpdateCIProvider should be PermissionDenied; got %v", code)
	}
}

// SEC-RBAC-27: a control plane may write back CITrust status only when the trust's
// referenced gateway belongs to its cluster; a foreign-cluster trust and a spec-field
// change are denied.
// Verifies: SEC-RBAC-27.03, SEC-RBAC-27.04
func TestControlPlaneGRPC_CITrustStatusWriteBack(t *testing.T) {
	ctx := context.Background()
	const m = "/hypershell.v1.CITrustService/UpdateCITrust"
	gateways := fakeGateways{}

	endpoint := "https://kc.example.test/token"
	workspace := "gh-acme"
	hint := "forge=github ..."

	// Own-cluster, status-only: authorized.
	ownStatus := &pb.UpdateCITrustRequest{
		Id:                     "ct-own",
		StatusExchangeEndpoint: &endpoint,
		StatusWorkspace:        &workspace,
		StatusWorkflowHint:     &hint,
	}
	if err := authorizeControlPlaneGRPC(ctx, "cluster-spoke3", m, ownStatus, gateways, nil); err != nil {
		t.Errorf("own-cluster status-only UpdateCITrust should be allowed; got %v", err)
	}

	// Foreign-cluster: denied.
	foreignStatus := &pb.UpdateCITrustRequest{Id: "ct-foreign", StatusWorkspace: &workspace}
	if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-spoke3", m, foreignStatus, gateways, nil)); code != codes.PermissionDenied {
		t.Errorf("foreign-cluster UpdateCITrust should be PermissionDenied; got %v", code)
	}

	// A spec-field change (role) on an own-cluster trust is denied: not a status write.
	role := "openshell-user"
	specChange := &pb.UpdateCITrustRequest{Id: "ct-own", Role: &role, StatusWorkspace: &workspace}
	if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-spoke3", m, specChange, gateways, nil)); code != codes.PermissionDenied {
		t.Errorf("spec-field UpdateCITrust should be PermissionDenied; got %v", code)
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

	// Create and the status-only Update methods are all denied for every user caller:
	// a user never reaches the control-plane status write-back path (SEC-RBAC-27.05).
	writes := []string{
		"/hypershell.v1.CIProviderService/CreateCIProvider",
		"/hypershell.v1.CIProviderService/UpdateCIProvider",
		"/hypershell.v1.CITrustService/CreateCITrust",
		"/hypershell.v1.CITrustService/UpdateCITrust",
	}
	for _, m := range writes {
		for _, b := range [][]BindingSummary{admin, owner, ciAdmin} {
			if code := codeOf(authorizeUserGRPC(m, nil, b)); code != codes.PermissionDenied {
				t.Errorf("user write %s should be denied over gRPC (authored over HTTP); got %v", m, code)
			}
		}
	}
}
