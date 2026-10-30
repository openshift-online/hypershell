package rbac

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// TestAuthorizeUserGRPC_ImportKinds pins the gRPC authorization for the
// declarative-import kinds (#525 REPO-C-GRPC): their read methods are
// platform-admin-only (unfiltered by caller) and every write over gRPC is the
// control-plane reconciler's exclusive path, denied to users.
// Verifies: SEC-RBAC-09.05
func TestAuthorizeUserGRPC_ImportKinds(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		req      interface{}
		bindings []BindingSummary
		want     codes.Code
	}{
		{"admin lists repositories", "/hypershell.v1.RepositoryService/ListRepositories", &pb.ListRepositoriesRequest{}, []BindingSummary{adminBinding}, codes.OK},
		{"admin gets a repository", "/hypershell.v1.RepositoryService/GetRepository", &pb.GetRepositoryRequest{Id: "r-1"}, []BindingSummary{adminBinding}, codes.OK},
		{"admin watches secret sources", "/hypershell.v1.SecretSourceService/WatchSecretSources", nil, []BindingSummary{adminBinding}, codes.OK},
		{"creator cannot list repositories", "/hypershell.v1.RepositoryService/ListRepositories", &pb.ListRepositoriesRequest{}, []BindingSummary{creatorBinding}, codes.PermissionDenied},
		{"admin cannot create a repository over gRPC", "/hypershell.v1.RepositoryService/CreateRepository", &pb.CreateRepositoryRequest{}, []BindingSummary{adminBinding}, codes.PermissionDenied},
		{"admin cannot delete a secret source over gRPC", "/hypershell.v1.SecretSourceService/DeleteSecretSource", &pb.DeleteSecretSourceRequest{Id: "s-1"}, []BindingSummary{adminBinding}, codes.PermissionDenied},
		{"admin lists sandbox templates", "/hypershell.v1.SandboxTemplateService/ListSandboxTemplates", &pb.ListSandboxTemplatesRequest{}, []BindingSummary{adminBinding}, codes.OK},
		{"admin lists provider specs", "/hypershell.v1.ProviderSpecService/ListProviderSpecs", &pb.ListProviderSpecsRequest{}, []BindingSummary{adminBinding}, codes.OK},
		{"admin lists provider bindings", "/hypershell.v1.ProviderBindingService/ListProviderBindings", &pb.ListProviderBindingsRequest{}, []BindingSummary{adminBinding}, codes.OK},
		{"admin lists inference routes", "/hypershell.v1.InferenceRouteService/ListInferenceRoutes", &pb.ListInferenceRoutesRequest{}, []BindingSummary{adminBinding}, codes.OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(authorizeUserGRPC(tt.method, tt.req, tt.bindings)); got != tt.want {
				t.Fatalf("authorizeUserGRPC(%s) = %s, want %s", tt.method, got, tt.want)
			}
		})
	}
}

// TestAuthorizeControlPlaneGRPC_ImportKinds confirms the registered control plane
// may READ every import kind (list/watch/get), and that a write WITHOUT a
// reconciled-Repository binding (no import metadata) is denied: an owned-kind write
// needs the import context (see TestAuthorizeControlPlaneGRPC_ImportWriteBinding),
// and Repository writes stay denied entirely. gateways and imports are nil here: no
// per-resource cluster check applies and, with no metadata, the import resolver is
// never consulted.
func TestAuthorizeControlPlaneGRPC_ImportKinds(t *testing.T) {
	const clusterID = "cluster-1"
	reads := []struct {
		method string
		req    interface{}
	}{
		{"/hypershell.v1.RepositoryService/ListRepositories", &pb.ListRepositoriesRequest{}},
		{"/hypershell.v1.RepositoryService/WatchRepositories", nil},
		{"/hypershell.v1.RepositoryService/GetRepository", &pb.GetRepositoryRequest{Id: "r-1"}},
		{"/hypershell.v1.SecretSourceService/WatchSecretSources", nil},
		{"/hypershell.v1.SandboxTemplateService/ListSandboxTemplates", &pb.ListSandboxTemplatesRequest{}},
	}
	for _, m := range reads {
		t.Run("read "+m.method, func(t *testing.T) {
			if err := authorizeControlPlaneGRPC(context.Background(), clusterID, m.method, m.req, nil, nil); err != nil {
				t.Fatalf("authorizeControlPlaneGRPC(%s) = %v, want nil", m.method, err)
			}
		})
	}

	writes := []struct {
		method string
		req    interface{}
	}{
		{"/hypershell.v1.RepositoryService/CreateRepository", &pb.CreateRepositoryRequest{}},
		{"/hypershell.v1.RepositoryService/UpdateRepository", &pb.UpdateRepositoryRequest{Id: "r-1"}},
		{"/hypershell.v1.RepositoryService/DeleteRepository", &pb.DeleteRepositoryRequest{Id: "r-1"}},
		{"/hypershell.v1.SandboxTemplateService/CreateSandboxTemplate", &pb.CreateSandboxTemplateRequest{}},
		{"/hypershell.v1.ProviderSpecService/DeleteProviderSpec", &pb.DeleteProviderSpecRequest{Id: "p-1"}},
		{"/hypershell.v1.ProviderBindingService/UpdateProviderBinding", &pb.UpdateProviderBindingRequest{Id: "b-1"}},
		{"/hypershell.v1.InferenceRouteService/CreateInferenceRoute", &pb.CreateInferenceRouteRequest{}},
		{"/hypershell.v1.SecretSourceService/CreateSecretSource", &pb.CreateSecretSourceRequest{}},
	}
	for _, m := range writes {
		t.Run("write denied "+m.method, func(t *testing.T) {
			if err := authorizeControlPlaneGRPC(context.Background(), clusterID, m.method, m.req, nil, nil); status.Code(err) != codes.PermissionDenied {
				t.Fatalf("authorizeControlPlaneGRPC(%s) = %s, want PermissionDenied", m.method, status.Code(err))
			}
		})
	}
}
