package dbmigrate

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func mockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

var (
	selectQuery = regexp.QuoteMeta(`SELECT id, labels::text AS data FROM things WHERE strpos(labels::text, $1) > 0`)
	updateQuery = regexp.QuoteMeta(`UPDATE things SET labels = CAST($1 AS jsonb) WHERE id = $2`)
)

// upper rewrites a row to upper case and reports no change when it already is.
func upper(raw []byte) ([]byte, bool, error) {
	out := bytes.ToUpper(raw)
	return out, !bytes.Equal(out, raw), nil
}

// Only rows the rewrite changed are written, with their new value.
func TestRewriteJSONColumnWritesOnlyChangedRows(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(selectQuery).WithArgs("old").
		WillReturnRows(sqlmock.NewRows([]string{"id", "data"}).
			AddRow("a", `["old"]`).
			AddRow("b", `["OLD"]`))
	mock.ExpectExec(updateQuery).WithArgs(`["OLD"]`, "a").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := RewriteJSONColumn(db, "things", "labels", "old", upper); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Every failure aborts the migration and names the table, column and row.
func TestRewriteJSONColumnReportsFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name    string
		setup   func(sqlmock.Sqlmock)
		rewrite func([]byte) ([]byte, bool, error)
		want    string
	}{
		{
			name:    "read",
			setup:   func(m sqlmock.Sqlmock) { m.ExpectQuery(selectQuery).WillReturnError(boom) },
			rewrite: upper,
			want:    "read things.labels",
		},
		{
			name: "rewrite",
			setup: func(m sqlmock.Sqlmock) {
				m.ExpectQuery(selectQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "data"}).AddRow("a", `["old"]`))
			},
			rewrite: func([]byte) ([]byte, bool, error) { return nil, false, boom },
			want:    "rewrite things.labels for row a",
		},
		{
			name: "write",
			setup: func(m sqlmock.Sqlmock) {
				m.ExpectQuery(selectQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "data"}).AddRow("a", `["old"]`))
				m.ExpectExec(updateQuery).WillReturnError(boom)
			},
			rewrite: upper,
			want:    "write things.labels for row a",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := mockDB(t)
			tc.setup(mock)
			err := RewriteJSONColumn(db, "things", "labels", "old", tc.rewrite)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, boom) {
				t.Fatalf("error = %v, want one containing %q that wraps the cause", err, tc.want)
			}
		})
	}
}
