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
	// A forge distinct from the env's own ci_provider (which defaults its name to
	// "github"): ci_providers.name is now uniquely indexed (one provider per
	// forge, SDLC-CONF-13 / find-by-name ambiguity), so the seeded pre-migration
	// row must backfill to a different name than the env row.
	cipForge := "gitlab"
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
			// NULL identity column (role) on a composite kind: the backfill derives
			// name = COALESCE(ci_provider_id,'') || '/' || COALESCE(gateway_id,'') ||
			// '/' || COALESCE(role,''). Without the COALESCE, a NULL role makes the
			// whole `a || '/' || b` expression NULL, leaving this row's name NULL and
			// unresolvable by find-by-name. The role column is omitted from the INSERT
			// below so it is genuinely NULL, exercising that exact case (FINDING 3).
			derivedName: env.ciProviderID + "/" + env.gatewayID + "/",
			insertSQL: `INSERT INTO ci_trusts (id, created_at, updated_at, ci_provider_id, gateway_id)
						VALUES (?, NOW(), NOW(), ?, ?)`,
			insertArgs: []interface{}{citID, env.ciProviderID, env.gatewayID},
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

// collisionPair is two genuine pre-migration rows that backfill to the SAME name:
// a single-column key with a duplicate value (ci_providers: same forge) or a
// composite natural key with the same identity (the other three Kinds). The
// migration must de-duplicate them collision-safely so the UNIQUE index can build.
type collisionPair struct {
	kind      string
	table     string
	nameMigID string
	// idLow/idHigh are the two row ids; idLow sorts before idHigh, so after
	// de-duplication idLow keeps baseName and idHigh is suffixed to baseName/idHigh.
	idLow      string
	idHigh     string
	baseName   string
	insertSQL  string
	argsForRow func(id string) []interface{}
}

