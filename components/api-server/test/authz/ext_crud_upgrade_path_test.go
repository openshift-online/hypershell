package authz_test

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	rhtrexdb "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestExtKindsUpgradePathConformance is the upgrade-path conformance test
// (SDLC-CONF-12). The fresh-database TestExtKindsCRUDConformance (SDLC-CONF-06)
// runs every migration for the first time against an empty testcontainer, so it
// can never exercise the case that broke hyp0/1/2: an environment that had
// ALREADY recorded a migration ID under an old body, where a corrected body
// shipped under the SAME ID is silently skipped (issue #862). This test
// reproduces that pre-#853 upgrade state directly in the database and asserts
// that the new-ID convergence migrations (SDLC-CONF-11) still converge it.
//
// The state it seeds matches a real upgraded hub:
//   - the clean tables (ci_providers/ci_trusts) are renamed to the
//     GORM-default-mangled names (c_iproviders/c_itrusts) the old AutoMigrate
//     body created, with ci_trusts.ci_provider_id mangled to c_iprovider_id;
//   - the ORIGINAL migration IDs stay recorded in the migrations table (so
//     gormigrate skips them, exactly as on a deployed hub);
//   - only the convergence migration IDs are removed, so re-running Migrate
//     runs just the convergence step, as a redeploy of the fixed image would.
//
// After convergence it asserts the clean tables exist, a row seeded into the
// stale table survived the copy, the stale tables are gone, and a full CRUD
// round-trip over the real HTTP+DB stack succeeds for ci_providers and ci_trusts.
func TestExtKindsUpgradePathConformance(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	ctx := context.Background()
	g2 := h.Env().Database.SessionFactory.New(ctx)

	seedPreConvergenceState(t, g2)

	// Re-run the migration suite. The original IDs remain recorded and are
	// skipped; only the convergence migrations run, as on a redeployed hub.
	Expect(rhtrexdb.Migrate(g2)).To(Succeed(), "convergence migration suite failed on the seeded upgrade state")

	// The clean tables exist and the stale mangled tables are gone.
	Expect(hasTable(g2, "ci_providers")).To(BeTrue(), "ci_providers was not created by the convergence migration")
	Expect(hasTable(g2, "ci_trusts")).To(BeTrue(), "ci_trusts was not created by the convergence migration")
	Expect(hasTable(g2, "c_iproviders")).To(BeFalse(), "stale c_iproviders was not dropped")
	Expect(hasTable(g2, "c_itrusts")).To(BeFalse(), "stale c_itrusts was not dropped")

	// The row seeded into the stale c_iproviders was copied into ci_providers.
	var copied int64
	Expect(g2.Raw(`SELECT count(*) FROM ci_providers WHERE id = ?`, seededProviderID).Scan(&copied).Error).To(Succeed())
	Expect(copied).To(Equal(int64(1)), "row from the stale table was not migrated into ci_providers")

	// A LIST over the real stack succeeds (no SQLSTATE 25P02) now that the clean
	// table exists.
	status, body := env.do(t, http.MethodGet, "ci_providers", "")
	Expect(status).To(Equal(http.StatusOK), "LIST ci_providers failed after convergence: %s", body)

	// A full CRUD round-trip over HTTP succeeds against the converged schema.
	cp, _, err := client.DefaultAPI.CreateCIProvider(env.owner.rest).
		CIProvider(*openapi.NewCIProvider("github", "https://issuer.example/upgrade-"+short(), "https://example.com/jwks", "hypershell")).Execute()
	Expect(err).NotTo(HaveOccurred(), "create ci_provider after convergence: %s", test.APIErrorReason(err))

	req := openapi.NewCITrust(cp.GetId(), env.gatewayID, openapi.CITrustAllOfSubject{Repository: openapi.PtrString("org/repo")}, "developer")
	ct, _, err := client.DefaultAPI.CreateCITrust(env.owner.rest).CITrust(*req).Execute()
	Expect(err).NotTo(HaveOccurred(), "create ci_trust after convergence: %s", test.APIErrorReason(err))

	status, body = env.do(t, http.MethodDelete, "ci_trusts/"+ct.GetId(), "")
	Expect(status).To(Or(Equal(http.StatusOK), Equal(http.StatusNoContent)), "delete ci_trust after convergence: %s", body)
	status, body = env.do(t, http.MethodDelete, "ci_providers/"+cp.GetId(), "")
	Expect(status).To(Or(Equal(http.StatusOK), Equal(http.StatusNoContent)), "delete ci_provider after convergence: %s", body)
}

// seededProviderID is the id of the legacy row planted in the stale table so the
// copy path (not just the create path) is asserted.
const seededProviderID = "upgrade-path-legacy-ci-provider"

// seedPreConvergenceState rewrites the database into the pre-convergence shape a
// deployed pre-#853 hub had: mangled stale tables present, clean tables absent,
// original migration IDs recorded, convergence IDs not yet recorded.
func seedPreConvergenceState(t *testing.T, g2 *gorm.DB) {
	t.Helper()

	// Rename the clean tables to the old GORM-default-mangled names, reproducing
	// what the pre-TableName() AutoMigrate body created. This reuses the real
	// schema rather than hand-writing legacy DDL.
	exec(t, g2, `ALTER TABLE ci_providers RENAME TO c_iproviders`)
	exec(t, g2, `ALTER TABLE ci_trusts RENAME TO c_itrusts`)
	// The old CITrust had no column tag, so its FK column was mangled too.
	exec(t, g2, `ALTER TABLE c_itrusts RENAME COLUMN ci_provider_id TO c_iprovider_id`)

	// Plant a legacy row so the convergence row-copy is exercised and asserted.
	exec(t, g2, `INSERT INTO c_iproviders (id, forge, issuer, jwks_url, audience)
		VALUES (?, 'github', 'https://legacy.example/issuer', 'https://example.com/jwks', 'hypershell')`, seededProviderID)

	// Drop only the convergence migration IDs so the suite re-runs just them; the
	// original create IDs stay recorded and are skipped, exactly as on a hub.
	exec(t, g2, `DELETE FROM migrations WHERE id IN ('2026103000000001', '2026103000000002')`)
}

func exec(t *testing.T, g2 *gorm.DB, sql string, args ...interface{}) {
	t.Helper()
	Expect(g2.Exec(sql, args...).Error).To(Succeed(), "seed sql failed: %s", sql)
}

func hasTable(g2 *gorm.DB, name string) bool {
	var count int64
	_ = g2.Raw(
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = current_schema() AND table_name = ? AND table_type = 'BASE TABLE'`,
		name,
	).Scan(&count).Error
	return count > 0
}
