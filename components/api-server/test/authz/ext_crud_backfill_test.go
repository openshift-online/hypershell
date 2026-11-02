package authz_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	rhtrexdb "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// preNameRow is one genuine pre-migration deployed row: a row inserted directly
// into the table with no name column, whose name the backfill must derive from
// the identity columns. derivedName MUST equal the Kind's DeriveName / migration
// backfill SQL exactly, so pre-existing rows resolve identically to new rows.
type preNameRow struct {
	kind        string
	table       string
	nameMigID   string
	id          string
	derivedName string
	insertSQL   string
	insertArgs  []interface{}
}

// TestExtKindsNameBackfillUpgradePath is the name-column backfill upgrade-path
// conformance test (SDLC-CONF-13). The generic TestExtKindsCRUDConformance
// creates every row through the HTTP stack AFTER all migrations have run, so the
// row's name is always populated by the model BeforeCreate hook; it never
// exercises the migration's BACKFILL of rows that existed BEFORE the name column
// was added. That pre-column upgrade path is the exact class that broke hyp0 in
// issue #862: a corrected body shipped under an already-recorded migration ID is
// silently skipped on upgraded hubs, so the name column and its backfill reach
// those environments only under a NEW migration ID.
//
// For all four Kinds that previously lacked a queryable name column
// (ci_providers, ci_trusts, inference_routes, workspace_memberships) this test
// reproduces a genuine pre-migration deployed row directly in the database:
//
//  1. It un-records the name-column migration ID and DROPs the name column, so
//     the table is back in its pre-name shape (the migration appears never to
//     have run), then INSERTs a row directly - never through BeforeCreate - with
//     only the composite identity columns the backfill derives name from.
//  2. It re-runs the migration suite. Only the four un-recorded name migrations
//     run again (every other ID stays recorded and is skipped), exactly as a
//     redeploy of the fixed image behaves on an upgraded hub.
//  3. It asserts the pre-existing row's name is now the derived identity
//     (backfill ran) AND that find-by-name (search=name = '<derived>') resolves
//     that pre-existing row over the real HTTP + lib/pq + PreferSimpleProtocol
//     stack with 200, never a transaction-poisoning 500 / SQLSTATE 25P02.
func TestExtKindsNameBackfillUpgradePath(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	ctx := context.Background()
	g2 := h.Env().Database.SessionFactory.New(ctx)

	// Shared identity values for the seeded rows, referencing the env FK targets so
	// each row is a valid, authorized member of the caller's tenancy.
	cipID := api.NewID()
	citID := api.NewID()
	irID := api.NewID()
	wmID := api.NewID()
	cipForge := "github"
	citRole := "developer"
	irModel := "gpt-4o"
	wmSubject := "subject-" + short()

	rows := []preNameRow{
		{
			kind:      "ci_providers",
			table:     "ci_providers",
			nameMigID: "2026110213000001",
			id:        cipID,
			// Backfill: name = forge.
			derivedName: cipForge,
			insertSQL: `INSERT INTO ci_providers (id, created_at, updated_at, forge, issuer, jwks_url, audience)
						VALUES (?, NOW(), NOW(), ?, ?, ?, ?)`,
			insertArgs: []interface{}{cipID, cipForge, "https://issuer.example/backfill-" + short(), "https://example.com/jwks", "hypershell"},
		},
		{
			kind:      "ci_trusts",
			table:     "ci_trusts",
			nameMigID: "2026110213000002",
			id:        citID,
			// Backfill: name = ci_provider_id || '/' || gateway_id || '/' || role.
			derivedName: env.ciProviderID + "/" + env.gatewayID + "/" + citRole,
			insertSQL: `INSERT INTO ci_trusts (id, created_at, updated_at, ci_provider_id, gateway_id, role)
						VALUES (?, NOW(), NOW(), ?, ?, ?)`,
			insertArgs: []interface{}{citID, env.ciProviderID, env.gatewayID, citRole},
		},
		{
			kind:      "inference_routes",
			table:     "inference_routes",
			nameMigID: "2026110213000003",
			id:        irID,
			// Backfill: name = workspace_id || '/' || provider_binding_id || '/' || model.
			derivedName: env.workspaceID + "/" + env.providerBindingID + "/" + irModel,
			insertSQL: `INSERT INTO inference_routes (id, created_at, updated_at, workspace_id, provider_binding_id, model)
						VALUES (?, NOW(), NOW(), ?, ?, ?)`,
			insertArgs: []interface{}{irID, env.workspaceID, env.providerBindingID, irModel},
		},
		{
			kind:      "workspace_memberships",
			table:     "workspace_memberships",
			nameMigID: "2026110213000004",
			id:        wmID,
			// Backfill: name = workspace_id || '/' || subject.
			derivedName: env.workspaceID + "/" + wmSubject,
			insertSQL: `INSERT INTO workspace_memberships (id, created_at, updated_at, workspace_id, subject, role)
						VALUES (?, NOW(), NOW(), ?, ?, ?)`,
			insertArgs: []interface{}{wmID, env.workspaceID, wmSubject, "member"},
		},
	}

	// Seed every Kind's pre-migration row before running the suite once, so the
	// single Migrate call re-runs all four un-recorded name migrations together,
	// as a single redeploy would.
	for _, r := range rows {
		seedPreNameRow(t, g2, r)
	}

	// Re-run the migration suite. The original create IDs stay recorded and are
	// skipped; only the four un-recorded name migrations run, adding the column
	// back and backfilling the pre-existing rows (SDLC-CONF-13).
	Expect(rhtrexdb.Migrate(g2)).To(Succeed(), "name-column backfill migrations failed on the seeded pre-migration rows")

	for _, r := range rows {
		r := r
		t.Run(r.kind, func(t *testing.T) {
			RegisterTestingT(t)

			// The backfill populated the pre-existing row's name to the derived
			// identity, at the database level.
			var got string
			err := g2.Raw(`SELECT name FROM `+r.table+` WHERE id = ?`, r.id).Scan(&got).Error
			Expect(err).NotTo(HaveOccurred(), "read back %s name for %s", r.table, r.id)
			Expect(got).To(Equal(r.derivedName),
				"backfill did not derive %s.name for pre-existing row %s", r.table, r.id)

			// find-by-name resolves the pre-existing row over the real HTTP stack
			// with 200 (no 25P02): the migration added a queryable name column on
			// the upgraded database, and the backfilled value is the search key.
			assertFindByName(t, env, r.kind, r.id, r.derivedName)
		})
	}
}

// seedPreNameRow rewrites one table into its pre-name-migration shape: the name
// migration is un-recorded and the name column is dropped, then the row is
// inserted directly (never through BeforeCreate) with only the backfill's
// identity columns. Re-running the suite then re-adds the column and backfills
// this row.
func seedPreNameRow(t *testing.T, g2 *gorm.DB, r preNameRow) {
	t.Helper()
	// Un-record the name migration so the suite re-runs just it.
	exec(t, g2, `DELETE FROM migrations WHERE id = ?`, r.nameMigID)
	// Drop the name column so the table is in its genuine pre-migration shape; the
	// inserted row then has no name, exactly like a row deployed before the column
	// existed. IF EXISTS keeps the seed idempotent.
	exec(t, g2, `ALTER TABLE `+r.table+` DROP COLUMN IF EXISTS name`)
	// Insert the pre-migration row directly, bypassing the model BeforeCreate hook
	// that would otherwise populate name.
	exec(t, g2, r.insertSQL, r.insertArgs...)
}
