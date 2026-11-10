package gateways

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	svcerrors "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// TestSplitManagedClusterSelector pins the partition of a mixed selector into
// managed-cluster/<name> identity names and the remaining capability labels.
func TestSplitManagedClusterSelector(t *testing.T) {
	identity, caps := splitManagedClusterSelector([]string{"managed-cluster/hyp0-mc1", "provider/aws", "managed-cluster/hyp0-mc2", "network/public"})
	if strings.Join(identity, ",") != "hyp0-mc1,hyp0-mc2" {
		t.Fatalf("identity names = %v, want [hyp0-mc1 hyp0-mc2]", identity)
	}
	if strings.Join(caps, ",") != "provider/aws,network/public" {
		t.Fatalf("capability labels = %v, want [provider/aws network/public]", caps)
	}

	identity, caps = splitManagedClusterSelector([]string{"provider/aws"})
	if len(identity) != 0 {
		t.Fatalf("identity names = %v, want none", identity)
	}
	if strings.Join(caps, ",") != "provider/aws" {
		t.Fatalf("capability labels = %v, want [provider/aws]", caps)
	}
}

// TestNormalizePlacementSelector pins identity-aware validation: identity tokens are
// validated as DNS-label names and preserved, capability labels are normalized
// through the authoritative vocabulary, the combined set is sorted and deduplicated,
// and malformed names or unknown capability namespaces are rejected.
func TestNormalizePlacementSelector(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    string // comma-joined; "" means expect an error
		wantErr bool
	}{
		{name: "identity only", input: []string{"managed-cluster/hyp0-mc1"}, want: "managed-cluster/hyp0-mc1"},
		{name: "identity and capability sorted", input: []string{"provider/aws", "managed-cluster/hyp0-mc1"}, want: "managed-cluster/hyp0-mc1,provider/aws"},
		{name: "duplicate identity collapses", input: []string{"managed-cluster/a", "managed-cluster/a"}, want: "managed-cluster/a"},
		{name: "capability only unchanged behavior", input: []string{"provider/aws", "network/public"}, want: "network/public,provider/aws"},
		{name: "invalid identity name", input: []string{"managed-cluster/Bad_Name"}, wantErr: true},
		{name: "empty identity name", input: []string{"managed-cluster/"}, wantErr: true},
		{name: "unknown capability namespace", input: []string{"managed-cluster/a", "cloud/aws"}, wantErr: true},
		{name: "empty list", input: []string{}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, svcErr := normalizePlacementSelector(tc.input)
			if tc.wantErr {
				if svcErr == nil {
					t.Fatalf("expected an error for %v, got %v", tc.input, got)
				}
				return
			}
			if svcErr != nil {
				t.Fatalf("unexpected error for %v: %v", tc.input, svcErr)
			}
			if strings.Join(got, ",") != tc.want {
				t.Fatalf("normalized = %v, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveManagedClusterIdentitySelection pins the deterministic resolution rules
// (PLAT-DATA-27): unknown name and unregistered target are both 400, the named
// cluster must satisfy the accompanying capability labels, an excluded identity
// naming the same cluster is contradictory, and a registered cluster that satisfies
// the selector resolves to its id.
func TestResolveManagedClusterIdentitySelection(t *testing.T) {
	registered := &managedClusterIdentity{ID: "mc-123", Capabilities: []string{"network/public", "provider/aws"}, Registered: true}

	tests := []struct {
		name          string
		cluster       *managedClusterIdentity
		requiredCaps  []string
		excludedNames []string
		excludedCaps  []string
		wantID        string
		wantErr       string // substring; "" means success
	}{
		{name: "unknown name", cluster: nil, wantErr: "names no ManagedCluster"},
		{name: "unregistered target", cluster: &managedClusterIdentity{ID: "mc-x", Registered: false}, wantErr: "no registered control plane"},
		{name: "resolves to id", cluster: registered, wantID: "mc-123"},
		{name: "required caps satisfied", cluster: registered, requiredCaps: []string{"provider/aws"}, wantID: "mc-123"},
		{name: "required caps not satisfied", cluster: registered, requiredCaps: []string{"provider/ibm"}, wantErr: "does not advertise every capability"},
		{name: "excluded cap present", cluster: registered, excludedCaps: []string{"provider/aws"}, wantErr: "advertises a capability the placement selector excludes"},
		{name: "excluded identity contradiction", cluster: registered, excludedNames: []string{"same"}, wantErr: "both targets and excludes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name := "same"
			id, svcErr := resolveManagedClusterIdentitySelection(name, tc.cluster, tc.requiredCaps, tc.excludedNames, tc.excludedCaps)
			if tc.wantErr != "" {
				if svcErr == nil {
					t.Fatalf("expected an error containing %q, got id %q", tc.wantErr, id)
				}
				if svcErr.HttpCode != http.StatusBadRequest {
					t.Fatalf("expected a 400 rejection, got HTTP %d (%v)", svcErr.HttpCode, svcErr)
				}
				if !strings.Contains(svcErr.Reason, tc.wantErr) {
					t.Fatalf("reason = %q, want substring %q", svcErr.Reason, tc.wantErr)
				}
				return
			}
			if svcErr != nil {
				t.Fatalf("unexpected error: %v", svcErr)
			}
			if id != tc.wantID {
				t.Fatalf("id = %q, want %q", id, tc.wantID)
			}
		})
	}
}

// TestPlacementReferencesManagedClusterSelector pins the gate's detection: a required
// or excluded managed-cluster/<name> token triggers the gate; a pure capability
// selector does not.
func TestPlacementReferencesManagedClusterSelector(t *testing.T) {
	cases := []struct {
		name  string
		req   []string
		exc   []string
		wants bool
	}{
		{name: "required identity", req: []string{"managed-cluster/a"}, wants: true},
		{name: "excluded identity", req: []string{"provider/aws"}, exc: []string{"managed-cluster/a"}, wants: true},
		{name: "capability only", req: []string{"provider/aws"}, exc: []string{"runtime/kata"}, wants: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intent := openapi.GatewayPlacementIntent{Selector: tc.req, ExcludedSelector: tc.exc}
			if got := placementReferencesManagedClusterSelector(intent); got != tc.wants {
				t.Fatalf("placementReferencesManagedClusterSelector = %t, want %t", got, tc.wants)
			}
		})
	}
	if !selectorsReferenceManagedClusterSelector([][]string{{"provider/aws"}, {"managed-cluster/a"}}) {
		t.Fatal("expected a probed managed-cluster selector to be detected")
	}
	if selectorsReferenceManagedClusterSelector([][]string{{"provider/aws"}}) {
		t.Fatal("expected a pure capability probe to not be detected")
	}
}

// TestManagedClusterSelectorAuthorize is the platform-admin gate at the create
// boundaries and the availability probe (SEC-RBAC-27): a caller with platform:admin
// passes, any other caller or anonymous is denied 403 before resolution, a create or
// probe that references no token needs no entitlement, a lookup failure is a server
// error, RBAC-off and privileged/import writes are decided elsewhere, and the gate
// applies identically to the availability probe.
func TestManagedClusterSelectorAuthorize(t *testing.T) {
	user := func() context.Context {
		return context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1")
	}
	bindingsOf := func(names ...string) []rbac.BindingSummary {
		out := make([]rbac.BindingSummary, 0, len(names))
		for _, name := range names {
			out = append(out, rbac.BindingSummary{RoleName: name, Scope: "global"})
		}
		return out
	}
	identityIntent := openapi.GatewayPlacementIntent{Selector: []string{"managed-cluster/hyp0-mc1"}}
	excludedIdentityIntent := openapi.GatewayPlacementIntent{Selector: []string{"provider/aws"}, ExcludedSelector: []string{"managed-cluster/hyp0-mc1"}}
	capabilityIntent := openapi.GatewayPlacementIntent{Selector: []string{"provider/aws"}}

	tests := []struct {
		name        string
		lookup      rbac.RoleBindingLookup
		enforceRBAC bool
		ctx         context.Context
		intent      openapi.GatewayPlacementIntent
		wantErr     string // "", "forbidden", or "server-error"
	}{
		{name: "platform:admin passes", lookup: fakeBindingLookup{bindings: bindingsOf("platform:admin")}, enforceRBAC: true, ctx: user(), intent: identityIntent},
		{name: "gateway:creator only is forbidden", lookup: fakeBindingLookup{bindings: bindingsOf("gateway:creator")}, enforceRBAC: true, ctx: user(), intent: identityIntent, wantErr: "forbidden"},
		{name: "excluded identity by non-admin is forbidden", lookup: fakeBindingLookup{bindings: bindingsOf("gateway:creator")}, enforceRBAC: true, ctx: user(), intent: excludedIdentityIntent, wantErr: "forbidden"},
		{name: "no bindings is forbidden", lookup: fakeBindingLookup{}, enforceRBAC: true, ctx: user(), intent: identityIntent, wantErr: "forbidden"},
		{name: "a capability-only selector needs no entitlement", lookup: fakeBindingLookup{bindings: bindingsOf("gateway:creator")}, enforceRBAC: true, ctx: user(), intent: capabilityIntent},
		{name: "RBAC off allows any caller", lookup: fakeBindingLookup{bindings: bindingsOf("gateway:creator")}, enforceRBAC: false, ctx: user(), intent: identityIntent},
		{name: "privileged control-plane write bypasses the check", lookup: fakeBindingLookup{bindings: bindingsOf("gateway:creator")}, enforceRBAC: true, ctx: rbac.WithGatewayWriterKind(user(), rbac.GatewayWriterControlPlane), intent: identityIntent},
		{name: "import write bypasses the user gate", lookup: fakeBindingLookup{bindings: bindingsOf("gateway:creator")}, enforceRBAC: true, ctx: rbac.WithGatewayWriterKind(user(), rbac.GatewayWriterImport), intent: identityIntent},
		{name: "anonymous caller fails closed", lookup: fakeBindingLookup{bindings: bindingsOf("platform:admin")}, enforceRBAC: true, ctx: context.Background(), intent: identityIntent, wantErr: "forbidden"},
		{name: "unwired lookup fails closed", lookup: nil, enforceRBAC: true, ctx: user(), intent: identityIntent, wantErr: "forbidden"},
		{name: "lookup failure is a server error", lookup: fakeBindingLookup{err: errors.New("database is down")}, enforceRBAC: true, ctx: user(), intent: identityIntent, wantErr: "server-error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authorizer := newManagedClusterSelectorAuthorizer(tc.lookup, tc.enforceRBAC)
			// The availability probe is gated identically to create; derive its probe
			// selectors from the same intent so both boundaries are covered together.
			probe := [][]string{tc.intent.Selector}
			if len(tc.intent.ExcludedSelector) > 0 {
				probe = append(probe, tc.intent.ExcludedSelector)
			}
			results := map[string]*svcerrors.ServiceError{
				"create":       authorizer.authorizeCreate(tc.ctx, tc.intent),
				"availability": authorizer.authorizeAvailability(tc.ctx, probe),
			}
			for label, svcErr := range results {
				switch tc.wantErr {
				case "":
					if svcErr != nil {
						t.Fatalf("[%s] expected authorization, got %v", label, svcErr)
					}
				case "forbidden":
					if svcErr == nil || !svcErr.IsForbidden() {
						t.Fatalf("[%s] expected Forbidden, got %v", label, svcErr)
					}
					if svcErr.Reason != managedClusterSelectorForbiddenReason {
						t.Fatalf("[%s] reason = %q, want %q", label, svcErr.Reason, managedClusterSelectorForbiddenReason)
					}
					if !strings.Contains(svcErr.Reason, "platform:admin") {
						t.Fatalf("[%s] reason %q must name platform:admin", label, svcErr.Reason)
					}
				case "server-error":
					if svcErr == nil || svcErr.IsForbidden() {
						t.Fatalf("[%s] expected a server error, got %v", label, svcErr)
					}
				default:
					t.Fatalf("unknown wantErr %q", tc.wantErr)
				}
			}
		})
	}
}
