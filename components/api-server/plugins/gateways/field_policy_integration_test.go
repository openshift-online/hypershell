package gateways_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestGRPCUserGatewayWriteRejectsLockedFields proves the default-deny field policy over
// the gRPC whole-row write path for a user caller: a user may set only name +
// placement/cluster_id on create and only name on update; any platform-managed or
// observed field is rejected with PermissionDenied naming the field.
// Verifies: SEC-RBAC-18.01, PLAT-DATA-23.01, PLAT-DATA-23.04
func TestGRPCUserGatewayWriteRejectsLockedFields(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	user := newGRPCGatewayClient(t, h)
	ctx := context.Background()
	clusterID := registerTestCluster(t)

	// A user create with a platform-managed field is rejected naming the field.
	img := "quay.io/example/image:v1"
	_, err := user.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "user-locked-create", ClusterId: clusterID, Image: &img})
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied), "user create with image: %v", err)
	Expect(err.Error()).To(ContainSubstring("image"))

	// A user create with only name + cluster is accepted.
	created, err := user.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "user-ok-create", ClusterId: clusterID})
	Expect(err).NotTo(HaveOccurred())

	// A user update changing a locked field (image) is rejected naming the field.
	_, err = user.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: created.Gateway.Metadata.Id, Image: &img})
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied), "user update of image: %v", err)
	Expect(err.Error()).To(ContainSubstring("image"))

	// A user rename is accepted.
	newName := "user-renamed"
	_, err = user.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: created.Gateway.Metadata.Id, Name: &newName})
	Expect(err).NotTo(HaveOccurred())
}

// TestGRPCImportGatewayWriteRejectsLockedFields proves the import caller is held to the
// same allowlist as a user: an import-marked gRPC create that declares a platform-managed
// field is rejected, exactly as for a direct user. The import caller kind is identified by
// the reconciled-Repository metadata.
// Verifies: SEC-RBAC-18.02, PLAT-DATA-23.05
func TestGRPCImportGatewayWriteRejectsLockedFields(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	client := newGRPCGatewayClient(t, h)
	clusterID := registerTestCluster(t)
	// Mark the call as a declarative-import write by carrying the reconciled-Repository id
	// in the trusted gRPC metadata the import reconciler uses.
	ctx := metadata.AppendToOutgoingContext(context.Background(), rbac.ImportRepositoryMetadataKey, "repo-under-test")

	oidc := `{"issuer":"https://kc"}`
	_, err := client.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "import-locked-create", ClusterId: clusterID, Oidc: &oidc})
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied), "import create with oidc: %v", err)
	Expect(err.Error()).To(ContainSubstring("oidc"))
}
