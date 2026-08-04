package managedDatabases_test

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

func TestManagedDatabaseGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, _, err := client.DefaultAPI.ApiHypershellV1ManagedDatabasesIdGet(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	_, resp, err := client.DefaultAPI.ApiHypershellV1ManagedDatabasesIdGet(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	managedDatabaseModel, err := newManagedDatabase(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	managedDatabaseOutput, resp, err := client.DefaultAPI.ApiHypershellV1ManagedDatabasesIdGet(ctx, managedDatabaseModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	Expect(*managedDatabaseOutput.Id).To(Equal(managedDatabaseModel.ID), "found object does not match test object")
	Expect(*managedDatabaseOutput.Kind).To(Equal("ManagedDatabase"))
	Expect(*managedDatabaseOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/managed_databases/%s", managedDatabaseModel.ID)))
	Expect(*managedDatabaseOutput.CreatedAt).To(BeTemporally("~", managedDatabaseModel.CreatedAt))
	Expect(*managedDatabaseOutput.UpdatedAt).To(BeTemporally("~", managedDatabaseModel.UpdatedAt))
}

func TestManagedDatabasePost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedDatabaseInput := openapi.ManagedDatabase{
		Name:             "test-name",
		FleetId:          "test-fleet_id",
		Provider:         "test-provider",
		Region:           openapi.PtrString("test-region"),
		Engine:           openapi.PtrString("test-engine"),
		EngineVersion:    openapi.PtrString("test-engine_version"),
		InstanceClass:    openapi.PtrString("test-instance_class"),
		ConnectionSecret: openapi.PtrString("test-connection_secret"),
		Status:           openapi.PtrString("test-status"),
	}

	managedDatabaseOutput, resp, err := client.DefaultAPI.ApiHypershellV1ManagedDatabasesPost(ctx).ManagedDatabase(managedDatabaseInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*managedDatabaseOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*managedDatabaseOutput.Kind).To(Equal("ManagedDatabase"))
	Expect(*managedDatabaseOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/managed_databases/%s", *managedDatabaseOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/managed_databases"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestManagedDatabasePatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedDatabaseModel, err := newManagedDatabase(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	managedDatabaseOutput, resp, err := client.DefaultAPI.ApiHypershellV1ManagedDatabasesIdPatch(ctx, managedDatabaseModel.ID).ManagedDatabasePatchRequest(openapi.ManagedDatabasePatchRequest{}).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*managedDatabaseOutput.Id).To(Equal(managedDatabaseModel.ID))
	Expect(*managedDatabaseOutput.CreatedAt).To(BeTemporally("~", managedDatabaseModel.CreatedAt))
	Expect(*managedDatabaseOutput.Kind).To(Equal("ManagedDatabase"))
	Expect(*managedDatabaseOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/managed_databases/%s", *managedDatabaseOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Patch(h.RestURL("/managed_databases/foo"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestManagedDatabasePaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, err := newManagedDatabaseList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ApiHypershellV1ManagedDatabasesGet(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedDatabase list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.Size).To(Equal(int32(20)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.ApiHypershellV1ManagedDatabasesGet(ctx).Page(2).Size(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedDatabase list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.Size).To(Equal(int32(5)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(2)))
}

func TestManagedDatabaseListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedDatabases, err := newManagedDatabaseList("bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id in ('%s')", managedDatabases[0].ID)
	list, _, err := client.DefaultAPI.ApiHypershellV1ManagedDatabasesGet(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedDatabase list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.Total).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(managedDatabases[0].ID))
}
