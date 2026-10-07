package secretSources

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type SecretSourceDao interface {
	Get(ctx context.Context, id string) (*SecretSource, error)
	Create(ctx context.Context, secretSource *SecretSource) (*SecretSource, error)
	Replace(ctx context.Context, secretSource *SecretSource) (*SecretSource, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (SecretSourceList, error)
	All(ctx context.Context) (SecretSourceList, error)
}

var _ SecretSourceDao = &sqlSecretSourceDao{}

type sqlSecretSourceDao struct {
	sessionFactory *db.SessionFactory
}

func NewSecretSourceDao(sessionFactory *db.SessionFactory) SecretSourceDao {
	return &sqlSecretSourceDao{sessionFactory: sessionFactory}
}

func (d *sqlSecretSourceDao) Get(ctx context.Context, id string) (*SecretSource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item SecretSource
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlSecretSourceDao) Create(ctx context.Context, secretSource *SecretSource) (*SecretSource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(secretSource).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return secretSource, nil
}

func (d *sqlSecretSourceDao) Replace(ctx context.Context, secretSource *SecretSource) (*SecretSource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(secretSource).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return secretSource, nil
}

func (d *sqlSecretSourceDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&SecretSource{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlSecretSourceDao) FindByIDs(ctx context.Context, ids []string) (SecretSourceList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := SecretSourceList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlSecretSourceDao) All(ctx context.Context) (SecretSourceList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := SecretSourceList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
