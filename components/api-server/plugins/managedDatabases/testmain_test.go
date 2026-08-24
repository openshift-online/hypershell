package managedDatabases_test

import (
	"flag"
	"os"
	"runtime"
	"testing"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/golang/glog"
	"gorm.io/gorm"

	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func TestMain(m *testing.M) {
	// ManagedDatabase deletion checks the gateways table, which the focused
	// plugin test binary does not otherwise migrate. Register only the minimal
	// cross-plugin contract needed by these tests instead of loading every
	// Gateway migration and dependency.
	db.RegisterMigration(&gormigrate.Migration{
		ID: "2026082400000099",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`CREATE TABLE IF NOT EXISTS gateways (
				id TEXT PRIMARY KEY,
				database_id TEXT,
				deleted_at TIMESTAMPTZ
			)`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS gateways").Error
		},
	})

	flag.Parse()
	glog.Infof("Starting managedDatabases integration test using go version %s", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
