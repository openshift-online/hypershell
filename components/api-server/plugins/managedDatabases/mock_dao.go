package managedDatabases

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/pkg/errors"
)

var _ ManagedDatabaseDao = &managedDatabaseDaoMock{}

type managedDatabaseDaoMock struct {
	managedDatabases ManagedDatabaseList
}

func NewMockManagedDatabaseDao() *managedDatabaseDaoMock {
	return &managedDatabaseDaoMock{}
}

func (d *managedDatabaseDaoMock) Get(ctx context.Context, id string) (*ManagedDatabase, error) {
	for _, managedDatabase := range d.managedDatabases {
		if managedDatabase.ID == id {
			return managedDatabase, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *managedDatabaseDaoMock) GetUnscoped(ctx context.Context, id string) (*ManagedDatabase, error) {
	return d.Get(ctx, id)
}

func (d *managedDatabaseDaoMock) ListDeleted(context.Context, int, int) ([]ManagedDatabase, error) {
	return nil, nil
}

func (d *managedDatabaseDaoMock) Create(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, error) {
	d.managedDatabases = append(d.managedDatabases, managedDatabase)
	return managedDatabase, nil
}

func (d *managedDatabaseDaoMock) Replace(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, error) {
	return nil, errors.NotImplemented("ManagedDatabase").AsError()
}

func (d *managedDatabaseDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("ManagedDatabase").AsError()
}

func (d *managedDatabaseDaoMock) FindByIDs(ctx context.Context, ids []string) (ManagedDatabaseList, error) {
	return nil, errors.NotImplemented("ManagedDatabase").AsError()
}

func (d *managedDatabaseDaoMock) All(ctx context.Context) (ManagedDatabaseList, error) {
	return d.managedDatabases, nil
}

func (d *managedDatabaseDaoMock) ExistsByDatabaseID(ctx context.Context, databaseID string) (bool, error) {
	return false, nil
}
