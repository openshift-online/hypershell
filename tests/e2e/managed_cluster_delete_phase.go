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
// detaches the gateways placed on it until the control plane registers again,
// and the control plane's next /registration restores the same cluster_id so
// the gateway re-attaches. Runs only under E2E_MULTICLUSTER=1, against a real
// second cluster and control plane; a single-cluster run cannot delete the only
// registered cluster without breaking itself, so it SHALL t.Skipf.
func (s *E2ESuite) assertManagedClusterDeleteAndReRegister(t *testing.T) {
	if envOrDefault("E2E_MULTICLUSTER", "0") != "1" {
		t.Skip("ManagedCluster delete-and-restore coverage requires E2E_MULTICLUSTER=1 and a second real cluster; a single-cluster run cannot delete its only registered cluster")
	}
	ctx := t.Context()
	secondName := os.Getenv("E2E_SEED_CLUSTER_NAME_2")
	s.Require().NotEmpty(secondName, "E2E_MULTICLUSTER=1 requires E2E_SEED_CLUSTER_NAME_2")
	secondClusterID := s.findManagedClusterIDByName(t, secondName)

	gw := s.placeGatewayOnCluster(t, "e2e-orphan-"+s.runID, secondClusterID, 8)
	if gw == nil {
		t.Skipf("could not place a throwaway gateway on cluster %q after several attempts; random placement never selected it", secondName)
	}
	s.trackGateway(gw.ID)
	// Fully reconcile the gateway to Running BEFORE deleting its cluster, so the
	// post-detachment marker captured below reflects the detached state rather
	// than in-flight initial provisioning.
	s.waitGatewayRunning(t, gw.ID)

	// DELETE /managed_clusters/{id} requires platform:admin (any record other
	// than the caller's own self-registered cluster); grant it rather than
	// assuming the base admin identity already carries it.
	s.Require().NoError(s.driver.AssignRealmRole(ctx, s.platformAdminCreds().Username, "platform:admin"), "grant platform:admin")
	platformAPI := s.apiClientForCreds(t, s.platformAdminCreds())

	s.runner.Show("DELETE /managed_clusters/%s  # %s, then expect gateway %s to detach", secondClusterID, secondName, gw.ID)
	status, body, err := platformAPI.RawJSON(ctx, http.MethodDelete, "/managed_clusters/"+secondClusterID, nil)
	s.Require().NoError(err, "delete second managed cluster record")
	s.Require().Truef(status == http.StatusNoContent || status == http.StatusOK || status == http.StatusAccepted,
		"delete managed cluster status %d (body: %s)", status, string(body))

	s.Require().NoError(harness.Poll(ctx, 2*time.Second, 30*time.Second, func(ctx context.Context) (bool, error) {
		status, _, err := s.admin.RawJSON(ctx, http.MethodGet, "/managed_clusters/"+secondClusterID, nil)
		return err == nil && status == http.StatusNotFound, nil
	}), "deleted managed cluster record reaches 404")

	// E2E-COV-06.01: the gateway is detached. Its cluster_id now dangles (no
	// registered ManagedCluster claims it), which is the only API-observable
	// signal that no control plane is currently positioned to reconcile it.
	detached, err := s.admin.Gateways().Get(ctx, gw.ID)
	s.Require().NoError(err, "get gateway after cluster delete")
	s.Assert().Equal(secondClusterID, detached.ClusterID, "detached gateway keeps its (now-dangling) cluster_id rather than being silently reassigned")
	// Observable marker captured AFTER detachment: re-attachment must drive a
	// reconcile that advances past this, which a gateway merely staying Running
	// (or the create-time generation) would not prove.
	detachedGeneration := detached.ObservedGeneration

	// E2E-COV-06.02: the second control plane's next /registration restores the
	// same cluster_id, and the gateway resumes reconciling.
	restoreTimeout := durationSecondsEnv("E2E_MANAGED_CLUSTER_RESTORE_TIMEOUT", 5*time.Minute)
	s.Require().NoError(harness.Poll(ctx, 5*time.Second, restoreTimeout, func(ctx context.Context) (bool, error) {
		clusters, err := s.admin.ManagedClusters().List(ctx, nil)
		if err != nil {
			return false, nil
		}
		for _, cluster := range clusters.Items {
			if cluster.Name == secondName && cluster.OidcSubject != "" {
				return cluster.ID == secondClusterID, nil
			}
		}
		return false, nil
	}), "second control plane re-registers and restores the same cluster_id")

	s.Require().NoError(harness.Poll(ctx, 5*time.Second, restoreTimeout, func(ctx context.Context) (bool, error) {
		current, err := s.admin.Gateways().Get(ctx, gw.ID)
		if err != nil {
			return false, nil
		}
		// Require a reconcile strictly newer than the post-detachment marker. A
		// gateway that merely stayed Running is not proof of re-attachment.
		return current.ObservedGeneration > detachedGeneration, nil
	}), "gateway reconciles again (observed generation advances past the detached marker) after the cluster re-registers")
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
