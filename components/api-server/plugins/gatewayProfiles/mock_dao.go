package gatewayProfiles

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/pkg/errors"
)

var _ GatewayProfileDao = &gatewayProfileDaoMock{}

type gatewayProfileDaoMock struct {
	gatewayProfiles GatewayProfileList
}

func NewMockGatewayProfileDao() *gatewayProfileDaoMock {
	return &gatewayProfileDaoMock{}
}

func (d *gatewayProfileDaoMock) Get(ctx context.Context, id string) (*GatewayProfile, error) {
	for _, gatewayProfile := range d.gatewayProfiles {
		if gatewayProfile.ID == id {
			return gatewayProfile, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *gatewayProfileDaoMock) Create(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, error) {
	d.gatewayProfiles = append(d.gatewayProfiles, gatewayProfile)
	return gatewayProfile, nil
}

func (d *gatewayProfileDaoMock) Replace(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, error) {
	return nil, errors.NotImplemented("GatewayProfile").AsError()
}

func (d *gatewayProfileDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("GatewayProfile").AsError()
}

func (d *gatewayProfileDaoMock) FindByIDs(ctx context.Context, ids []string) (GatewayProfileList, error) {
	return nil, errors.NotImplemented("GatewayProfile").AsError()
}

func (d *gatewayProfileDaoMock) All(ctx context.Context) (GatewayProfileList, error) {
	return d.gatewayProfiles, nil
}

func (d *gatewayProfileDaoMock) ExistsByClusterProfileID(ctx context.Context, profileID string) (bool, error) {
	return false, nil
}

func (d *gatewayProfileDaoMock) ExistsByGatewayProfileID(ctx context.Context, profileID string) (bool, error) {
	return false, nil
}
