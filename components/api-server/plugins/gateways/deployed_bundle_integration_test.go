package gateways_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// The control plane records deployed_bundle with observed_generation after a
// successful pass; the write never advances generation, and a user caller may
// not set it.
// Verifies: PLAT-DATA-34.01, PLAT-DATA-23.06
func TestGRPCControlPlaneWritesDeployedBundle(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	cp, clusterID := newControlPlaneGatewayClient(t, h)
	ctx := context.Background()
	created, err := cp.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "bundle-write", ClusterId: clusterID})
	Expect(err).NotTo(HaveOccurred())
	id := created.GetGateway().GetMetadata().GetId()
	generation := created.GetGateway().GetGeneration()

	bundle := "hypershell-2026.11.27-1"
	updated, err := cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: id, ObservedGeneration: &generation, DeployedBundle: &bundle})
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.GetGateway().GetDeployedBundle()).To(Equal(bundle))
	Expect(updated.GetGateway().GetGeneration()).To(Equal(generation))
	Expect(updated.GetGateway().GetObservedGeneration()).To(Equal(generation))
	Expect(updated.GetGateway().TargetBundle).To(BeNil())

	user := newGRPCGatewayClient(t, h)
	userGW, err := user.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "bundle-user", ClusterId: registerTestCluster(t)})
	Expect(err).NotTo(HaveOccurred())
	forged := "forged"
	_, err = user.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: userGW.GetGateway().GetMetadata().GetId(), DeployedBundle: &forged})
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied), "user write of deployed_bundle: %v", err)
	Expect(err.Error()).To(ContainSubstring("deployed_bundle"))
}
