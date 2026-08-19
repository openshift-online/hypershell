package gateways

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/pkg/errors"
)

var _ GatewayDao = &gatewayDaoMock{}

type gatewayDaoMock struct {
	gateways GatewayList
}

func NewMockGatewayDao() *gatewayDaoMock {
	return &gatewayDaoMock{}
}

func (d *gatewayDaoMock) Get(ctx context.Context, id string) (*Gateway, error) {
	for _, gateway := range d.gateways {
		if gateway.ID == id {
			return gateway, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *gatewayDaoMock) GetUnscoped(ctx context.Context, id string) (*Gateway, error) {
	return d.Get(ctx, id)
}

func (d *gatewayDaoMock) Create(ctx context.Context, gateway *Gateway) (*Gateway, error) {
	d.gateways = append(d.gateways, gateway)
	return gateway, nil
}

func (d *gatewayDaoMock) Replace(ctx context.Context, gateway *Gateway) (*Gateway, error) {
	return nil, errors.NotImplemented("Gateway").AsError()
}

func (d *gatewayDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("Gateway").AsError()
}

func (d *gatewayDaoMock) FindByIDs(ctx context.Context, ids []string) (GatewayList, error) {
	return nil, errors.NotImplemented("Gateway").AsError()
}

func (d *gatewayDaoMock) All(ctx context.Context) (GatewayList, error) {
	return d.gateways, nil
}

func (d *gatewayDaoMock) CountByPhase(ctx context.Context) (map[string]int64, error) {
	counts := make(map[string]int64)
	for _, gw := range d.gateways {
		if gw.Phase != nil {
			counts[*gw.Phase]++
		}
	}
	return counts, nil
}
