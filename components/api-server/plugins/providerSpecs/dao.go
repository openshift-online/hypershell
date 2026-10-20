package providerSpecs

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type ProviderSpecDao interface {
	Get(ctx context.Context, id string) (*ProviderSpec, error)
	Create(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, error)
	Replace(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (ProviderSpecList, error)
	All(ctx context.Context) (ProviderSpecList, error)
}

var _ ProviderSpecDao = &sqlProviderSpecDao{}

type sqlProviderSpecDao struct {
	sessionFactory *db.SessionFactory
}

func NewProviderSpecDao(sessionFactory *db.SessionFactory) ProviderSpecDao {
	return &sqlProviderSpecDao{sessionFactory: sessionFactory}
}

func (d *sqlProviderSpecDao) Get(ctx context.Context, id string) (*ProviderSpec, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item ProviderSpec
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlProviderSpecDao) Create(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(providerSpec).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return providerSpec, nil
}

func (d *sqlProviderSpecDao) Replace(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(providerSpec).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return providerSpec, nil
}

func (d *sqlProviderSpecDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&ProviderSpec{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlProviderSpecDao) FindByIDs(ctx context.Context, ids []string) (ProviderSpecList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := ProviderSpecList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlProviderSpecDao) All(ctx context.Context) (ProviderSpecList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := ProviderSpecList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
