package roleBindings

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server/grpcutil"
)

// readTrackingService embeds RoleBindingService so it satisfies the interface
// without implementing every method, and records whether either list read was
// invoked. Only the read paths ListRoleBindings can reach are overridden; any
// other call panics (nil embedded interface) and fails the test loudly.
type readTrackingService struct {
	RoleBindingService
	findByUserIDCalled bool
	allCalled          bool
}

func (s *readTrackingService) FindByUserID(context.Context, string) (RoleBindingList, *errors.ServiceError) {
	s.findByUserIDCalled = true
	return RoleBindingList{}, nil
}

func (s *readTrackingService) All(context.Context) (RoleBindingList, *errors.ServiceError) {
	s.allCalled = true
	return RoleBindingList{}, nil
}

// PLAT-CLUS-15.01: a ListRoleBindings request with an empty user_id and a
// gateway_id is rejected with InvalidArgument and reads no role bindings.
func TestListRoleBindingsFailsClosedWithoutUser(t *testing.T) {
	svc := &readTrackingService{}
	lookup := func(context.Context, string) (string, bool, *errors.ServiceError) { return "", false, nil }
	h := &roleBindingGRPCHandler{service: svc, gatewayClusters: lookup}

	gatewayID := "gw-x"
	clusterID := "cluster-x"
	_, err := h.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{
		GatewayId: &gatewayID,
		ClusterId: &clusterID,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty user_id with gateway_id: code = %s, want InvalidArgument (err=%v)", status.Code(err), err)
	}
	if svc.findByUserIDCalled || svc.allCalled {
		t.Fatalf("rejected request read role bindings (FindByUserID=%v, All=%v); want no read", svc.findByUserIDCalled, svc.allCalled)
	}

	// The same empty user_id with no gateway_id is the cluster-wide resync path:
	// it reaches the all-binding read.
	_, err = h.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{ClusterId: &clusterID})
	if err != nil {
		t.Fatalf("cluster-wide resync: unexpected error %v", err)
	}
	if !svc.allCalled {
		t.Fatal("cluster-wide resync did not reach the all-binding read")
	}
}

func TestLoadRoleBindingWithRetryRetriesNotFound(t *testing.T) {
	attempts := 0
	want := &RoleBinding{}
	got, err := loadRoleBindingWithRetry(context.Background(), func() (*RoleBinding, *errors.ServiceError) {
		attempts++
		if attempts < 3 {
			return nil, errors.NotFound("pending")
		}
		return want, nil
	})
	if err != nil {
		t.Fatalf("loadRoleBindingWithRetry() error = %v", err)
	}
	if got != want {
		t.Fatalf("loadRoleBindingWithRetry() = %v, want the committed binding", got)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestLoadRoleBindingWithRetryDoesNotRetryOtherErrors(t *testing.T) {
	attempts := 0
	_, err := loadRoleBindingWithRetry(context.Background(), func() (*RoleBinding, *errors.ServiceError) {
		attempts++
		return nil, errors.GeneralError("boom")
	})
	if err == nil {
		t.Fatal("loadRoleBindingWithRetry() error = nil, want GeneralError")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on non-NotFound)", attempts)
	}
}

func strPtr(s string) *string { return &s }

func TestNewClusterScope(t *testing.T) {
	lookup := func(context.Context, string) (string, bool, *errors.ServiceError) { return "", false, nil }

	h := &roleBindingGRPCHandler{gatewayClusters: lookup}
	if scope, err := h.newClusterScope(""); err != nil || scope != nil {
		t.Fatalf("newClusterScope(\"\") = %v, %v; want nil scope (unfiltered)", scope, err)
	}
	if _, err := h.newClusterScope(strings.Repeat("c", grpcutil.MaxStringFieldLength+1)); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("over-length cluster_id: code = %s, want InvalidArgument", status.Code(err))
	}

	// Fail closed: a filtered request cannot be served without the lookup.
	unwired := &roleBindingGRPCHandler{}
	if _, err := unwired.newClusterScope("cluster-x"); status.Code(err) != codes.Unavailable {
		t.Fatalf("nil lookup: code = %s, want Unavailable", status.Code(err))
	}
	if scope, err := unwired.newClusterScope(""); err != nil || scope != nil {
		t.Fatalf("nil lookup, unfiltered: %v, %v; want nil scope and no error", scope, err)
	}
}

func TestClusterScopeIncludes(t *testing.T) {
	assignments := map[string]string{"gw-x": "cluster-x", "gw-y": "cluster-y"}
	calls := map[string]int{}
	lookup := func(_ context.Context, gatewayID string) (string, bool, *errors.ServiceError) {
		calls[gatewayID]++
		if gatewayID == "gw-broken" {
			return "", false, errors.GeneralError("db down")
		}
		clusterID, ok := assignments[gatewayID]
		return clusterID, ok, nil
	}
	h := &roleBindingGRPCHandler{gatewayClusters: lookup}
	scope, err := h.newClusterScope("cluster-x")
	if err != nil || scope == nil {
		t.Fatalf("newClusterScope: %v, %v", scope, err)
	}

	cases := []struct {
		name         string
		rb           *RoleBinding
		wantIncluded bool
		wantFound    bool
		wantErr      bool
	}{
		{name: "own cluster gateway", rb: &RoleBinding{GatewayID: strPtr("gw-x")}, wantIncluded: true, wantFound: true},
		{name: "foreign cluster gateway", rb: &RoleBinding{GatewayID: strPtr("gw-y")}, wantIncluded: false, wantFound: true},
		{name: "global binding", rb: &RoleBinding{}, wantIncluded: false, wantFound: true},
		{name: "empty gateway id", rb: &RoleBinding{GatewayID: strPtr("")}, wantIncluded: false, wantFound: true},
		{name: "missing gateway", rb: &RoleBinding{GatewayID: strPtr("gw-gone")}, wantIncluded: false, wantFound: false},
		{name: "lookup failure", rb: &RoleBinding{GatewayID: strPtr("gw-broken")}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			included, found, lookupErr := scope.includes(context.Background(), tc.rb)
			if (lookupErr != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", lookupErr, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if included != tc.wantIncluded || found != tc.wantFound {
				t.Fatalf("includes = (%v, %v), want (%v, %v)", included, found, tc.wantIncluded, tc.wantFound)
			}
		})
	}

	// Resolved assignments are cached for the scope's lifetime (one list or
	// one replay); a failed lookup is not cached.
	if _, _, err := scope.includes(context.Background(), &RoleBinding{GatewayID: strPtr("gw-x")}); err != nil {
		t.Fatalf("cached lookup: %v", err)
	}
	if calls["gw-x"] != 1 || calls["gw-gone"] != 1 {
		t.Fatalf("lookup calls = %v, want gw-x and gw-gone resolved once", calls)
	}
	if _, _, err := scope.includes(context.Background(), &RoleBinding{GatewayID: strPtr("gw-broken")}); err == nil || calls["gw-broken"] != 2 {
		t.Fatalf("failed lookup was cached (calls=%d, err=%v)", calls["gw-broken"], err)
	}
}
