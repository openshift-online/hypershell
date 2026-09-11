package managedClusters

import (
	"context"
	"time"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

type ManagedClusterDao interface {
	Get(ctx context.Context, id string) (*ManagedCluster, error)
	Create(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, error)
	Replace(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (ManagedClusterList, error)
	All(ctx context.Context) (ManagedClusterList, error)
	FindByOIDCSubject(ctx context.Context, subject string) (*ManagedCluster, error)
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

func (d *sqlManagedClusterDao) InventorySnapshot(ctx context.Context, evaluationTime time.Time) (*ClusterInventorySnapshot, error) {
	clusters, err := d.All(ctx)
	if err != nil {
		return nil, err
	}
	return buildClusterInventorySnapshot(clusters, evaluationTime), nil
}
