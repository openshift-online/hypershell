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

func (d *gatewayDaoMock) AdjustActiveSandboxCount(ctx context.Context, namespace string, delta int) (string, int, bool, error) {
	gw := d.findByNamespace(namespace)
	if gw == nil {
		return "", 0, false, nil
	}
	cur := derefCount(gw.ActiveSandboxCount)
	next := cur + delta
	if next < 0 {
		next = 0
	}
	if next == cur {
		return gw.ID, cur, false, nil
	}
	gw.ActiveSandboxCount = &next
	return gw.ID, next, true, nil
}

func (d *gatewayDaoMock) SetActiveSandboxCount(ctx context.Context, namespace string, count int) (string, int, bool, error) {
	gw := d.findByNamespace(namespace)
	if gw == nil {
		return "", 0, false, nil
	}
	if count < 0 {
		count = 0
	}
	cur := derefCount(gw.ActiveSandboxCount)
	if count == cur && gw.ActiveSandboxCount != nil {
		return gw.ID, cur, false, nil
	}
	gw.ActiveSandboxCount = &count
	return gw.ID, count, true, nil
}

func (d *gatewayDaoMock) findByNamespace(namespace string) *Gateway {
	for _, gateway := range d.gateways {
		if gateway.Namespace == namespace {
			return gateway
		}
	}
	return nil
}
