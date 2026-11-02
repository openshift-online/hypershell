package managedClusters_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
)

// controlPlaneContext authenticates as a control plane: a unique JWT subject
// holding the managed-cluster-registrar realm role.
func controlPlaneContext(h *test.Helper, subject string) context.Context {
	account := h.NewAccount("service-account-"+subject, "control plane", "")
	token := h.CreateJWTStringWithClaims(account, test.ControlPlaneClaims(subject))
	return context.WithValue(context.Background(), openapi.ContextAccessToken, token)
}

func register(client *openapi.APIClient, ctx context.Context, name string) (*openapi.ManagedClusterRegistrationResponse, *http.Response, error) {
	return client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: name, Capabilities: []string{"network/public", "provider/aws"}}).
		Execute()
}

func uniqueName(prefix string) string {
	return prefix + "-" + strings.ToLower(api.NewID())
}

func clusterCount(client *openapi.APIClient, ctx context.Context) int32 {
	list, _, err := client.DefaultAPI.ListManagedClusters(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	return list.GetTotal()
}

// Idempotent Registration and Name change rejected.
// Verifies: PLAT-CLUS-01.01, PLAT-CLUS-01.02, PLAT-CLUS-01.04
func TestManagedClusterRegistrationIdempotent(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	subject := uniqueName("cp")
	ctx := controlPlaneContext(h, subject)
	name := uniqueName("hyp0-mc")

	first, resp, err := register(client, ctx, name)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	again, resp, err := register(client, ctx, name)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(again.ClusterId).To(Equal(first.ClusterId))

	_, resp, err = register(client, ctx, uniqueName("hyp0-mc"))
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusConflict))
}

// A registration without capabilities is rejected and changes nothing: the
// stored set and last_seen_at keep their values from the last good heartbeat.
// Verifies: PLAT-CLUS-07.03
func TestManagedClusterRegistrationRejectsCapabilityLessHeartbeat(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	ctx := controlPlaneContext(h, uniqueName("cp"))
	name := uniqueName("hyp0-mc")

	// A first registration without capabilities is rejected and creates nothing.
	_, resp, err := client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: name}).
		Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	created, resp, err := register(client, ctx, name)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	before, _, err := client.DefaultAPI.GetManagedCluster(ctx, created.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())

	for _, capabilities := range [][]string{nil, {}} {
		_, resp, err = client.DefaultAPI.RegisterManagedCluster(ctx).
			ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: name, Capabilities: capabilities}).
			Execute()
		Expect(err).To(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	}

	after, _, err := client.DefaultAPI.GetManagedCluster(ctx, created.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(after.Capabilities).To(Equal([]string{"network/public", "provider/aws"}))
	Expect(after.LastSeenAt).NotTo(BeNil())
	Expect(after.LastSeenAt.Equal(*before.LastSeenAt)).To(BeTrue())
}

// Verifies: PLAT-CLUS-01.03
func TestManagedClusterRegistrationRestoreRefreshesCapabilities(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	ctx := controlPlaneContext(h, uniqueName("cp"))
	name := uniqueName("hyp0-mc")

	first, resp, err := register(client, ctx, name)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	resp, err = client.DefaultAPI.DeleteManagedCluster(ctx, first.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	restored, resp, err := client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{
			Name:         name,
			Capabilities: []string{"network/public", "provider/ibm"},
		}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(restored.ClusterId).To(Equal(first.ClusterId))

	cluster, _, err := client.DefaultAPI.GetManagedCluster(ctx, restored.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(cluster.Capabilities).To(Equal([]string{"network/public", "provider/ibm"}))
	Expect(cluster.LastSeenAt).NotTo(BeNil())
}

// A registration without capabilities cannot restore a soft-deleted record:
// it is rejected before the restore path runs.
// Verifies: PLAT-CLUS-07.03
func TestManagedClusterRegistrationRestoreRejectsCapabilityLessHeartbeat(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	ctx := controlPlaneContext(h, uniqueName("cp"))
	name := uniqueName("hyp0-mc")

	first, resp, err := register(client, ctx, name)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	resp, err = client.DefaultAPI.DeleteManagedCluster(ctx, first.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	_, resp, err = client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: name}).
		Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	_, resp, err = client.DefaultAPI.GetManagedCluster(ctx, first.ClusterId).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

// Name Collision Is a Conflict: a name held by a manually created record (empty
// oidc_subject) is a 409 naming the record, nothing is created or adopted, and
// deleting the record lets the control plane register (201, new id).
// Verifies: PLAT-CLUS-02.01, PLAT-CLUS-02.03
func TestManagedClusterRegistrationNameHeldByManualRecord(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	userCtx := h.NewAuthenticatedContext(h.NewRandAccount())
	name := uniqueName("local-openshift")

	manual, _, err := client.DefaultAPI.CreateManagedCluster(userCtx).ManagedCluster(openapi.ManagedCluster{
		Name: name, KubeconfigSecret: "openshift-kubeconfig",
	}).Execute()
	Expect(err).NotTo(HaveOccurred())
	before := clusterCount(client, userCtx)

	cpCtx := controlPlaneContext(h, uniqueName("cp"))
	_, resp, err := register(client, cpCtx, name)
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusConflict))
	reason := test.APIErrorReason(err)
	Expect(reason).To(ContainSubstring(*manual.Id), "the conflict must name the existing record")
	Expect(reason).To(ContainSubstring("operator must delete"))

	Expect(clusterCount(client, userCtx)).To(Equal(before), "no record may be created")
	unchanged, _, err := client.DefaultAPI.GetManagedCluster(userCtx, *manual.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(unchanged.GetOidcSubject()).To(BeEmpty(), "the manual record must not be adopted")
	Expect(unchanged.LastSeenAt).To(BeNil())

	// Operator resolves the collision.
	_, err = client.DefaultAPI.DeleteManagedCluster(userCtx, *manual.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	registered, resp, err := register(client, cpCtx, name)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(registered.ClusterId).NotTo(Equal(*manual.Id))
}

// Name Collision Is a Conflict: a name registered by another OIDC subject is a
// 409 and nothing is created or modified.
// Verifies: PLAT-CLUS-02.02
func TestManagedClusterRegistrationNameHeldByAnotherControlPlane(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	name := uniqueName("hyp0-mc1")

	_, resp, err := register(client, controlPlaneContext(h, uniqueName("cp-a")), name)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	userCtx := h.NewAuthenticatedContext(h.NewRandAccount())
	before := clusterCount(client, userCtx)

	_, resp, err = register(client, controlPlaneContext(h, uniqueName("cp-b")), name)
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusConflict))
	Expect(clusterCount(client, userCtx)).To(Equal(before))
}

// name is unique across all records: a manual create reusing a registered name
// is a 409 too.
func TestManagedClusterNameIsUnique(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	name := uniqueName("unique")
	_, _, err := register(client, controlPlaneContext(h, uniqueName("cp")), name)
	Expect(err).NotTo(HaveOccurred())

	userCtx := h.NewAuthenticatedContext(h.NewRandAccount())
	_, resp, err := client.DefaultAPI.CreateManagedCluster(userCtx).ManagedCluster(openapi.ManagedCluster{
		Name: name, KubeconfigSecret: "kubeconfig",
	}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusConflict))
}
