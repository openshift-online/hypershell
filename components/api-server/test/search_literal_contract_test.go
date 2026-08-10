package test

import (
	"testing"

	"github.com/yaacov/tree-search-language/pkg/tsl"
	sqlFilter "github.com/yaacov/tree-search-language/pkg/walkers/sql"
)

func TestSearchLiteralUsesTSLQuoteDoubling(t *testing.T) {
	tree, err := tsl.ParseTSL("name ilike '%team''s\\_100\\%%'")
	if err != nil {
		t.Fatalf("parse TSL search: %v", err)
	}

	filter, err := sqlFilter.Walk(tree)
	if err != nil {
		t.Fatalf("translate TSL search: %v", err)
	}
	query, arguments, err := filter.ToSql()
	if err != nil {
		t.Fatalf("render TSL search: %v", err)
	}
	if query != "name ILIKE ?" {
		t.Fatalf("unexpected SQL expression: %q", query)
	}
	if len(arguments) != 1 || arguments[0] != "%team's\\_100\\%%" {
		t.Fatalf("unexpected SQL arguments: %#v", arguments)
	}
}
