package gatewayReleases

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type GatewayReleaseDao interface {
	Get(ctx context.Context, id string) (*GatewayRelease, error)
	Create(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, error)
	Replace(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (GatewayReleaseList, error)
	All(ctx context.Context) (GatewayReleaseList, error)
}

var _ GatewayReleaseDao = &sqlGatewayReleaseDao{}

type sqlGatewayReleaseDao struct {
	sessionFactory *db.SessionFactory
}

func NewGatewayReleaseDao(sessionFactory *db.SessionFactory) GatewayReleaseDao {
	return &sqlGatewayReleaseDao{sessionFactory: sessionFactory}
}

func (d *sqlGatewayReleaseDao) Get(ctx context.Context, id string) (*GatewayRelease, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var gatewayRelease GatewayRelease
	if err := g2.Take(&gatewayRelease, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &gatewayRelease, nil
}

func (d *sqlGatewayReleaseDao) Create(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(gatewayRelease).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return gatewayRelease, nil
}

func (d *sqlGatewayReleaseDao) Replace(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(gatewayRelease).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return gatewayRelease, nil
}

func (d *sqlGatewayReleaseDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&GatewayRelease{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlGatewayReleaseDao) FindByIDs(ctx context.Context, ids []string) (GatewayReleaseList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	gatewayReleases := GatewayReleaseList{}
	if err := g2.Where("id in (?)", ids).Find(&gatewayReleases).Error; err != nil {
		return nil, err
	}
	return gatewayReleases, nil
}

func (d *sqlGatewayReleaseDao) All(ctx context.Context) (GatewayReleaseList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	gatewayReleases := GatewayReleaseList{}
	if err := g2.Find(&gatewayReleases).Error; err != nil {
		return nil, err
	}
	return gatewayReleases, nil
}
