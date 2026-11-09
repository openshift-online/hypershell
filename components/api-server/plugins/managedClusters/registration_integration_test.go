package managedClusters_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

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

// healthVerdict builds a well-formed self-check verdict for registration tests.
func healthVerdict(overall string) openapi.ManagedClusterHealth {
	observed := time.Now().UTC()
	transition := observed.Add(-time.Hour)
	return openapi.ManagedClusterHealth{
		Overall:    overall,
		ObservedAt: &observed,
		DurationMs: openapi.PtrInt64(842),
		Conditions: []openapi.ManagedClusterHealthCondition{
			{Name: "oidc-issuer-reachable", Status: "fail", LastTransitionAt: &transition, Message: openapi.PtrString("issuer returned 503")},
			{Name: "images-pullable", Status: "pass", LastTransitionAt: &transition},
		},
	}
}

// The heartbeat carries the self-check verdict: the hub stores it on the record,
// surfaces it on GET, and updates last_seen_at in the same call.
// Verifies: PLAT-CLUS-16.01, PLAT-DATA-27.02
func TestManagedClusterRegistrationStoresAndSurfacesHealthVerdict(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	ctx := controlPlaneContext(h, uniqueName("cp"))
	name := uniqueName("hyp0-mc")

	// A cluster that has never reported a verdict reads back health as null.
	created, resp, err := client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: name, Capabilities: []string{"network/public", "provider/aws"}}).
		Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	before, _, err := client.DefaultAPI.GetManagedCluster(ctx, created.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(before.Health).To(BeNil(), "health is null until the first verdict is reported")
	Expect(before.LastSeenAt).NotTo(BeNil())

	// A heartbeat carrying the verdict stores it.
	verdict := healthVerdict("failing")
	_, resp, err = client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{
			Name:         name,
			Capabilities: []string{"network/public", "provider/aws"},
			Health:       &verdict,
		}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	after, _, err := client.DefaultAPI.GetManagedCluster(ctx, created.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(after.Health).NotTo(BeNil(), "GET must surface the stored verdict")
	Expect(after.Health.Overall).To(Equal("failing"))
	Expect(after.Health.Conditions).To(HaveLen(2))
	Expect(after.Health.GetDurationMs()).To(Equal(int64(842)))
	Expect(after.LastSeenAt).NotTo(BeNil())

	// GET /managed_clusters surfaces it too.
	list, _, err := client.DefaultAPI.ListManagedClusters(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	var found bool
	for _, item := range list.Items {
		if item.GetId() == created.ClusterId {
			found = true
			Expect(item.Health).NotTo(BeNil())
			Expect(item.Health.Overall).To(Equal("failing"))
		}
	}
	Expect(found).To(BeTrue(), "the list must include the registered cluster")
}

// A health-less heartbeat is a no-op for the stored verdict: it is preserved, and
// last_seen_at is still updated, so a mixed-version rolling upgrade never drops a
// cluster's verdict.
// Verifies: PLAT-CLUS-16.02
func TestManagedClusterHealthLessHeartbeatPreservesVerdict(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	ctx := controlPlaneContext(h, uniqueName("cp"))
	name := uniqueName("hyp0-mc")

	verdict := healthVerdict("degraded")
	created, resp, err := client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{
			Name:         name,
			Capabilities: []string{"network/public", "provider/aws"},
			Health:       &verdict,
		}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	// A later heartbeat with no health object preserves the stored verdict.
	_, resp, err = client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: name, Capabilities: []string{"network/public", "provider/aws"}}).
		Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	after, _, err := client.DefaultAPI.GetManagedCluster(ctx, created.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(after.Health).NotTo(BeNil(), "the stored verdict must be preserved")
	Expect(after.Health.Overall).To(Equal("degraded"))
	Expect(after.LastSeenAt).NotTo(BeNil())
}

// A malformed verdict is rejected with 400 and changes neither the stored verdict
// nor last_seen_at.
// Verifies: PLAT-CLUS-16.03
func TestManagedClusterMalformedHealthRejectedAtomically(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	ctx := controlPlaneContext(h, uniqueName("cp"))
	name := uniqueName("hyp0-mc")

	verdict := healthVerdict("healthy")
	created, resp, err := client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{
			Name:         name,
			Capabilities: []string{"network/public", "provider/aws"},
			Health:       &verdict,
		}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	before, _, err := client.DefaultAPI.GetManagedCluster(ctx, created.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(before.Health).NotTo(BeNil())

	// A verdict with an out-of-enumeration overall is malformed and rejected.
	malformed := healthVerdict("failing")
	malformed.Overall = "bogus"
	_, resp, err = client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{
			Name:         name,
			Capabilities: []string{"network/public", "provider/aws"},
			Health:       &malformed,
		}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	after, _, err := client.DefaultAPI.GetManagedCluster(ctx, created.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(after.Health).NotTo(BeNil(), "the stored verdict must be unchanged")
	Expect(after.Health.Overall).To(Equal("healthy"))
}

// health is control-plane-reported and read-only to a generic PATCH: a PATCH never
// assigns or modifies the stored verdict.
// Verifies: PLAT-DATA-27.01
func TestManagedClusterHealthReadOnlyToPatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	ctx := controlPlaneContext(h, uniqueName("cp"))
	name := uniqueName("hyp0-mc")

	verdict := healthVerdict("failing")
	created, resp, err := client.DefaultAPI.RegisterManagedCluster(ctx).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{
			Name:         name,
			Capabilities: []string{"network/public", "provider/aws"},
			Health:       &verdict,
		}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	// A PATCH has no health field in its contract and must not disturb the verdict.
	userCtx := h.NewAuthenticatedContext(h.NewRandAccount())
	_, _, err = client.DefaultAPI.UpdateManagedCluster(userCtx, created.ClusterId).
		ManagedClusterPatchRequest(openapi.ManagedClusterPatchRequest{Status: openapi.PtrString("Ready")}).
		Execute()
	Expect(err).NotTo(HaveOccurred())

	after, _, err := client.DefaultAPI.GetManagedCluster(ctx, created.ClusterId).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(after.Health).NotTo(BeNil(), "PATCH must not clear the stored verdict")
	Expect(after.Health.Overall).To(Equal("failing"))
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
