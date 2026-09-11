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

	_, _, err := client.DefaultAPI.GetManagedDatabase(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	_, resp, err := client.DefaultAPI.GetManagedDatabase(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	managedDatabaseModel, err := newManagedDatabase(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	managedDatabaseOutput, resp, err := client.DefaultAPI.GetManagedDatabase(ctx, managedDatabaseModel.ID).Execute()
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
		Provider:         "deployment",
		Region:           openapi.PtrString("test-region"),
		Engine:           openapi.PtrString("test-engine"),
		EngineVersion:    openapi.PtrString("test-engine_version"),
		InstanceClass:    openapi.PtrString("test-instance_class"),
		ConnectionSecret: openapi.PtrString("test-connection_secret"),
		Status:           openapi.PtrString("test-status"),
	}

	managedDatabaseOutput, resp, err := client.DefaultAPI.CreateManagedDatabase(ctx).ManagedDatabase(managedDatabaseInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*managedDatabaseOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*managedDatabaseOutput.Kind).To(Equal("ManagedDatabase"))
	Expect(*managedDatabaseOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/managed_databases/%s", *managedDatabaseOutput.Id)))

	_, resp, err = client.DefaultAPI.CreateManagedDatabase(ctx).ManagedDatabase(openapi.ManagedDatabase{Name: "invalid-provider", Provider: "unsupported"}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

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

	managedDatabaseOutput, resp, err := client.DefaultAPI.UpdateManagedDatabase(ctx, managedDatabaseModel.ID).ManagedDatabasePatchRequest(openapi.ManagedDatabasePatchRequest{}).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*managedDatabaseOutput.Id).To(Equal(managedDatabaseModel.ID))
	Expect(*managedDatabaseOutput.CreatedAt).To(BeTemporally("~", managedDatabaseModel.CreatedAt))
	Expect(*managedDatabaseOutput.Kind).To(Equal("ManagedDatabase"))
	Expect(*managedDatabaseOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/managed_databases/%s", *managedDatabaseOutput.Id)))

	_, resp, err = client.DefaultAPI.UpdateManagedDatabase(ctx, managedDatabaseModel.ID).
		ManagedDatabasePatchRequest(openapi.ManagedDatabasePatchRequest{Provider: openapi.PtrString("unsupported")}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	persisted, resp, err := client.DefaultAPI.GetManagedDatabase(ctx, managedDatabaseModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(persisted.Provider).To(Equal("deployment"))

	_, resp, err = client.DefaultAPI.UpdateManagedDatabase(ctx, managedDatabaseModel.ID).
		ManagedDatabasePatchRequest(openapi.ManagedDatabasePatchRequest{Provider: openapi.PtrString("cnpg")}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	persisted, resp, err = client.DefaultAPI.GetManagedDatabase(ctx, managedDatabaseModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(persisted.Provider).To(Equal("deployment"))

	managedDatabaseOutput, resp, err = client.DefaultAPI.UpdateManagedDatabase(ctx, managedDatabaseModel.ID).
		ManagedDatabasePatchRequest(openapi.ManagedDatabasePatchRequest{Provider: openapi.PtrString("deployment")}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(managedDatabaseOutput.Provider).To(Equal("deployment"))

	managedDatabaseOutput, resp, err = client.DefaultAPI.UpdateManagedDatabase(ctx, managedDatabaseModel.ID).
		ManagedDatabasePatchRequest(openapi.ManagedDatabasePatchRequest{Status: openapi.PtrString("Ready")}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(managedDatabaseOutput.Provider).To(Equal("deployment"))
	Expect(managedDatabaseOutput.GetStatus()).To(Equal("Ready"))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Patch(h.RestURL("/managed_databases/foo"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestManagedDatabaseDelete(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedDatabaseModel, err := newManagedDatabase(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	resp, err := client.DefaultAPI.DeleteManagedDatabase(ctx, managedDatabaseModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	_, resp, err = client.DefaultAPI.GetManagedDatabase(ctx, managedDatabaseModel.ID).Execute()
	Expect(err).To(HaveOccurred(), "Expected deleted managed database to return 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

func TestManagedDatabasePaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, err := newManagedDatabaseList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ListManagedDatabases(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedDatabase list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.GetSize()).To(Equal(int32(20)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.ListManagedDatabases(ctx).Page(2).Size(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedDatabase list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.GetSize()).To(Equal(int32(5)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(2)))
}

func TestManagedDatabaseListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	managedDatabases, err := newManagedDatabaseList("bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id in ('%s')", managedDatabases[0].ID)
	list, _, err := client.DefaultAPI.ListManagedDatabases(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting managedDatabase list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.GetTotal()).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(managedDatabases[0].ID))
}
