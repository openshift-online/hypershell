package gateways_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/test"
	trexapi "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

func newGRPCGatewayClient(t *testing.T, h *test.Helper) pb.GatewayServiceClient {
	t.Helper()
	account := h.NewRandAccount()
	jwtToken := h.CreateJWTString(account)
	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { Expect(conn.Close()).To(Succeed()) })
	return pb.NewGatewayServiceClient(conn)
}

// A selector that matches no connected cluster late-binds rather than fails: the
// gateway is created Pending with an awaiting-placement status and its placement
// persisted, so the declarative .hypershell import never hits "cluster_id is
// required" (data-model.spec.md, "Gateway Placement and Late Binding"; GP-07).
func TestGRPCCreateGatewayLateBindsWhenNoClusterMatches(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	grpcClient := newGRPCGatewayClient(t, h)

	suffix := strings.ToLower(trexapi.NewID())
	label := "runtime/nomatch-" + suffix
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name: "late-bind-" + suffix,
		Placement: &pb.GatewayPlacementIntent{Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
			CapabilitySelector: &pb.CapabilitySelectorPlacement{Selector: []string{label}},
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Gateway.ClusterId).To(BeEmpty())
	Expect(created.Gateway.GetPhase()).To(Equal(string(gatewayhealth.PhasePending)))
	Expect(created.Gateway.GetStatus()).To(ContainSubstring("Awaiting placement"))
}

// An unplaced gateway binds when a ManagedCluster that matches its selector
// registers: the registration event drives the API server's re-resolution, which
// assigns cluster_id (data-model.spec.md, "Unplaced Gateway binds when a matching
// cluster appears").
func TestLateBoundGatewayBindsWhenMatchingClusterRegisters(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	grpcClient := newGRPCGatewayClient(t, h)

	suffix := strings.ToLower(trexapi.NewID())
	label := "runtime/bindtest-" + suffix
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name: "bind-" + suffix,
		Placement: &pb.GatewayPlacementIntent{Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
			CapabilitySelector: &pb.CapabilitySelectorPlacement{Selector: []string{"network/public", "provider/aws", label}},
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Gateway.ClusterId).To(BeEmpty())

	// Register a cluster that carries the unique label, which emits a registration
	// event the GatewayPlacementRebinder handles.
	clusterService := managedClusters.Service(&environments.Environment().Services)
	cluster, _, svcErr := clusterService.Register(ctx, "bindcluster-"+suffix, []string{"network/public", "provider/aws", label}, "cp-"+suffix)
	Expect(svcErr).To(BeNil())

	Eventually(func() string {
		got, getErr := grpcClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: created.Gateway.Metadata.Id})
		if getErr != nil {
			return ""
		}
		return got.Gateway.ClusterId
	}, 10*time.Second, 200*time.Millisecond).Should(Equal(cluster.ID), "late-bound gateway should be assigned the newly registered matching cluster")
}

func TestGRPCCreateGatewayRejectsBothClusterAndPlacement(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	grpcClient := newGRPCGatewayClient(t, h)

	_, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:      "both-" + strings.ToLower(trexapi.NewID()),
		ClusterId: registerTestCluster(t),
		Placement: &pb.GatewayPlacementIntent{Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
			CapabilitySelector: &pb.CapabilitySelectorPlacement{Selector: []string{"network/public", "provider/aws"}},
		}},
	})
	Expect(err).To(HaveOccurred())
	Expect(grpcstatus.Code(err)).To(Equal(codes.InvalidArgument))
}

func TestGRPCCreateGatewayRejectsNeitherClusterNorPlacement(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	grpcClient := newGRPCGatewayClient(t, h)

	_, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name: "neither-" + strings.ToLower(trexapi.NewID()),
	})
	Expect(err).To(HaveOccurred())
	Expect(grpcstatus.Code(err)).To(Equal(codes.InvalidArgument))
}

// A pinned cluster_id (no placement) is accepted on the REST create path, so
// hsctl apply and the declarative import can pin a gateway explicitly.
func TestGatewayPostAcceptsPinnedClusterID(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	clusterID := registerTestCluster(t)
	input := openapi.GatewayCreateRequest{Name: "pinned-" + strings.ToLower(trexapi.NewID())}
	input.SetClusterId(clusterID)

	out, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(input).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(out.ClusterId).To(Equal(clusterID))
}

// The REST create path late-binds a selector that matches no cluster, mirroring
// the gRPC behavior exercised above.
func TestGatewayPostLateBindsWhenNoClusterMatches(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	label := "runtime/nomatch-" + strings.ToLower(trexapi.NewID())
	input := openapi.GatewayCreateRequest{Name: "late-rest-" + strings.ToLower(trexapi.NewID())}
	input.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{label}})

	out, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(input).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(out.ClusterId).To(BeEmpty())
	Expect(out.GetPhase()).To(Equal(string(gatewayhealth.PhasePending)))
	Expect(out.GetStatus()).To(ContainSubstring("Awaiting placement"))
}

// A REST create with neither cluster_id nor placement is rejected.
func TestGatewayPostRejectsNeitherClusterNorPlacement(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	input := openapi.GatewayCreateRequest{Name: "rest-neither-" + strings.ToLower(trexapi.NewID())}
	_, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(input).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
}
