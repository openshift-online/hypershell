package managedClusters_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestManagedClusterGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, _, err := client.DefaultAPI.GetManagedCluster(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	_, resp, err := client.DefaultAPI.GetManagedCluster(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	managedClusterModel, err := newManagedCluster(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	managedClusterOutput, resp, err := client.DefaultAPI.GetManagedCluster(ctx, managedClusterModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	Expect(*managedClusterOutput.Id).To(Equal(managedClusterModel.ID), "found object does not match test object")
	Expect(*managedClusterOutput.Kind).To(Equal("ManagedCluster"))
	Expect(*managedClusterOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/managed_clusters/%s", managedClusterModel.ID)))
	Expect(*managedClusterOutput.CreatedAt).To(BeTemporally("~", managedClusterModel.CreatedAt))
	Expect(*managedClusterOutput.UpdatedAt).To(BeTemporally("~", managedClusterModel.UpdatedAt))
}

func TestManagedClusterPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedClusterInput := openapi.ManagedCluster{
		Name:             "test-name",
		Provider:         "test-provider",
		Region:           openapi.PtrString("test-region"),
		KubeconfigSecret: "test-kubeconfig_secret",
		Status:           openapi.PtrString("test-status"),
		ApiServerUrl:     openapi.PtrString("test-api_server_url"),
	}

	managedClusterOutput, resp, err := client.DefaultAPI.CreateManagedCluster(ctx).ManagedCluster(managedClusterInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*managedClusterOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*managedClusterOutput.Kind).To(Equal("ManagedCluster"))
	Expect(*managedClusterOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/managed_clusters/%s", *managedClusterOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/managed_clusters"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestManagedClusterPatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedClusterModel, err := newManagedCluster(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	managedClusterOutput, resp, err := client.DefaultAPI.UpdateManagedCluster(ctx, managedClusterModel.ID).ManagedClusterPatchRequest(openapi.ManagedClusterPatchRequest{}).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*managedClusterOutput.Id).To(Equal(managedClusterModel.ID))
	Expect(*managedClusterOutput.CreatedAt).To(BeTemporally("~", managedClusterModel.CreatedAt))
	Expect(*managedClusterOutput.Kind).To(Equal("ManagedCluster"))
	Expect(*managedClusterOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/managed_clusters/%s", *managedClusterOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Patch(h.RestURL("/managed_clusters/foo"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestManagedClusterDelete(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedClusterModel, err := newManagedCluster(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	resp, err := client.DefaultAPI.DeleteManagedCluster(ctx, managedClusterModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	_, resp, err = client.DefaultAPI.GetManagedCluster(ctx, managedClusterModel.ID).Execute()
	Expect(err).To(HaveOccurred(), "Expected deleted managed cluster to return 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

func TestManagedClusterPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, err := newManagedClusterList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ListManagedClusters(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedCluster list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.GetSize()).To(Equal(int32(20)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.ListManagedClusters(ctx).Page(2).Size(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedCluster list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.GetSize()).To(Equal(int32(5)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(2)))
}

func TestManagedClusterListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedClusters, err := newManagedClusterList("bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id in ('%s')", managedClusters[0].ID)
	list, _, err := client.DefaultAPI.ListManagedClusters(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedCluster list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.GetTotal()).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(managedClusters[0].ID))
}

func TestManagedClusterListSearchEscapedLiteral(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	clusterName := "team's_100%"
	clusterInput := openapi.ManagedCluster{
		Name:             clusterName,
		Provider:         "test-provider",
		KubeconfigSecret: "test-kubeconfig_secret",
	}

	_, _, err := client.DefaultAPI.CreateManagedCluster(ctx).ManagedCluster(clusterInput).Execute()
	Expect(err).NotTo(HaveOccurred())

	search := "name ilike '%team''s\\_100\\%%'"
	list, _, err := client.DefaultAPI.ListManagedClusters(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error searching for a literal managed cluster name: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.GetTotal()).To(Equal(int32(1)))
	Expect(list.Items[0].GetName()).To(Equal(clusterName))
}
