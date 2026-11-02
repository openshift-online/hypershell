// Package extsearch provides defensive validation for the shared Extension API
// list search path.
//
// The shared ext list/search path translates a `search=` filter into SQL via the
// upstream generic service. That path does not verify that a bare column named in
// the filter actually exists on the target table: an unknown column flows into the
// generated SQL, the row-count statement fails, and the request transaction is
// left aborted so the downstream SELECT returns 500 / SQLSTATE 25P02 (issue #871).
//
// ValidateSearchColumns closes that gap in the hypershell ext handler layer
// (SDLC-CONF-13(b)): it parses the search filter and rejects any unknown column
// with a clean 400 that names the bad column, BEFORE the clause is compiled into
// SQL and reaches the database, so a malformed or unsupported search can never
// poison the request transaction. It is defensive depth in addition to the
// per-Kind queryable `name` column: even a Kind that legitimately lacks a searched
// column answers with a 400, not a 25P02.
package extsearch

import (
	"reflect"
	"strings"
	"sync"

	"github.com/yaacov/tree-search-language/pkg/tsl"
	"github.com/yaacov/tree-search-language/pkg/walkers/ident"
	"gorm.io/gorm/schema"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// columnCache memoizes the parsed column set per model type so the schema is
// parsed once per Kind rather than on every list request.
var columnCache sync.Map // reflect.Type -> map[string]bool

// ValidateSearchColumns parses the TSL search expression and returns a clean 400
// (BadRequest) if it is malformed, or if it names a bare column that is not a
// known column of model. model is a pointer to a GORM model value (for example
// &CIProvider{}); its column set is derived from the GORM schema, so column tag
// overrides (for example ci_provider_id) and the embedded api.Meta columns (id,
// created_at, updated_at, deleted_at) are all honored.
//
// An empty search is valid (returns nil). Qualified identifiers (table.column and
// JSON `->` paths) are left to the shared join-resolution layer, which already
// reports an unknown related resource as a 400; this function guards the bare
// column case that otherwise reaches SQL unchecked.
func ValidateSearchColumns(search string, model interface{}) *errors.ServiceError {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil
	}

	tree, err := tsl.ParseTSL(search)
	if err != nil {
		return errors.BadRequest("invalid search filter: %s", search)
	}

	cols, serr := columnSet(model)
	if serr != nil {
		return serr
	}

	var invalid []string
	seen := map[string]bool{}
	walkFn := func(field string) (string, error) {
		// Only validate bare column references against this model's columns.
		// Qualified identifiers (table.column) and JSON paths are handled by the
		// upstream join resolution.
		if strings.Contains(field, ".") || strings.Contains(field, "->") {
			return field, nil
		}
		col := strings.ToLower(field)
		if !cols[col] && !seen[col] {
			seen[col] = true
			invalid = append(invalid, field)
		}
		return field, nil
	}
	if _, err := ident.Walk(tree, walkFn); err != nil {
		return errors.BadRequest("invalid search filter: %s", err.Error())
	}
	if len(invalid) > 0 {
		return errors.BadRequest("unknown search column(s): %s", strings.Join(invalid, ", "))
	}
	return nil
}

// columnSet returns the lowercase set of database column names for model, parsed
// from the GORM schema and memoized per type.
func columnSet(model interface{}) (map[string]bool, *errors.ServiceError) {
	t := reflect.TypeOf(model)
	if cached, ok := columnCache.Load(t); ok {
		return cached.(map[string]bool), nil
	}
	sch, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		return nil, errors.GeneralError("cannot parse search model schema: %s", err.Error())
	}
	cols := make(map[string]bool, len(sch.DBNames))
	for _, name := range sch.DBNames {
		cols[strings.ToLower(name)] = true
	}
	columnCache.Store(t, cols)
	return cols, nil
}
