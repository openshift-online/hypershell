package managedClusters

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/pkg/errors"
)

var _ ManagedClusterDao = &managedClusterDaoMock{}

type managedClusterDaoMock struct {
	managedClusters ManagedClusterList
}

func NewMockManagedClusterDao() *managedClusterDaoMock {
	return &managedClusterDaoMock{}
}

func (d *managedClusterDaoMock) Get(ctx context.Context, id string) (*ManagedCluster, error) {
	for _, managedCluster := range d.managedClusters {
		if managedCluster.ID == id {
			return managedCluster, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *managedClusterDaoMock) Create(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, error) {
	d.managedClusters = append(d.managedClusters, managedCluster)
	return managedCluster, nil
}

func (d *managedClusterDaoMock) Replace(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, error) {
	for i, mc := range d.managedClusters {
		if mc.ID == managedCluster.ID {
			d.managedClusters[i] = managedCluster
			return managedCluster, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *managedClusterDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("ManagedCluster").AsError()
}

func (d *managedClusterDaoMock) FindByIDs(ctx context.Context, ids []string) (ManagedClusterList, error) {
	return nil, errors.NotImplemented("ManagedCluster").AsError()
}

func (d *managedClusterDaoMock) All(ctx context.Context) (ManagedClusterList, error) {
	return d.managedClusters, nil
}

func (d *managedClusterDaoMock) FindByOIDCSubject(ctx context.Context, subject string) (*ManagedCluster, error) {
	for _, mc := range d.managedClusters {
		if mc.OIDCSubject == subject {
			return mc, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