// TestExtKindsNameBackfillDeduplicatesCollisions is the collision-safe backfill
// regression (FINDING 1 and FINDING 3). The name backfill is NOT unique by
// construction on pre-existing data: two ci_providers can share a forge, and two
// composite-key rows can share an identity. If the migration created the UNIQUE
// name index without de-duplicating first, CREATE UNIQUE INDEX would fail on the
// duplicate, gormigrate would roll the migration back, and the API server would
// refuse to start on upgraded data.
//
// For every affected Kind this test seeds TWO pre-migration rows that backfill to
// the SAME name, re-runs the migration suite, and asserts:
//   - the migration COMPLETES (no duplicate-key failure, no rollback);
//   - both rows end up with distinct, non-empty names (one keeps the derived name,
//     the other is suffixed with its row id);
//   - the UNIQUE name index exists;
//   - find-by-name on the derived name resolves EXACTLY ONE row (the other no
//     longer answers to the base name, so hsctl apply can never resolve the wrong
//     record).
//
// Without the de-duplication in dbmigrate.AddUniqueNameColumn the Migrate call
// itself fails, so this whole test fails.
func TestExtKindsNameBackfillDeduplicatesCollisions(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	ctx := context.Background()
	g2 := h.Env().Database.SessionFactory.New(ctx)

	// A forge distinct from the env's own ci_provider (default name "github") so the
	// collision is strictly between the two seeded rows, not with the env row.
	dupForge := "forgejo"
	dupSubject := "subject-" + short()
	dupModel := "gpt-4o-dedup"
	dupRole := "duprole"

	pairs := []collisionPair{
		{
			kind:      "ci_providers",
			table:     "ci_providers",
			nameMigID: "2026110213000001",
			baseName:  dupForge, // FINDING 1: two providers on the same forge.
			insertSQL: `INSERT INTO ci_providers (id, created_at, updated_at, forge, issuer, jwks_url, audience)
						VALUES (?, NOW(), NOW(), ?, ?, ?, ?)`,
			argsForRow: func(id string) []interface{} {
				// Issuer is also uniquely indexed, so each row needs a distinct issuer;
				// only the forge (and thus the derived name) collides.
				return []interface{}{id, dupForge, "https://issuer.example/dedup-" + id, "https://example.com/jwks", "hypershell"}
			},
		},
		{
			kind:      "ci_trusts",
			table:     "ci_trusts",
			nameMigID: "2026110213000002",
			// FINDING 3: same composite identity (ci_provider_id/gateway_id/role).
			baseName: env.ciProviderID + "/" + env.gatewayID + "/" + dupRole,
			insertSQL: `INSERT INTO ci_trusts (id, created_at, updated_at, ci_provider_id, gateway_id, role)
						VALUES (?, NOW(), NOW(), ?, ?, ?)`,
			argsForRow: func(id string) []interface{} {
				return []interface{}{id, env.ciProviderID, env.gatewayID, dupRole}
			},
		},
		{
			kind:      "inference_routes",
			table:     "inference_routes",
			nameMigID: "2026110213000003",
			baseName:  env.workspaceID + "/" + env.providerBindingID + "/" + dupModel,
			insertSQL: `INSERT INTO inference_routes (id, created_at, updated_at, workspace_id, provider_binding_id, model)
						VALUES (?, NOW(), NOW(), ?, ?, ?)`,
			argsForRow: func(id string) []interface{} {
				return []interface{}{id, env.workspaceID, env.providerBindingID, dupModel}
			},
		},
		{
			kind:      "workspace_memberships",
			table:     "workspace_memberships",
			nameMigID: "2026110213000004",
			baseName:  env.workspaceID + "/" + dupSubject,
			insertSQL: `INSERT INTO workspace_memberships (id, created_at, updated_at, workspace_id, subject, role)
						VALUES (?, NOW(), NOW(), ?, ?, ?)`,
			argsForRow: func(id string) []interface{} {
				return []interface{}{id, env.workspaceID, dupSubject, "member"}
			},
		},
	}

	// Assign a sorted id pair to each kind, then seed both colliding rows.
	for i := range pairs {
		pairs[i].idLow, pairs[i].idHigh = sortedIDPair()
		seedCollisionPair(t, g2, pairs[i])
	}

	// Re-run the migration suite. Only the four un-recorded name migrations run.
	// This MUST succeed: without collision-safe de-duplication the CREATE UNIQUE
	// INDEX would fail on the duplicate name and roll the migration back.
	Expect(rhtrexdb.Migrate(g2)).To(Succeed(),
		"name-column migration failed on colliding pre-migration rows (de-dup missing? unique index could not build)")

	for _, p := range pairs {
		p := p
		t.Run(p.kind, func(t *testing.T) {
			RegisterTestingT(t)

			// Both rows have distinct, non-empty names: the low-id row keeps the
			// derived base name, the high-id row is suffixed with its own id.
			var lowName, highName string
			Expect(g2.Raw(`SELECT name FROM `+p.table+` WHERE id = ?`, p.idLow).Scan(&lowName).Error).To(Succeed())
			Expect(g2.Raw(`SELECT name FROM `+p.table+` WHERE id = ?`, p.idHigh).Scan(&highName).Error).To(Succeed())
			Expect(lowName).To(Equal(p.baseName), "low-id row should keep the derived name for %s", p.table)
			Expect(highName).To(Equal(p.baseName+"/"+p.idHigh), "high-id row should be suffixed with its id for %s", p.table)
			Expect(lowName).NotTo(Equal(highName), "collision was not de-duplicated for %s", p.table)

			// The UNIQUE name index exists (it could only build because the names
			// were made distinct first).
			Expect(hasIndex(g2, "idx_"+p.table+"_name")).To(BeTrue(),
				"unique name index missing for %s", p.table)

			// find-by-name on the base name resolves EXACTLY ONE row: the low-id row
			// answers to it, the high-id row no longer does.
			assertFindByName(t, env, p.kind, p.idLow, p.baseName)
			assertNotResolvableByName(t, env, p.kind, p.idHigh, p.baseName)
			// The suffixed name resolves the high-id row.
			assertFindByName(t, env, p.kind, p.idHigh, highName)
		})
	}
}

