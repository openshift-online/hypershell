package ciTrusts

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type CITrustDao interface {
	Get(ctx context.Context, id string) (*CITrust, error)
	Create(ctx context.Context, ciTrust *CITrust) (*CITrust, error)
	Replace(ctx context.Context, ciTrust *CITrust) (*CITrust, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (CITrustList, error)
	All(ctx context.Context) (CITrustList, error)
	CIProviderExists(ctx context.Context, ciProviderID string) (bool, error)
	GatewayExists(ctx context.Context, gatewayID string) (bool, error)
	// GatewayRepositoryID returns the repository_id ownership mark of the referenced
	// gateway (found=false when no live gateway has that id). It backs the imported
	// CITrust tenancy gate (SEC-RBAC-26): an import may author a CITrust only on a
	// gateway owned by the same reconciled Repository.
	GatewayRepositoryID(ctx context.Context, gatewayID string) (repositoryID *string, found bool, err error)
}

var _ CITrustDao = &sqlCITrustDao{}

type sqlCITrustDao struct {
	sessionFactory *db.SessionFactory
}

func NewCITrustDao(sessionFactory *db.SessionFactory) CITrustDao {
	return &sqlCITrustDao{sessionFactory: sessionFactory}
}

func (d *sqlCITrustDao) Get(ctx context.Context, id string) (*CITrust, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item CITrust
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlCITrustDao) Create(ctx context.Context, ciTrust *CITrust) (*CITrust, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(ciTrust).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return ciTrust, nil
}

func (d *sqlCITrustDao) Replace(ctx context.Context, ciTrust *CITrust) (*CITrust, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(ciTrust).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return ciTrust, nil
}

func (d *sqlCITrustDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&CITrust{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlCITrustDao) FindByIDs(ctx context.Context, ids []string) (CITrustList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := CITrustList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlCITrustDao) All(ctx context.Context) (CITrustList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := CITrustList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlCITrustDao) CIProviderExists(ctx context.Context, ciProviderID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Table("ci_providers").
		Where("id = ? AND deleted_at IS NULL", ciProviderID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *sqlCITrustDao) GatewayExists(ctx context.Context, gatewayID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Table("gateways").
		Where("id = ? AND deleted_at IS NULL", gatewayID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *sqlCITrustDao) GatewayRepositoryID(ctx context.Context, gatewayID string) (*string, bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var row struct {
		RepositoryID *string `gorm:"column:repository_id"`
	}
	// Take returns ErrRecordNotFound when no live gateway has that id; the caller
	// treats a missing gateway as "not owned" and refuses the import write.
	if err := g2.Table("gateways").
		Select("repository_id").
		Where("id = ? AND deleted_at IS NULL", gatewayID).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return row.RepositoryID, true, nil
}
