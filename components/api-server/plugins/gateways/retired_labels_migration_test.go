package gateways_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/test"
	rhtrexdb "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// TestRetiredCapabilityLabelMigrations seeds a gateway placement and a cluster
// capability set that still use runtime/release-*, then re-runs the migration
// suite with only the two rewrite migrations unrecorded, as a redeploy of the new
// image would. Both must hold the purpose/ replacements afterwards, a row with no
// retired label must be untouched, and a second run must change nothing.
// Verifies: PLAT-CLUS-06.06
func TestRetiredCapabilityLabelMigrations(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	ctx := context.Background()
	g2 := h.Env().Database.SessionFactory.New(ctx)
	gatewayService := gateways.Service(&environments.Environment().Services)

	create := func(name string) *gateways.Gateway {
		gw, err := gatewayService.Create(controlPlaneWriteContext(), &gateways.Gateway{Name: name, ClusterId: "test-cluster_id"})
		Expect(err).NotTo(HaveOccurred())
		return gw
	}
	required := create("retired-required")
	excluded := create("retired-excluded")
	untouched := create("retired-untouched")
	clusterID, _ := mustRegisterCluster()

	seed := func(table, column, id, value string) {
		Expect(g2.Exec("UPDATE "+table+" SET "+column+" = CAST(? AS jsonb) WHERE id = ?", value, id).Error).To(Succeed())
	}
	seed("gateways", "placement", required.ID, `{"selector": ["network/public", "provider/ibm", "runtime/release-check"]}`)
	seed("gateways", "placement", excluded.ID, `{"selector": ["provider/ibm"], "excludedSelector": ["runtime/release-check"]}`)
	untouchedPlacement := `{"selector": ["provider/ibm", "runtime/kata"]}`
	seed("gateways", "placement", untouched.ID, untouchedPlacement)
	seed("managed_clusters", "capabilities", clusterID, `["network/public", "provider/ibm", "runtime/release-check", "runtime/release-matrix"]`)

	read := func(table, column, id string) any {
		var raw string
		Expect(g2.Raw("SELECT "+column+"::text FROM "+table+" WHERE id = ?", id).Row().Scan(&raw)).To(Succeed())
		var v any
		Expect(json.Unmarshal([]byte(raw), &v)).To(Succeed())
		return v
	}
	asJSON := func(s string) any {
		var v any
		Expect(json.Unmarshal([]byte(s), &v)).To(Succeed())
		return v
	}

	for run := 1; run <= 2; run++ {
		Expect(g2.Exec("DELETE FROM migrations WHERE id IN ('2026112518000001', '2026112518000002')").Error).To(Succeed())
		Expect(rhtrexdb.Migrate(g2)).To(Succeed(), "migration suite run %d", run)

		Expect(read("gateways", "placement", required.ID)).To(Equal(asJSON(
			`{"selector": ["network/public", "provider/ibm", "purpose/release-check"]}`)), "required selector, run %d", run)
		Expect(read("gateways", "placement", excluded.ID)).To(Equal(asJSON(
			`{"selector": ["provider/ibm"], "excludedSelector": ["purpose/release-check"]}`)), "excluded selector, run %d", run)
		Expect(read("gateways", "placement", untouched.ID)).To(Equal(asJSON(untouchedPlacement)), "untouched placement, run %d", run)
		Expect(read("managed_clusters", "capabilities", clusterID)).To(Equal(asJSON(
			`["network/public", "provider/ibm", "purpose/release-check", "purpose/release-matrix"]`)), "cluster capabilities, run %d", run)
	}
}
