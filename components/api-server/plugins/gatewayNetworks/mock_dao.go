package gatewayNetworks

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

var _ GatewayNetworkDao = &gatewayNetworkDaoMock{}

type gatewayNetworkDaoMock struct {
	gatewayNetworks GatewayNetworkList
}

func NewMockGatewayNetworkDao() *gatewayNetworkDaoMock {
	return &gatewayNetworkDaoMock{}
}

func (d *gatewayNetworkDaoMock) Get(ctx context.Context, id string) (*GatewayNetwork, error) {
	for _, gatewayNetwork := range d.gatewayNetworks {
		if gatewayNetwork.ID == id {
			return gatewayNetwork, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *gatewayNetworkDaoMock) Create(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, error) {
	d.gatewayNetworks = append(d.gatewayNetworks, gatewayNetwork)
	return gatewayNetwork, nil
}

func (d *gatewayNetworkDaoMock) Replace(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, error) {
	return nil, errors.NotImplemented("GatewayNetwork").AsError()
}

func (d *gatewayNetworkDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("GatewayNetwork").AsError()
}

func (d *gatewayNetworkDaoMock) FindByIDs(ctx context.Context, ids []string) (GatewayNetworkList, error) {
	return nil, errors.NotImplemented("GatewayNetwork").AsError()
}

func (d *gatewayNetworkDaoMock) All(ctx context.Context) (GatewayNetworkList, error) {
	return d.gatewayNetworks, nil
}
