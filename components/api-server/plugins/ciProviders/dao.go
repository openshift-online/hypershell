package ciProviders

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type CIProviderDao interface {
	Get(ctx context.Context, id string) (*CIProvider, error)
	Create(ctx context.Context, ciProvider *CIProvider) (*CIProvider, error)
	Replace(ctx context.Context, ciProvider *CIProvider) (*CIProvider, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (CIProviderList, error)
	All(ctx context.Context) (CIProviderList, error)
	FindByIssuer(ctx context.Context, issuer string) (*CIProvider, error)
	ReferencingCITrustIDs(ctx context.Context, ciProviderID string) ([]string, error)
}

var _ CIProviderDao = &sqlCIProviderDao{}

type sqlCIProviderDao struct {
	sessionFactory *db.SessionFactory
}

func NewCIProviderDao(sessionFactory *db.SessionFactory) CIProviderDao {
	return &sqlCIProviderDao{sessionFactory: sessionFactory}
}

func (d *sqlCIProviderDao) Get(ctx context.Context, id string) (*CIProvider, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item CIProvider
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlCIProviderDao) Create(ctx context.Context, ciProvider *CIProvider) (*CIProvider, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(ciProvider).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return ciProvider, nil
}

func (d *sqlCIProviderDao) Replace(ctx context.Context, ciProvider *CIProvider) (*CIProvider, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(ciProvider).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return ciProvider, nil
}

func (d *sqlCIProviderDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&CIProvider{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlCIProviderDao) FindByIDs(ctx context.Context, ids []string) (CIProviderList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := CIProviderList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlCIProviderDao) All(ctx context.Context) (CIProviderList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := CIProviderList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlCIProviderDao) FindByIssuer(ctx context.Context, issuer string) (*CIProvider, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item CIProvider
	if err := g2.Where("issuer = ? AND deleted_at IS NULL", issuer).Take(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlCIProviderDao) ReferencingCITrustIDs(ctx context.Context, ciProviderID string) ([]string, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var ids []string
	if err := g2.Table("ci_trusts").
		Where("ci_provider_id = ? AND deleted_at IS NULL", ciProviderID).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
