package gateways_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"gopkg.in/resty.v1"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/test"
	trexapi "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// registerNamedCluster registers a ManagedCluster with the given capabilities under a
// unique subject and returns its id. The name is a DNS-label so it can be targeted by
// the managed-cluster/<name> identity selector.
func registerNamedCluster(t *testing.T, name string, capabilities []string) string {
	t.Helper()
	svc := managedClusters.Service(&environments.Environment().Services)
	cluster, _, svcErr := svc.Register(context.Background(), name, capabilities, "cp-"+strings.ToLower(trexapi.NewID()))
	Expect(svcErr).To(BeNil(), "register named cluster %s", name)
	return cluster.ID
}

// TestGatewayPostResolvesManagedClusterIdentitySelector proves the REST create path
// resolves a managed-cluster/<name> identity token deterministically to the named
// cluster's id, both alone and combined with a capability label the cluster
// satisfies.
// Verifies: PLAT-DATA-27.01, WEB-PLC-07.06
func TestGatewayPostResolvesManagedClusterIdentitySelector(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	suffix := strings.ToLower(trexapi.NewID())
	name := "idmc-" + suffix
	clusterID := registerNamedCluster(t, name, []string{"network/public", "provider/aws"})

	input := openapi.GatewayCreateRequest{Name: "id-rest-" + suffix}
	input.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{"managed-cluster/" + name}})
	out, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(input).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(out.ClusterId).To(Equal(clusterID), "identity selector must resolve to the named cluster")

	combined := openapi.GatewayCreateRequest{Name: "id-rest-combined-" + suffix}
	combined.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{"managed-cluster/" + name, "provider/aws"}})
	outCombined, respCombined, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(combined).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(respCombined.StatusCode).To(Equal(http.StatusCreated))
	Expect(outCombined.ClusterId).To(Equal(clusterID), "identity selector combined with a satisfied capability must still resolve")
}

// TestGatewayPostRejectsUnresolvableManagedClusterIdentity proves the identity
// selector fails fast rather than late-binding: an unknown name is a 400, an
// unregistered placeholder (empty oidc_subject) is a 400 naming the missing control
// plane, and a capability the named cluster does not satisfy is a 400.
// Verifies: PLAT-DATA-27.03
func TestGatewayPostRejectsUnresolvableManagedClusterIdentity(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	suffix := strings.ToLower(trexapi.NewID())

	// Unknown name.
	unknown := openapi.GatewayCreateRequest{Name: "id-unknown-" + suffix}
	unknown.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{"managed-cluster/absent-" + suffix}})
	_, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(unknown).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	// A manual placeholder (empty oidc_subject) is not a registered control plane.
	svc := managedClusters.Service(&environments.Environment().Services)
	placeholderName := "idplaceholder-" + suffix
	_, createErr := svc.Create(context.Background(), &managedClusters.ManagedCluster{Name: placeholderName})
	Expect(createErr).To(BeNil())
	unregistered := openapi.GatewayCreateRequest{Name: "id-unregistered-" + suffix}
	unregistered.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{"managed-cluster/" + placeholderName}})
	_, respUnreg, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(unregistered).Execute()
	Expect(err).To(HaveOccurred())
	Expect(respUnreg.StatusCode).To(Equal(http.StatusBadRequest))

	// A registered cluster that does not satisfy an accompanying capability label.
	name := "idcap-" + suffix
	registerNamedCluster(t, name, []string{"network/public", "provider/aws"})
	mismatch := openapi.GatewayCreateRequest{Name: "id-capmiss-" + suffix}
	mismatch.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{"managed-cluster/" + name, "provider/ibm"}})
	_, respMismatch, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(mismatch).Execute()
	Expect(err).To(HaveOccurred())
	Expect(respMismatch.StatusCode).To(Equal(http.StatusBadRequest))
}

// TestGRPCCreateGatewayResolvesManagedClusterIdentitySelector proves the gRPC create
// boundary resolves the identity token to the named cluster and rejects an unknown
// name with InvalidArgument.
// Verifies: PLAT-DATA-27.01, PLAT-DATA-27.03
func TestGRPCCreateGatewayResolvesManagedClusterIdentitySelector(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	grpcClient := newGRPCGatewayClient(t, h)

	suffix := strings.ToLower(trexapi.NewID())
	name := "grpcidmc-" + suffix
	clusterID := registerNamedCluster(t, name, []string{"network/public", "provider/aws"})

	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name: "grpc-id-" + suffix,
		Placement: &pb.GatewayPlacementIntent{Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
			CapabilitySelector: &pb.CapabilitySelectorPlacement{Selector: []string{"managed-cluster/" + name}},
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Gateway.ClusterId).To(Equal(clusterID))

	_, err = grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name: "grpc-id-absent-" + suffix,
		Placement: &pb.GatewayPlacementIntent{Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
			CapabilitySelector: &pb.CapabilitySelectorPlacement{Selector: []string{"managed-cluster/absent-" + suffix}},
		}},
	})
	Expect(err).To(HaveOccurred())
	Expect(grpcstatus.Code(err)).To(Equal(codes.InvalidArgument))
}

// TestPlacementAvailabilityResolvesManagedClusterIdentity proves the availability
// probe resolves a managed-cluster/<name> selector by name rather than rejecting it
// as an unknown label: it reports available for a registered cluster and unavailable
// for an unknown name.
// Verifies: WEB-PLC-07.06
func TestPlacementAvailabilityResolvesManagedClusterIdentity(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	suffix := strings.ToLower(trexapi.NewID())
	name := "availidmc-" + suffix
	registerNamedCluster(t, name, []string{"network/public", "provider/aws"})
	jwtToken := ctx.Value(openapi.ContextAccessToken)

	probe := func(selector string) string {
		resp, err := resty.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
			SetBody(fmt.Sprintf(`{"selectors":[[%q]]}`, selector)).
			Post(h.RestURL("/gateways/placement-availability"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK), "body = %s", string(resp.Body()))
		return string(resp.Body())
	}

	Expect(probe("managed-cluster/"+name)).To(ContainSubstring(`"available":true`), "a registered named cluster must be available")
	Expect(probe("managed-cluster/absent-"+suffix)).To(ContainSubstring(`"available":false`), "an unknown named cluster must be unavailable, not a 400")
}

// TestManagedClusterRejectsIdentityNamespaceAsStoredCapability proves managed-cluster/
// is never a stored capability: a registration that submits it among capabilities is
// rejected 400 like any unknown namespace, and a valid registration never advertises
// a managed-cluster/ label.
// Verifies: PLAT-CLUS-06.04, PLAT-DATA-27.04
func TestManagedClusterRejectsIdentityNamespaceAsStoredCapability(t *testing.T) {
	test.RegisterIntegration(t)
	svc := managedClusters.Service(&environments.Environment().Services)
	suffix := strings.ToLower(trexapi.NewID())

	_, _, svcErr := svc.Register(context.Background(), "capmc-"+suffix, []string{"network/public", "managed-cluster/foo"}, "cp-cap-"+suffix)
	Expect(svcErr).NotTo(BeNil(), "a managed-cluster/ capability must be rejected")
	Expect(svcErr.HttpCode).To(Equal(http.StatusBadRequest))

	cluster, _, svcErr := svc.Register(context.Background(), "capmc2-"+suffix, []string{"network/public", "provider/aws"}, "cp-cap2-"+suffix)
	Expect(svcErr).To(BeNil())
	for _, capability := range cluster.Capabilities {
		Expect(strings.HasPrefix(capability, "managed-cluster/")).To(BeFalse(), "capabilities must never contain a managed-cluster/ label, got %q", capability)
	}
}