// seedCollisionPair seeds two pre-migration rows that backfill to the same name,
// after un-recording the name migration and dropping the name column (so the
// re-run migration re-adds the column, backfills, and de-duplicates).
func seedCollisionPair(t *testing.T, g2 *gorm.DB, p collisionPair) {
	t.Helper()
	exec(t, g2, `DELETE FROM migrations WHERE id = ?`, p.nameMigID)
	exec(t, g2, `ALTER TABLE `+p.table+` DROP COLUMN IF EXISTS name`)
	exec(t, g2, p.insertSQL, p.argsForRow(p.idLow)...)
	exec(t, g2, p.insertSQL, p.argsForRow(p.idHigh)...)
}

// sortedIDPair returns two distinct ids, the lexicographically smaller first, so a
// test can predict which row the de-duplication keeps (smallest id) and which it
// suffixes.
func sortedIDPair() (string, string) {
	a, b := api.NewID(), api.NewID()
	if a > b {
		return b, a
	}
	return a, b
}

// hasIndex reports whether an index with the given name exists in the current
// schema.
func hasIndex(g2 *gorm.DB, name string) bool {
	var count int64
	_ = g2.Raw(`SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`, name).Scan(&count).Error
	return count > 0
}

// TestExtKindsNameBackfillDedupIgnoresSoftDeleted is the live-scoped de-duplication
// regression for FINDING 2. The backfill de-duplication must consider LIVE rows
// only (deleted_at IS NULL on both sides). The prior UPDATE ordered by id alone and
// ignored deleted_at, so an OLDER soft-deleted row sharing a name kept the clean
// derived name and the UPDATE suffixed the LIVE newer row - leaving the live,
// find-by-name-visible record under a mangled "name/id" key while a tombstone held
// the clean name.
//
// This seeds, in a genuine pre-name shape, a soft-deleted row with the SMALLER id
// and a live row with the LARGER id, both backfilling to the same derived name,
// then re-runs the migration suite and asserts the LIVE row keeps the clean name
// and find-by-name resolves it. Under the old id-only de-duplication the live row
// would be suffixed, so this test fails.
func TestExtKindsNameBackfillDedupIgnoresSoftDeleted(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	env := newConformanceEnv(t, h, client)

	ctx := context.Background()
	g2 := h.Env().Database.SessionFactory.New(ctx)

	const nameMigID = "2026110213000002" // ci_trusts
	role := "softdel-dup-" + short()
	baseName := env.ciProviderID + "/" + env.gatewayID + "/" + role

	// deletedID < liveID so the old id-only de-duplication would keep the
	// soft-deleted (smaller-id) row's name and suffix the live (larger-id) row.
	deletedID, liveID := sortedIDPair()

	// Rewrite ci_trusts into its pre-name shape, then seed both rows directly.
	exec(t, g2, `DELETE FROM migrations WHERE id = ?`, nameMigID)
	exec(t, g2, `ALTER TABLE ci_trusts DROP COLUMN IF EXISTS name`)
	// Soft-deleted row (smaller id): deleted_at is set.
	exec(t, g2, `INSERT INTO ci_trusts (id, created_at, updated_at, deleted_at, ci_provider_id, gateway_id, role)
				VALUES (?, NOW(), NOW(), NOW(), ?, ?, ?)`, deletedID, env.ciProviderID, env.gatewayID, role)
	// Live row (larger id): deleted_at NULL.
	exec(t, g2, `INSERT INTO ci_trusts (id, created_at, updated_at, ci_provider_id, gateway_id, role)
				VALUES (?, NOW(), NOW(), ?, ?, ?)`, liveID, env.ciProviderID, env.gatewayID, role)

	Expect(rhtrexdb.Migrate(g2)).To(Succeed(),
		"name migration failed on a soft-deleted/live name collision")

	// The LIVE row keeps the clean derived name; the soft-deleted older row must not
	// steal it (FINDING 2).
	var liveName string
	Expect(g2.Raw(`SELECT name FROM ci_trusts WHERE id = ?`, liveID).Scan(&liveName).Error).To(Succeed())
	Expect(liveName).To(Equal(baseName),
		"the live row must keep the clean name; a soft-deleted older row stole it (de-dup not live-scoped?)")

	// The partial unique index exists and find-by-name resolves the live row.
	Expect(hasIndex(g2, "idx_ci_trusts_name")).To(BeTrue(), "partial unique name index missing")
	assertFindByName(t, env, "ci_trusts", liveID, baseName)
}
