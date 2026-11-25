package dbmigrate

import (
	"fmt"

	"gorm.io/gorm"
)

// RewriteJSONColumn rewrites one JSONB column, row by row, with rewrite.
//
// It reads id and the column as text for every row (soft-deleted rows too) whose
// column text contains the substring match, so rows that cannot need a change
// are never decoded. rewrite returns the new JSON and whether the row changed;
// only changed rows are written. Running it again after a full rewrite is a no-op
// when rewrite stops changing its own output.
//
// table and column are trusted identifiers from the calling migration, never
// request input. A rewrite error aborts the migration with the row id, so a bad
// stored value is reported instead of skipped.
func RewriteJSONColumn(tx *gorm.DB, table, column, match string, rewrite func(raw []byte) ([]byte, bool, error)) error {
	type row struct {
		ID   string
		Data string
	}
	var rows []row
	query := fmt.Sprintf(`SELECT id, %[2]s::text AS data FROM %[1]s WHERE strpos(%[2]s::text, ?) > 0`, table, column)
	if err := tx.Raw(query, match).Scan(&rows).Error; err != nil {
		return fmt.Errorf("read %s.%s rows containing %q: %w", table, column, match, err)
	}
	update := fmt.Sprintf(`UPDATE %s SET %s = CAST(? AS jsonb) WHERE id = ?`, table, column)
	for _, r := range rows {
		rewritten, changed, err := rewrite([]byte(r.Data))
		if err != nil {
			return fmt.Errorf("rewrite %s.%s for row %s: %w", table, column, r.ID, err)
		}
		if !changed {
			continue
		}
		if err := tx.Exec(update, string(rewritten), r.ID).Error; err != nil {
			return fmt.Errorf("write %s.%s for row %s: %w", table, column, r.ID, err)
		}
	}
	return nil
}
