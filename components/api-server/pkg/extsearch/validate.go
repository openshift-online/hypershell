// Package extsearch provides defensive validation for the shared list search
// path used by every HyperShell Kind (core and extension).
//
// The shared list/search path translates a `search=` filter into SQL via the
// upstream generic service. That path does not verify that a bare column named in
// the filter actually exists on the target table: an unknown column flows into the
// generated SQL, the row-count statement fails, and the request transaction is
// left aborted so the downstream SELECT returns 500 / SQLSTATE 25P02 (issue #871).
//
// ValidateSearchColumns closes that gap (SDLC-CONF-13(b)): it parses the search
// filter and rejects any unknown column with a clean 400 that names the bad
// column, BEFORE the clause is compiled into SQL and reaches the database, so a
// malformed or unsupported search can never poison the request transaction. The
// validation is wired in once, in the shared generic-service wrapper (see
// service.go), so it covers EVERY Kind - core kinds (gateways, managed_clusters)
// and extension kinds alike - and cannot be omitted by the next Kind author.
//
// The validation is fail-closed: it walks the parsed TSL tree itself rather than
// trusting an upstream walker to visit every column identifier. Any operand shape
// it cannot fully enumerate (an identifier hidden in an IN array, an unrecognized
// node type) is rejected with a 400 rather than passed through to SQL.
package extsearch

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/yaacov/tree-search-language/pkg/tsl"
	"gorm.io/gorm/schema"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// columnCache memoizes the parsed column set per model type so the schema is
// parsed once per Kind rather than on every list request.
var columnCache sync.Map // reflect.Type -> map[string]bool

// ValidateSearchColumns parses the TSL search expression and returns a clean 400
// (BadRequest) if it is malformed, if it names a bare column that is not a known
// column of model, or if it contains an operand this validator cannot fully
// enumerate. model is a pointer to a GORM model value (for example
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
	collect := func(field string) {
		// Only validate bare column references against this model's columns.
		// Qualified identifiers (table.column) and JSON paths are handled by the
		// upstream join resolution.
		if strings.Contains(field, ".") || strings.Contains(field, "->") {
			return
		}
		col := strings.ToLower(field)
		if !cols[col] && !seen[col] {
			seen[col] = true
			invalid = append(invalid, field)
		}
	}

	// Fail-closed walk of the parsed tree. Unlike the upstream ident.Walk (which
	// does not descend into IN-array operands, so a column hidden there would
	// reach SQL unchecked), checkNode enumerates every identifier in every operand
	// position and rejects any operand shape it cannot classify.
	if err := checkNode(tree, collect); err != nil {
		return errors.BadRequest("invalid search filter: %s", err.Error())
	}
	if len(invalid) > 0 {
		return errors.BadRequest("unknown search column(s): %s", strings.Join(invalid, ", "))
	}
	return nil
}

// checkNode walks a parsed TSL node, invoking collect for every column
// identifier it finds, and returns an error (fail-closed) for any operand shape
// it cannot enumerate. A column identifier that slips past enumeration would
// otherwise reach SQL and abort the request transaction (issue #871).
func checkNode(n tsl.Node, collect func(string)) error {
	switch n.Func {
	case tsl.IdentOp:
		name, ok := n.Left.(string)
		if !ok {
			return fmt.Errorf("unrecognized identifier operand")
		}
		collect(name)
		return nil
	case tsl.StringOp, tsl.NumberOp, tsl.NullOp:
		// Literal leaves carry a value, not a column reference.
		return nil
	default:
		if err := checkOperand(n.Left, collect); err != nil {
			return err
		}
		return checkOperand(n.Right, collect)
	}
}

// checkOperand classifies one side of a node. It descends into nested nodes and
// node arrays (for example the operand of an IN clause, which the upstream walker
// skips), treats bare literal values as benign, and fails closed on any other
// shape so an unenumerable operand can never pass validation.
func checkOperand(v interface{}, collect func(string)) error {
	switch t := v.(type) {
	case nil:
		return nil
	case tsl.Node:
		return checkNode(t, collect)
	case []tsl.Node:
		for _, child := range t {
			if err := checkNode(child, collect); err != nil {
				return err
			}
		}
		return nil
	case string, float64, bool:
		// A bare literal value (not wrapped in a node). Not a column reference.
		return nil
	default:
		return fmt.Errorf("unsupported search operand %T", v)
	}
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
