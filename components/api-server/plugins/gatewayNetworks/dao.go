package gatewayNetworks

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type GatewayNetworkDao interface {
	Get(ctx context.Context, id string) (*GatewayNetwork, error)
	Create(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, error)
	Replace(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (GatewayNetworkList, error)
	All(ctx context.Context) (GatewayNetworkList, error)
}

var _ GatewayNetworkDao = &sqlGatewayNetworkDao{}

type sqlGatewayNetworkDao struct {
	sessionFactory *db.SessionFactory
}

func NewGatewayNetworkDao(sessionFactory *db.SessionFactory) GatewayNetworkDao {
	return &sqlGatewayNetworkDao{sessionFactory: sessionFactory}
}

func (d *sqlGatewayNetworkDao) Get(ctx context.Context, id string) (*GatewayNetwork, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var gatewayNetwork GatewayNetwork
	if err := g2.Take(&gatewayNetwork, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &gatewayNetwork, nil
}

func (d *sqlGatewayNetworkDao) Create(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(gatewayNetwork).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return gatewayNetwork, nil
}

func (d *sqlGatewayNetworkDao) Replace(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(gatewayNetwork).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return gatewayNetwork, nil
}

func (d *sqlGatewayNetworkDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&GatewayNetwork{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlGatewayNetworkDao) FindByIDs(ctx context.Context, ids []string) (GatewayNetworkList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	gatewayNetworks := GatewayNetworkList{}
	if err := g2.Where("id in (?)", ids).Find(&gatewayNetworks).Error; err != nil {
		return nil, err
	}
	return gatewayNetworks, nil
}

func (d *sqlGatewayNetworkDao) All(ctx context.Context) (GatewayNetworkList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	gatewayNetworks := GatewayNetworkList{}
	if err := g2.Find(&gatewayNetworks).Error; err != nil {
		return nil, err
	}
	return gatewayNetworks, nil
}
