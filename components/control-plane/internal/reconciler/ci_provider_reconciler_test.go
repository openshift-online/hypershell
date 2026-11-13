package reconciler

import (
	"context"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
)

// resolveIdpAlias prefers the explicit spec idp_alias over the derived alias, and
// falls back to the derived alias only when idp_alias is unset.
// Verifies: PLAT-CITR-01.03, PLAT-DATA-24.03
func TestResolveIdpAlias(t *testing.T) {
	const issuer = "https://token.actions.githubusercontent.com"
	derived := deriveIdpAlias("github", issuer) // ci-github-token-actions-githubusercontent-com

	t.Run("explicit idp_alias is preferred over the derived alias", func(t *testing.T) {
		p := &pb.CIProvider{Forge: "github", Issuer: issuer, IdpAlias: "github-actions"}
		if got := resolveIdpAlias(p); got != "github-actions" {
			t.Fatalf("resolveIdpAlias = %q, want %q", got, "github-actions")
		}
		if derived == "github-actions" {
			t.Fatalf("test precondition: derived alias unexpectedly equals the explicit alias")
		}
	})

	t.Run("unset idp_alias falls back to the derived alias", func(t *testing.T) {
		p := &pb.CIProvider{Forge: "github", Issuer: issuer}
		if got := resolveIdpAlias(p); got != derived {
			t.Fatalf("resolveIdpAlias = %q, want derived %q", got, derived)
		}
	})
}

// recordingCIProviderSvc counts UpdateCIProvider calls.
type recordingCIProviderSvc struct {
	pb.CIProviderServiceClient
	updates []*pb.UpdateCIProviderRequest
}

func (f *recordingCIProviderSvc) UpdateCIProvider(_ context.Context, req *pb.UpdateCIProviderRequest, _ ...grpc.CallOption) (*pb.UpdateCIProviderResponse, error) {
	f.updates = append(f.updates, req)
	return &pb.UpdateCIProviderResponse{}, nil
}

// The status write-back must happen only when status.idp_alias changes. Each
// write emits a watch event, so writing an unchanged alias loops forever.
// Verifies: STD-RCON-02.01
func TestCIProviderUpsert_WritesStatusOnlyWhenChanged(t *testing.T) {
	cases := []struct {
		name        string
		status      *string
		wantUpdates int
	}{
		{name: "status already current", status: strptr("github-actions"), wantUpdates: 0},
		{name: "status unset", status: nil, wantUpdates: 1},
		{name: "status stale", status: strptr("ci-github-old"), wantUpdates: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &recordingCIProviderSvc{}
			r := &CIProviderReconciler{ciProviderSvc: svc}
			provider := &pb.CIProvider{
				Metadata:       &pb.ObjectReference{Id: "prov-1"},
				Forge:          "github",
				Issuer:         "https://token.actions.githubusercontent.com",
				IdpAlias:       "github-actions",
				StatusIdpAlias: tc.status,
			}
			err := r.Handle(t.Context(), watcher.Event[*pb.CIProvider]{Type: watcher.EventUpdated, ResourceID: "prov-1", Resource: provider})
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if len(svc.updates) != tc.wantUpdates {
				t.Fatalf("UpdateCIProvider calls = %d, want %d", len(svc.updates), tc.wantUpdates)
			}
			if tc.wantUpdates == 1 && svc.updates[0].GetStatusIdpAlias() != "github-actions" {
				t.Errorf("status_idp_alias = %q, want github-actions", svc.updates[0].GetStatusIdpAlias())
			}
		})
	}
}
