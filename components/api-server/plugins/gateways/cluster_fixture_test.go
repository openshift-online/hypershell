package gateways_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

// registerTestCluster registers a ManagedCluster the way a control plane does
// (a unique OIDC subject via POST /registration semantics) and returns its id.
// Gateway create/update handlers accept only a cluster_id that references a
// registered cluster (managed-cluster-registration.spec.md).
func registerTestCluster(t *testing.T) string {
	t.Helper()
	id, _ := registerTestClusterWithSubject(t)
	return id
}

// registerTestClusterWithSubject is registerTestCluster that also returns the
// OIDC subject the cluster registered under, so a test can act as its control
// plane.
func registerTestClusterWithSubject(t *testing.T) (string, string) {
	t.Helper()
	return mustRegisterCluster()
}

// mustRegisterCluster registers a uniquely named cluster under a unique subject
// and returns (cluster id, subject). It asserts with gomega, so it can be used
// from helpers that do not receive a *testing.T.
func mustRegisterCluster() (string, string) {
	suffix := strings.ToLower(api.NewID())
	subject := "cp-" + suffix
	name := fmt.Sprintf("mc-%s", suffix)
	svc := managedClusters.Service(&environments.Environment().Services)
	cluster, _, svcErr := svc.Register(context.Background(), name, "", subject)
	Expect(svcErr).To(BeNil(), "register test managed cluster %s", name)
	return cluster.ID, subject
}

// createManualCluster creates a ManagedCluster the way POST /managed_clusters
// does (empty oidc_subject): an inert placeholder no control plane serves.
func createManualCluster(t *testing.T) string {
	t.Helper()
	svc := managedClusters.Service(&environments.Environment().Services)
	cluster, svcErr := svc.Create(context.Background(), &managedClusters.ManagedCluster{
		Name:     "manual-" + strings.ToLower(api.NewID()),
		Provider: "kind",
	})
	if svcErr != nil {
		t.Fatalf("create manual managed cluster: %v", svcErr)
	}
	return cluster.ID
}

func openapiErrorReason(_ *test.Helper, err error) string {
	return test.APIErrorReason(err)
}
