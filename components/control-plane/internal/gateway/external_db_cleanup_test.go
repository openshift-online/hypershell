package gateway

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

var errCleanupQuery = errors.New("catalog query failed")

type cleanupQueryDriver struct{}
type cleanupQueryConn struct{ failedCatalog string }
type absentObjectRows struct{ read bool }

func (cleanupQueryDriver) Open(name string) (driver.Conn, error) {
	return &cleanupQueryConn{failedCatalog: name}, nil
}
func (*cleanupQueryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not supported")
}
func (*cleanupQueryConn) Begin() (driver.Tx, error) { return nil, errors.New("not supported") }
func (*cleanupQueryConn) Close() error              { return nil }
func (*cleanupQueryConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}
func (c *cleanupQueryConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, c.failedCatalog) {
		return nil, errCleanupQuery
	}
	return &absentObjectRows{}, nil
}
func (*absentObjectRows) Columns() []string { return []string{"exists"} }
func (*absentObjectRows) Close() error      { return nil }
func (r *absentObjectRows) Next(values []driver.Value) error {
	if r.read {
		return io.EOF
	}
	values[0] = false
	r.read = true
	return nil
}

func init() { sql.Register("hypershell-cleanup-query-test", cleanupQueryDriver{}) }

func TestExternalCleanupReturnsCatalogErrors(t *testing.T) {
	for _, catalog := range []string{"pg_database", "pg_roles"} {
		t.Run(catalog, func(t *testing.T) {
			db, err := sql.Open("hypershell-cleanup-query-test", catalog)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			err = deleteExternalSQLResources(context.Background(), db, "test")
			if !errors.Is(err, errCleanupQuery) {
				t.Fatalf("failed catalog query was treated as successful cleanup: %v", err)
			}
		})
	}
}
