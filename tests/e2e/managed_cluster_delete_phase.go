package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// assertManagedClusterDeleteAndReRegister extends the managedcluster-lifecycle
// phase (P2.4) with E2E-COV-06: deleting a registered ManagedCluster's record
// detaches the gateways placed on it until the control plane registers again, and
// the control plane's next /registration restores the same cluster_id so the
// gateway re-attaches. Runs only under E2E_MULTICLUSTER=1, against a real second
// cluster and control plane; a single-cluster run cannot delete the only
// registered cluster without breaking itself, so it SHALL skip.
func (s *E2ESuite) assertManagedClusterDeleteAndReRegister(t *testing.T) {
	if envOrDefault("E2E_MULTICLUSTER", "0") != "1" {
		t.Skip("ManagedCluster delete-and-restore coverage requires E2E_MULTICLUSTER=1 and a second real cluster; a single-cluster run cannot delete its only registered cluster")
	}
	ctx := t.Context()
	secondName := os.Getenv("E2E_SEED_CLUSTER_NAME_2")
	s.Require().NotEmpty(secondName, "E2E_MULTICLUSTER=1 requires E2E_SEED_CLUSTER_NAME_2")
	secondClusterID := s.findManagedClusterIDByName(t, secondName)

	// Capture the cluster's pre-delete heartbeat so a post-restore heartbeat can be
	// required to be strictly newer - the control-plane-written reconcile signal
	// that proves a live control plane re-attached (not a stale leftover record).
	before, err := s.admin.ManagedClusters().Get(ctx, secondClusterID)
	s.Require().NoError(err, "get second managed cluster before delete")
	s.Require().NotNil(before.LastSeenAt, "registered second cluster has a last_seen_at")
	lastSeenBeforeDelete := *before.LastSeenAt

	gw := s.placeGatewayOnCluster(t, "e2e-orphan-"+s.runID, secondClusterID, 8)
	if gw == nil {
		t.Skipf("could not place a throwaway gateway on cluster %q after several attempts; random placement never selected it", secondName)
	}
	s.trackGateway(gw.ID)
	// Fully reconcile the gateway to Running before deleting its cluster, so a
	// re-attachment observed afterward is a real restore, not in-flight provisioning.
	s.waitGatewayRunning(t, gw.ID)

	// DELETE /managed_clusters/{id} requires platform:admin (any record other than
	// the caller's own self-registered cluster); grant it rather than assuming the
	// base admin identity already carries it.
	s.Require().NoError(s.driver.AssignRealmRole(ctx, s.platformAdminCreds().Username, "platform:admin"), "grant platform:admin")
	platformAPI := s.apiClientForCreds(t, s.platformAdminCreds())

	s.runner.Show("DELETE /managed_clusters/%s  # %s, then expect gateway %s to detach", secondClusterID, secondName, gw.ID)
	status, body, err := platformAPI.RawJSON(ctx, http.MethodDelete, "/managed_clusters/"+secondClusterID, nil)
	s.Require().NoError(err, "delete second managed cluster record")
	s.Require().Truef(status == http.StatusNoContent || status == http.StatusOK || status == http.StatusAccepted,
		"delete managed cluster status %d (body: %s)", status, string(body))
	s.Require().NoError(harness.Poll(ctx, 2*time.Second, 30*time.Second, func(ctx context.Context) (bool, error) {
		st, _, err := s.admin.RawJSON(ctx, http.MethodGet, "/managed_clusters/"+secondClusterID, nil)
		return err == nil && st == http.StatusNotFound, nil
	}), "deleted managed cluster record reaches 404")

	// E2E-COV-06.01: the gateway is detached but keeps its (now-dangling) cluster_id
	// rather than being silently reassigned. PLAT-DATA-23 makes every gateway field
	// except name control-plane-only, so there is no user-writable field to drive a
	// deliberate pending change while detached; the control-plane-observable
	// re-attachment signal below (the cluster re-registering with a fresh heartbeat)
	// is used instead.
	detached, err := s.admin.Gateways().Get(ctx, gw.ID)
	s.Require().NoError(err, "get gateway after cluster delete")
	s.Assert().Equal(secondClusterID, detached.ClusterID, "detached gateway keeps its (now-dangling) cluster_id rather than being silently reassigned")

	// E2E-COV-06.02: the second control plane's next /registration restores the same
	// cluster_id and resumes heartbeating. Require the restored record to carry a
	// last_seen_at strictly newer than the pre-delete heartbeat, proving a live
	// control plane re-attached and is reconciling again.
	restoreTimeout := durationSecondsEnv("E2E_MANAGED_CLUSTER_RESTORE_TIMEOUT", 5*time.Minute)
	s.Require().NoError(harness.Poll(ctx, 5*time.Second, restoreTimeout, func(ctx context.Context) (bool, error) {
		clusters, err := s.admin.ManagedClusters().List(ctx, nil)
		if err != nil {
			return false, nil
		}
		for _, cluster := range clusters.Items {
			if cluster.Name == secondName && cluster.ID == secondClusterID && cluster.OidcSubject != "" &&
				cluster.LastSeenAt != nil && cluster.LastSeenAt.After(lastSeenBeforeDelete) {
				return true, nil
			}
		}
		return false, nil
	}), "second control plane re-registers the same cluster_id with a fresh heartbeat")

	// The gateway remains bound to the restored cluster_id: re-attached rather than
	// orphaned or reassigned.
	reattached, err := s.admin.Gateways().Get(ctx, gw.ID)
	s.Require().NoError(err, "get gateway after re-registration")
	s.Assert().Equal(secondClusterID, reattached.ClusterID, "gateway re-attaches to the restored cluster_id")

	s.deleteGatewayAndWaitGC(t, gw.ID, reattached.Namespace)
}

// findManagedClusterIDByName resolves name's id through /managed_clusters.
func (s *E2ESuite) findManagedClusterIDByName(t *testing.T, name string) string {
	clusters, err := s.admin.ManagedClusters().List(t.Context(), nil)
	s.Require().NoErrorf(err, "list managed clusters to resolve %s", name)
	for _, cluster := range clusters.Items {
		if cluster.Name == name {
			return cluster.ID
		}
	}
	s.Require().Failf("managed cluster not found", "cluster %q not present in /managed_clusters", name)
	return ""
}

// placeGatewayOnCluster creates gateways using the standard placement selector
// until one resolves onto targetClusterID (WEB-PLC-07's uniform-random placement
// makes this a small number of attempts in a two-cluster fleet), deleting any
// that land elsewhere. Returns nil if targetClusterID was never selected within
// maxAttempts.
func (s *E2ESuite) placeGatewayOnCluster(t *testing.T, namePrefix, targetClusterID string, maxAttempts int) *sdktypes.Gateway {
	for attempt := range maxAttempts {
		gw := s.createGateway(t, fmt.Sprintf("%s-%d", namePrefix, attempt))
		if gw.ClusterID == targetClusterID {
			return gw
		}
		_ = s.admin.Gateways().Delete(context.Background(), gw.ID)
	}
	return nil
}
