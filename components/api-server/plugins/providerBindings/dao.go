package providerBindings

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type ProviderBindingDao interface {
	Get(ctx context.Context, id string) (*ProviderBinding, error)
	Create(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, error)
	Replace(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (ProviderBindingList, error)
	All(ctx context.Context) (ProviderBindingList, error)
}

var _ ProviderBindingDao = &sqlProviderBindingDao{}

type sqlProviderBindingDao struct {
	sessionFactory *db.SessionFactory
}

func NewProviderBindingDao(sessionFactory *db.SessionFactory) ProviderBindingDao {
	return &sqlProviderBindingDao{sessionFactory: sessionFactory}
}

func (d *sqlProviderBindingDao) Get(ctx context.Context, id string) (*ProviderBinding, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item ProviderBinding
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlProviderBindingDao) Create(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(providerBinding).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return providerBinding, nil
}

func (d *sqlProviderBindingDao) Replace(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(providerBinding).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return providerBinding, nil
}

func (d *sqlProviderBindingDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&ProviderBinding{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlProviderBindingDao) FindByIDs(ctx context.Context, ids []string) (ProviderBindingList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := ProviderBindingList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlProviderBindingDao) All(ctx context.Context) (ProviderBindingList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := ProviderBindingList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
