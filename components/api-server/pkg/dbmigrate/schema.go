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
