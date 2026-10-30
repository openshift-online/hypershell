package dbmigrate

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ConvergeRenamedTable idempotently converges a Kind whose effective table name
// changed after an earlier migration had already shipped and been recorded.
//
// The ci_providers/ci_trusts outage (issue #862) was this class of defect: the
// original migration ran GORM AutoMigrate on a model with no TableName(), so it
// created the GORM-default-mangled table ("c_iproviders"/"c_itrusts"). A later
// fix added TableName() and switched to CreateTable, but kept the SAME migration
// ID, so gormigrate skipped the corrected body on every environment that had
// already recorded that ID (SDLC-CONF-09). The clean table was never created and
// the stale mangled table lingered.
//
// Convergence therefore cannot live in the original migration; it must ship as a
// NEW migration ID that runs on every environment regardless of prior state
// (SDLC-CONF-11). This helper is that new migration's body. On every run it:
//
//   - creates the clean table from the model if it is missing (via CreateTable);
//   - if the stale table still exists, copies any rows into the clean table over
//     the set of columns common to both tables (there are none in practice, but
//     the copy is done safely), then drops the stale table.
//
// It is fully idempotent: on a healthy environment (clean table present, stale
// table already gone) it makes no change and returns no error, so re-running the
// deploy is a no-op. It uses only HasTable probes (plain information_schema
// SELECTs) and guarded raw SQL, never the AutoMigrate column-diff path, so it is
// safe under the production lib/pq + PreferSimpleProtocol driver and never
// poisons the migration transaction.
func ConvergeRenamedTable(tx *gorm.DB, model interface{}, staleTable string) error {
	// 1. Ensure the clean table exists. CreateTable is a no-op when present.
	if err := CreateTable(tx, model); err != nil {
		return err
	}

	m := tx.Migrator()

	// 2. Nothing to converge if the stale table is already gone.
	if !m.HasTable(staleTable) {
		return nil
	}

	cleanTable, err := resolveTableName(tx, model)
	if err != nil {
		return err
	}
	// Defensive: never treat the clean table as its own stale predecessor.
	if cleanTable == staleTable {
		return nil
	}

	// 3. Copy rows over the columns common to both tables. The mangled table was
	//    created by the old AutoMigrate body, so some column names may also be
	//    mangled (for example c_itrusts.c_iprovider_id vs ci_trusts.ci_provider_id);
	//    intersecting the column sets keeps the copy safe and never references a
	//    column that does not exist on either side. There are no rows in practice.
	common, err := commonColumns(tx, cleanTable, staleTable)
	if err != nil {
		return err
	}
	if len(common) > 0 {
		cols := make([]string, len(common))
		for i, c := range common {
			cols[i] = quoteIdent(c)
		}
		colList := strings.Join(cols, ", ")
		copySQL := fmt.Sprintf(
			"INSERT INTO %s (%s) SELECT %s FROM %s ON CONFLICT (id) DO NOTHING",
			quoteIdent(cleanTable), colList, colList, quoteIdent(staleTable),
		)
		if err := tx.Exec(copySQL).Error; err != nil {
			return fmt.Errorf("copy rows %s -> %s: %w", staleTable, cleanTable, err)
		}
	}

	// 4. Drop the stale table. IF EXISTS keeps the statement idempotent.
	if err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(staleTable))).Error; err != nil {
		return fmt.Errorf("drop stale table %s: %w", staleTable, err)
	}
	return nil
}

// resolveTableName returns the table name GORM resolves for the model, honoring
// its TableName() method and naming strategy.
func resolveTableName(tx *gorm.DB, model interface{}) (string, error) {
	stmt := &gorm.Statement{DB: tx}
	if err := stmt.Parse(model); err != nil {
		return "", fmt.Errorf("parse model %T: %w", model, err)
	}
	return stmt.Table, nil
}

// commonColumns returns the column names present in BOTH tables, in the order
// they appear on the clean (destination) table. Both lookups are plain
// information_schema SELECTs that do not poison the transaction.
func commonColumns(tx *gorm.DB, cleanTable, staleTable string) ([]string, error) {
	cleanCols, err := tableColumns(tx, cleanTable)
	if err != nil {
		return nil, err
	}
	staleSet, err := tableColumns(tx, staleTable)
	if err != nil {
		return nil, err
	}
	have := make(map[string]struct{}, len(staleSet))
	for _, c := range staleSet {
		have[c] = struct{}{}
	}
	var common []string
	for _, c := range cleanCols {
		if _, ok := have[c]; ok {
			common = append(common, c)
		}
	}
	return common, nil
}

func tableColumns(tx *gorm.DB, table string) ([]string, error) {
	var cols []string
	err := tx.Raw(
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = ?
		 ORDER BY ordinal_position`,
		table,
	).Scan(&cols).Error
	if err != nil {
		return nil, fmt.Errorf("list columns for %s: %w", table, err)
	}
	return cols, nil
}

// quoteIdent double-quotes a PostgreSQL identifier. All identifiers passed here
// originate from our own GORM schema and information_schema, never user input;
// the quoting guards only against reserved words and the acronym-mangled names.
func quoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}
