package managedDatabases

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

type ManagedDatabaseDao interface {
	Get(ctx context.Context, id string) (*ManagedDatabase, error)
	GetUnscoped(ctx context.Context, id string) (*ManagedDatabase, error)
	Create(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, error)
	Replace(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (ManagedDatabaseList, error)
	All(ctx context.Context) (ManagedDatabaseList, error)
	FindSoleInFleet(ctx context.Context, fleetID string) (*ManagedDatabase, error)
	ExistsByDatabaseID(ctx context.Context, databaseID string) (bool, error)
}

var _ ManagedDatabaseDao = &sqlManagedDatabaseDao{}

type sqlManagedDatabaseDao struct {
	sessionFactory *db.SessionFactory
}

func NewManagedDatabaseDao(sessionFactory *db.SessionFactory) ManagedDatabaseDao {
	return &sqlManagedDatabaseDao{sessionFactory: sessionFactory}
}

func (d *sqlManagedDatabaseDao) Get(ctx context.Context, id string) (*ManagedDatabase, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var managedDatabase ManagedDatabase
	if err := g2.Take(&managedDatabase, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &managedDatabase, nil
}

func (d *sqlManagedDatabaseDao) GetUnscoped(ctx context.Context, id string) (*ManagedDatabase, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var managedDatabase ManagedDatabase
	if err := g2.Unscoped().Take(&managedDatabase, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &managedDatabase, nil
}

func (d *sqlManagedDatabaseDao) Create(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(managedDatabase).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return managedDatabase, nil
}

func (d *sqlManagedDatabaseDao) Replace(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(managedDatabase).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return managedDatabase, nil
}

func (d *sqlManagedDatabaseDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&ManagedDatabase{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlManagedDatabaseDao) FindByIDs(ctx context.Context, ids []string) (ManagedDatabaseList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	managedDatabases := ManagedDatabaseList{}
	if err := g2.Where("id in (?)", ids).Find(&managedDatabases).Error; err != nil {
		return nil, err
	}
	return managedDatabases, nil
}

func (d *sqlManagedDatabaseDao) All(ctx context.Context) (ManagedDatabaseList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	managedDatabases := ManagedDatabaseList{}
	if err := g2.Find(&managedDatabases).Error; err != nil {
		return nil, err
	}
	return managedDatabases, nil
}

func (d *sqlManagedDatabaseDao) FindSoleInFleet(ctx context.Context, fleetID string) (*ManagedDatabase, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var databases []ManagedDatabase
	if err := g2.Where("fleet_id = ?", fleetID).Find(&databases).Error; err != nil {
		return nil, err
	}
	if len(databases) == 1 {
		return &databases[0], nil
	}
	return nil, nil
}

func (d *sqlManagedDatabaseDao) ExistsByDatabaseID(ctx context.Context, databaseID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Raw("SELECT COUNT(*) FROM gateways WHERE database_id = ? AND deleted_at IS NULL", databaseID).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
