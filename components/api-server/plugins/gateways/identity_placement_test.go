package gateways

import (
	"context"
	"net/http"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
)

func adminContext() context.Context {
	ctx := context.WithValue(context.Background(), rbac.ContextUserIDKey, "admin-user")
	return context.WithValue(ctx, rbac.ContextJWTRolesKey, []string{"platform:admin"})
}

func nonAdminContext() context.Context {
	return context.WithValue(context.Background(), rbac.ContextUserIDKey, "normal-user")
}

// Identity-selection authorization: admin allowed, non-admin and anonymous rejected,
// requirement-only selectors ungated.
// Verifies: PLAT-DATA-02, PLAT-CLUS-14.03, PLAT-CLUS-14.04
func TestAuthorizeIdentityPlacement(t *testing.T) {
	identity := openapi.GatewayPlacementIntent{Selector: []string{managedClusters.IdentityCapability("hyp0-spoke1")}}
	requirement := openapi.GatewayPlacementIntent{Selector: []string{"provider/aws", "network/public"}}

	// A platform:admin caller may place by an identity selector.
	if svcErr := authorizeIdentityPlacement(adminContext(), identity); svcErr != nil {
		t.Fatalf("platform:admin identity selector must be authorized, got %v", svcErr)
	}

	// A non-admin caller's identity selector is rejected with 403.
	svcErr := authorizeIdentityPlacement(nonAdminContext(), identity)
	if svcErr == nil {
		t.Fatal("non-admin identity selector must be rejected")
	}
	if svcErr.HttpCode != http.StatusForbidden {
		t.Fatalf("non-admin identity selector must be 403, got %d", svcErr.HttpCode)
	}

	// An unauthenticated caller (no user id) is rejected too.
	if svcErr := authorizeIdentityPlacement(context.Background(), identity); svcErr == nil {
		t.Fatal("anonymous identity selector must be rejected")
	}

	// Requirement-only selectors are unaffected for every caller.
	for _, ctx := range []context.Context{adminContext(), nonAdminContext(), context.Background()} {
		if svcErr := authorizeIdentityPlacement(ctx, requirement); svcErr != nil {
			t.Fatalf("requirement-only selector must never be gated, got %v", svcErr)
		}
	}

	// An identity reference in excludedSelector is gated the same way.
	excluded := openapi.GatewayPlacementIntent{
		Selector:         []string{"provider/aws"},
		ExcludedSelector: []string{managedClusters.IdentityCapability("hyp0-spoke1")},
	}
	if svcErr := authorizeIdentityPlacement(nonAdminContext(), excluded); svcErr == nil {
		t.Fatal("non-admin identity reference in excludedSelector must be rejected")
	}
}

// An identity selector resolves deterministically to exactly the cluster whose
// derived identity capability it names, and to no other.
// Verifies: PLAT-CLUS-14.03
func TestIdentitySelectorResolvesDeterministically(t *testing.T) {
	candidates := []PlacementCandidate{
		{ID: "id-A", Capabilities: []string{"provider/aws", "network/public", managedClusters.IdentityCapability("A")}, Connected: true},
		{ID: "id-B", Capabilities: []string{"provider/aws", "network/public", managedClusters.IdentityCapability("B")}, Connected: true},
	}

	got, err := ResolvePlacement(PlacementIntent{Selector: []string{managedClusters.IdentityCapability("A")}}, candidates)
	if err != nil {
		t.Fatalf("identity selector must resolve: %v", err)
	}
	if got != "id-A" {
		t.Fatalf("identity selector resolved to %q, want id-A", got)
	}

	// A requirement-only selector still matches both and picks one of them (random),
	// never constrained to a single cluster by identity.
	eligible := eligiblePlacementCandidates(PlacementIntent{Selector: []string{"provider/aws"}}, candidates)
	if len(eligible) != 2 {
		t.Fatalf("requirement selector must keep matching every eligible cluster, got %d", len(eligible))
	}
}
