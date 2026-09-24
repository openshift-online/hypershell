package gateways_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func TestGatewayGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, _, err := client.DefaultAPI.GetGateway(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	_, resp, err := client.DefaultAPI.GetGateway(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	gatewayModel, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	gatewayOutput, resp, err := client.DefaultAPI.GetGateway(ctx, gatewayModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	Expect(*gatewayOutput.Id).To(Equal(gatewayModel.ID), "found object does not match test object")
	Expect(*gatewayOutput.Kind).To(Equal("Gateway"))
	Expect(*gatewayOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateways/%s", gatewayModel.ID)))
	Expect(*gatewayOutput.CreatedAt).To(BeTemporally("~", gatewayModel.CreatedAt))
	Expect(*gatewayOutput.UpdatedAt).To(BeTemporally("~", gatewayModel.UpdatedAt))
	Expect(gatewayOutput.GetGatewayVersion()).To(Equal("0.0.109"))
}

func TestGatewayPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayInput := openapi.GatewayCreateRequest{
		Name:        "test-name",
		ClusterId:   registerTestCluster(t),
		ReleaseId:   "test-release_id",
		ExternalDns: openapi.PtrString("test-external_dns"),
		TlsMode:     openapi.PtrString("test-tls_mode"),
		ServiceType: openapi.PtrString("test-service_type"),
		Status:      openapi.PtrString("test-status"),
		Phase:       openapi.PtrString("Provisioning"),
	}

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*gatewayOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*gatewayOutput.Kind).To(Equal("Gateway"))
	Expect(*gatewayOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateways/%s", *gatewayOutput.Id)))
	Expect(gatewayOutput.Namespace).To(MatchRegexp(`^openshell-[0-9a-f]{16}$`))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/gateways"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestGatewayPostAllowsEmptyReleaseID(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	clusterID := registerTestCluster(t)
	gatewayInput := openapi.GatewayCreateRequest{
		Name:      "local-gateway",
		ClusterId: clusterID,
		ReleaseId: "",
	}

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting gateway with empty release_id: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gatewayOutput.ClusterId).To(Equal(clusterID))
	Expect(gatewayOutput.ReleaseId).To(BeEmpty())
	Expect(gatewayOutput.Namespace).To(MatchRegexp(`^openshell-[0-9a-f]{16}$`))
}

// Gateways Reference a Registered Cluster: an empty cluster_id is a 400 naming
// cluster_id, and no gateway is created.
func TestGatewayPostRejectsEmptyClusterID(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(openapi.GatewayCreateRequest{
		Name:      "no-cluster",
		ClusterId: "",
	}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	Expect(openapiErrorReason(h, err)).To(ContainSubstring("cluster_id"))
	expectNoGatewayNamed(ctx, client, "no-cluster")
}

// Gateways Reference a Registered Cluster: a cluster_id naming a manually
// created ManagedCluster (empty oidc_subject) or no ManagedCluster at all is a
// 400 stating the cluster has no registered control plane.
func TestGatewayPostRejectsUnregisteredCluster(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	for _, clusterID := range []string{createManualCluster(t), "2doesnotexist000000000000000"} {
		_, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(openapi.GatewayCreateRequest{
			Name:      "unregistered-cluster",
			ClusterId: clusterID,
		}).Execute()
		Expect(err).To(HaveOccurred(), "cluster_id %s must be rejected", clusterID)
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		Expect(openapiErrorReason(h, err)).To(ContainSubstring("registered control plane"))
	}
	expectNoGatewayNamed(ctx, client, "unregistered-cluster")
}

// A PATCH that moves a gateway to an unregistered cluster is rejected; one that
// re-sends the stored cluster_id is not a reassignment and is accepted.
func TestGatewayPatchValidatesClusterReassignment(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	clusterID := registerTestCluster(t)
	created, _, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(openapi.GatewayCreateRequest{
		Name:      "patch-cluster",
		ClusterId: clusterID,
	}).Execute()
	Expect(err).NotTo(HaveOccurred())

	_, resp, err := client.DefaultAPI.UpdateGateway(ctx, *created.Id).GatewayPatchRequest(openapi.GatewayPatchRequest{
		ClusterId: openapi.PtrString(createManualCluster(t)),
	}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	_, resp, err = client.DefaultAPI.UpdateGateway(ctx, *created.Id).GatewayPatchRequest(openapi.GatewayPatchRequest{
		ClusterId: openapi.PtrString(""),
	}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	other := registerTestCluster(t)
	patched, resp, err := client.DefaultAPI.UpdateGateway(ctx, *created.Id).GatewayPatchRequest(openapi.GatewayPatchRequest{
		ClusterId: openapi.PtrString(other),
	}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(patched.ClusterId).To(Equal(other))

	// A legacy gateway (service-created with an unregistered cluster_id) can still
	// have other fields edited when the PATCH re-sends its stored cluster_id.
	legacy, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())
	_, resp, err = client.DefaultAPI.UpdateGateway(ctx, legacy.ID).GatewayPatchRequest(openapi.GatewayPatchRequest{
		ClusterId: openapi.PtrString(legacy.ClusterId),
		TlsMode:   openapi.PtrString("updated"),
	}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
}

func expectNoGatewayNamed(ctx context.Context, client *openapi.APIClient, name string) {
	list, _, err := client.DefaultAPI.ListGateways(ctx).Search(fmt.Sprintf("name = '%s'", name)).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(list.Items).To(BeEmpty(), "no gateway named %s may be created", name)
}

func TestGatewayPostWithoutRouteRemainsUnrouted(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayInput := openapi.GatewayCreateRequest{
		Name:      "route-default-test",
		ClusterId: registerTestCluster(t),
		ReleaseId: "",
	}

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gatewayOutput.GetRoute()).To(BeEmpty())
}

func TestGatewayPostPreservesExplicitRoute(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	customRoute := `{"enabled":true,"host":"custom.example.com"}`
	gatewayInput := openapi.GatewayCreateRequest{
		Name:      "route-explicit-test",
		ClusterId: registerTestCluster(t),
		ReleaseId: "",
		Route:     openapi.PtrString(customRoute),
	}

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gatewayOutput.GetRoute()).To(Equal(customRoute))
}

func TestGatewayPatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayModel, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	gatewayOutput, resp, err := client.DefaultAPI.UpdateGateway(ctx, gatewayModel.ID).GatewayPatchRequest(openapi.GatewayPatchRequest{}).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*gatewayOutput.Id).To(Equal(gatewayModel.ID))
	Expect(*gatewayOutput.CreatedAt).To(BeTemporally("~", gatewayModel.CreatedAt))
	Expect(*gatewayOutput.Kind).To(Equal("Gateway"))
	Expect(*gatewayOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateways/%s", *gatewayOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Patch(h.RestURL("/gateways/foo"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestGatewayDelete(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayModel, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	resp, err := client.DefaultAPI.DeleteGateway(ctx, gatewayModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	_, resp, err = client.DefaultAPI.GetGateway(ctx, gatewayModel.ID).Execute()
	Expect(err).To(HaveOccurred(), "Expected deleted gateway to return 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

func TestGatewayPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, err := newGatewayList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ListGateways(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gateway list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.GetSize()).To(Equal(int32(20)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.ListGateways(ctx).Page(2).Size(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gateway list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.GetSize()).To(Equal(int32(5)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(2)))
}

func TestGatewayPagingSortedByCreator(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gateways, err := newGatewayList("creator-sort", 2)
	Expect(err).NotTo(HaveOccurred())

	envServices := &environments.Environment().Services
	userService := users.Service(envServices)
	bindingService := roleBindings.Service(envServices)
	creatorNames := []string{"zulu-creator", "alpha-creator"}
	for i, creatorName := range creatorNames {
		userID, createErr := userService.UpsertByUsername(context.Background(), creatorName, nil, nil)
		Expect(createErr).NotTo(HaveOccurred())
		Expect(bindingService.CreateGatewayOwnerBinding(context.Background(), userID, gateways[i].ID)).To(Succeed())
	}

	search := fmt.Sprintf("id in ('%s', '%s')", gateways[0].ID, gateways[1].ID)
	list, _, err := client.DefaultAPI.ListGateways(ctx).
		Search(search).
		OrderBy("created_by asc").
		Execute()
	Expect(err).NotTo(HaveOccurred(), "Error sorting gateway list by creator: %v", err)
	Expect(list.Items).To(HaveLen(2))
	Expect(list.Items[0].GetCreatedBy()).To(Equal("alpha-creator"))
	Expect(list.Items[1].GetCreatedBy()).To(Equal("zulu-creator"))
}

func TestGatewayPostWithCredentialDriver(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	credDriver := `{"type":"kubernetes-secrets","kubernetes_secrets":{"namespace":"creds-ns"}}`
	gatewayInput := openapi.GatewayCreateRequest{
		Name:             "test-cred-driver",
		ClusterId:        registerTestCluster(t),
		ReleaseId:        "test-release_id",
		CredentialDriver: openapi.PtrString(credDriver),
	}

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting gateway with credential_driver: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gatewayOutput.CredentialDriver).NotTo(BeNil())
	Expect(*gatewayOutput.CredentialDriver).To(Equal(credDriver))

	retrieved, resp, err := client.DefaultAPI.GetGateway(ctx, *gatewayOutput.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(retrieved.CredentialDriver).NotTo(BeNil())
	// Read-back goes through the jsonb column, which canonicalizes whitespace,
	// so compare JSON semantically rather than byte-for-byte.
	Expect(*retrieved.CredentialDriver).To(MatchJSON(credDriver))
}

func TestGatewayPatchCredentialDriver(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayModel, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	credDriver := `{"type":"vault","vault":{"address":"https://vault.example.com","role":"gw-role"}}`
	patchReq := openapi.GatewayPatchRequest{
		CredentialDriver: openapi.PtrString(credDriver),
	}

	gatewayOutput, resp, err := client.DefaultAPI.UpdateGateway(ctx, gatewayModel.ID).GatewayPatchRequest(patchReq).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error patching credential_driver: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(gatewayOutput.CredentialDriver).NotTo(BeNil())
	Expect(*gatewayOutput.CredentialDriver).To(Equal(credDriver))

	retrieved, resp, err := client.DefaultAPI.GetGateway(ctx, gatewayModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	// Read-back goes through the jsonb column, which canonicalizes whitespace,
	// so compare JSON semantically rather than byte-for-byte.
	Expect(*retrieved.CredentialDriver).To(MatchJSON(credDriver))
}

func TestGatewayListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gateways, err := newGatewayList("bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id in ('%s')", gateways[0].ID)
	list, _, err := client.DefaultAPI.ListGateways(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gateway list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.GetTotal()).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(gateways[0].ID))
}
