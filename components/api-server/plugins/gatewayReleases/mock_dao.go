package gatewayReleases

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

var _ GatewayReleaseDao = &gatewayReleaseDaoMock{}

type gatewayReleaseDaoMock struct {
	gatewayReleases GatewayReleaseList
}

func NewMockGatewayReleaseDao() *gatewayReleaseDaoMock {
	return &gatewayReleaseDaoMock{}
}

func (d *gatewayReleaseDaoMock) Get(ctx context.Context, id string) (*GatewayRelease, error) {
	for _, gatewayRelease := range d.gatewayReleases {
		if gatewayRelease.ID == id {
			return gatewayRelease, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *gatewayReleaseDaoMock) Create(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, error) {
	d.gatewayReleases = append(d.gatewayReleases, gatewayRelease)
	return gatewayRelease, nil
}

func (d *gatewayReleaseDaoMock) Replace(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, error) {
	return nil, errors.NotImplemented("GatewayRelease").AsError()
}

func (d *gatewayReleaseDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("GatewayRelease").AsError()
}

func (d *gatewayReleaseDaoMock) FindByIDs(ctx context.Context, ids []string) (GatewayReleaseList, error) {
	return nil, errors.NotImplemented("GatewayRelease").AsError()
}

func (d *gatewayReleaseDaoMock) All(ctx context.Context) (GatewayReleaseList, error) {
	return d.gatewayReleases, nil
}
