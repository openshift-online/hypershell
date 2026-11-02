// Package dbmigrate provides driver-safe, idempotent migration primitives that
// replace GORM AutoMigrate in every plugin migration.
//
// GORM AutoMigrate is forbidden across all Kinds (SDLC-CONF-02,
// specs/standards/platform/cross-cutting.spec.md): its column-diff introspection
// fails closed on the production PostgreSQL driver and protocol (lib/pq with
// PreferSimpleProtocol), which poisons the transaction and aborts every
// subsequent query on the table with SQLSTATE 25P02. The ci_providers/ci_trusts
// outage on hyp0 was exactly this failure.
//
// CreateTable renders the table DDL from the GORM-parsed model schema (the same
// CREATE TABLE statement AutoMigrate's create path would emit) and executes it
// as raw SQL, guarded by HasTable so a re-run is a no-op. It never calls the
// AutoMigrate column-diff path. Columns added after a table's first migration
// are evolved with a separate migration using raw ALTER TABLE ... ADD COLUMN
// IF NOT EXISTS inline in that plugin's migration, so schema evolution never
// depends on AutoMigrate introspection either.
package dbmigrate

import (
	"fmt"

	"gorm.io/gorm"
)

// CreateTable creates the table(s) for the given model(s) from the GORM schema,
// idempotently. A model whose table already exists is skipped, so running the
// migration twice makes no change and does not error (SDLC-CONF-02.02). It
// deliberately avoids gorm AutoMigrate; it uses Migrator.CreateTable, which
// emits a plain CREATE TABLE (with the model's indexes and constraints) as raw
// SQL and does not run AutoMigrate's fail-closed column introspection.
//
// Columns added to a model after its table was first created are NOT applied by
// re-running CreateTable (the table already exists); add them in a separate
// migration with inline raw ALTER TABLE ... ADD COLUMN IF NOT EXISTS.
func CreateTable(tx *gorm.DB, models ...interface{}) error {
	m := tx.Migrator()
	for _, model := range models {
		if m.HasTable(model) {
			continue
		}
		if err := m.CreateTable(model); err != nil {
			return fmt.Errorf("create table for %T: %w", model, err)
		}
	}
	return nil
}

// AddUniqueNameColumn is the shared SDLC-CONF-13 name-column evolution primitive
// used by every ext Kind that gained a find-by-name key after its table had
// already shipped. find-by-name REQUIRES name to be unique per Kind (an ambiguous
// name makes `hsctl apply` resolve the wrong record), but adding a unique index
// must NEVER be able to brick API-server startup on upgraded data. This helper
// guarantees both. In order it:
//
//  1. adds a nullable `name text` column (idempotent ADD COLUMN IF NOT EXISTS);
//  2. backfills NULL/empty names from nameExpr - a raw SQL expression over the
//     row's own columns (for example "COALESCE(forge,”)" for a single-column key,
//     or the COALESCE-joined composite natural key for a multi-column Kind) - so a
//     pre-existing row resolves identically to a row the model BeforeCreate hook
//     would have named;
//  3. de-duplicates colliding names COLLISION-SAFELY: nameExpr is not guaranteed
//     unique on pre-existing data (duplicate forges, or two rows with the same
//     composite identity), so for every set of rows sharing a name it keeps the
//     smallest-id row's name and appends "/" || id to the rest. id is the KSUID
//     primary key (globally unique), so "name/id" is unique by construction and
//     the UNIQUE index in step 4 can ALWAYS build - a duplicate-key value can
//     never abort the migration, roll gormigrate back, and prevent the server from
//     starting. The UPDATE is idempotent: once resolved the names differ, so a
//     re-run matches no rows;
//  4. creates the UNIQUE index idx_<table>_name so find-by-name resolves exactly
//     one row (the index name matches GORM's uniqueIndex tag on the model, so a
//     fresh database that already built it via CreateTable is an IF NOT EXISTS
//     no-op here).
//
// All statements are idempotent raw SQL safe under the production lib/pq +
// PreferSimpleProtocol driver; it never calls AutoMigrate (SDLC-CONF-02).
func AddUniqueNameColumn(tx *gorm.DB, table, nameExpr string) error {
	if err := tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN IF NOT EXISTS name text`).Error; err != nil {
		return err
	}
	if err := tx.Exec(`UPDATE ` + table + ` SET name = ` + nameExpr + ` WHERE name IS NULL OR name = ''`).Error; err != nil {
		return err
	}
	// Collision-safe de-duplication. A row is suffixed with its own id only when an
	// earlier-id row already holds the same name, so exactly one row per collision
	// set keeps the derived name and all others become "name/id" (unique by the PK).
	if err := tx.Exec(`UPDATE ` + table + ` AS t SET name = t.name || '/' || t.id ` +
		`WHERE EXISTS (SELECT 1 FROM ` + table + ` AS d WHERE d.name = t.name AND d.id < t.id)`).Error; err != nil {
		return err
	}
	return tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_` + table + `_name ON ` + table + ` (name)`).Error
}

// DropUniqueNameColumn reverses AddUniqueNameColumn: it drops the unique name
// index, then the name column. Idempotent via IF EXISTS so a partial-apply
// rollback is safe.
func DropUniqueNameColumn(tx *gorm.DB, table string) error {
	if err := tx.Exec(`DROP INDEX IF EXISTS idx_` + table + `_name`).Error; err != nil {
		return err
	}
	return tx.Exec(`ALTER TABLE ` + table + ` DROP COLUMN IF EXISTS name`).Error
}
