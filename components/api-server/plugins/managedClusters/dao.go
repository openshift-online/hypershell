package managedClusters

import (
	"context"
	"time"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type ManagedClusterDao interface {
	Get(ctx context.Context, id string) (*ManagedCluster, error)
	Create(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, error)
	Replace(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (ManagedClusterList, error)
	All(ctx context.Context) (ManagedClusterList, error)
	FindByOIDCSubject(ctx context.Context, subject string) (*ManagedCluster, error)
	FindByName(ctx context.Context, name string) (*ManagedCluster, error)
	// FindDeletedBySubjectAndName returns the soft-deleted record registered
	// under (subject, name), or gorm.ErrRecordNotFound. The unique index on
	// (oidc_subject, name) spans soft-deleted rows, so there is at most one.
	FindDeletedBySubjectAndName(ctx context.Context, subject, name string) (*ManagedCluster, error)
	// Restore clears deleted_at on a soft-deleted record, keeping its id, and
	// stamps last_seen_at.
	Restore(ctx context.Context, id string, lastSeenAt time.Time) (*ManagedCluster, error)
	InventorySnapshot(ctx context.Context, evaluationTime time.Time) (*ClusterInventorySnapshot, error)
}

var _ ManagedClusterDao = &sqlManagedClusterDao{}

type sqlManagedClusterDao struct {
	sessionFactory *db.SessionFactory
}

func NewManagedClusterDao(sessionFactory *db.SessionFactory) ManagedClusterDao {
	return &sqlManagedClusterDao{sessionFactory: sessionFactory}
}

func (d *sqlManagedClusterDao) Get(ctx context.Context, id string) (*ManagedCluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var managedCluster ManagedCluster
	if err := g2.Take(&managedCluster, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &managedCluster, nil
}

func (d *sqlManagedClusterDao) Create(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(managedCluster).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return managedCluster, nil
}

func (d *sqlManagedClusterDao) Replace(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(managedCluster).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return managedCluster, nil
}

func (d *sqlManagedClusterDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&ManagedCluster{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlManagedClusterDao) FindByIDs(ctx context.Context, ids []string) (ManagedClusterList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	managedClusters := ManagedClusterList{}
	if err := g2.Where("id in (?)", ids).Find(&managedClusters).Error; err != nil {
		return nil, err
	}
	return managedClusters, nil
}

func (d *sqlManagedClusterDao) All(ctx context.Context) (ManagedClusterList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	managedClusters := ManagedClusterList{}
	if err := g2.Find(&managedClusters).Error; err != nil {
		return nil, err
	}
	return managedClusters, nil
}

func (d *sqlManagedClusterDao) FindByOIDCSubject(ctx context.Context, subject string) (*ManagedCluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var managedCluster ManagedCluster
	if err := g2.Take(&managedCluster, "oidc_subject = ?", subject).Error; err != nil {
		return nil, err
	}
	return &managedCluster, nil
}

func (d *sqlManagedClusterDao) FindByName(ctx context.Context, name string) (*ManagedCluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var managedCluster ManagedCluster
	if err := g2.Take(&managedCluster, "name = ?", name).Error; err != nil {
		return nil, err
	}
	return &managedCluster, nil
}

func (d *sqlManagedClusterDao) FindDeletedBySubjectAndName(ctx context.Context, subject, name string) (*ManagedCluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var managedCluster ManagedCluster
	if err := g2.Unscoped().
		Where("oidc_subject = ? AND name = ? AND deleted_at IS NOT NULL", subject, name).
		Take(&managedCluster).Error; err != nil {
		return nil, err
	}
	return &managedCluster, nil
}

func (d *sqlManagedClusterDao) Restore(ctx context.Context, id string, lastSeenAt time.Time) (*ManagedCluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Unscoped().Model(&ManagedCluster{}).Where("id = ?", id).
		Updates(map[string]interface{}{"deleted_at": nil, "last_seen_at": lastSeenAt, "updated_at": time.Now()}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return d.Get(ctx, id)
}

func (d *sqlManagedClusterDao) InventorySnapshot(ctx context.Context, evaluationTime time.Time) (*ClusterInventorySnapshot, error) {
	clusters, err := d.All(ctx)
	if err != nil {
		return nil, err
	}
	return buildClusterInventorySnapshot(clusters, evaluationTime), nil
}
