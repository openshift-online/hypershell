package managedClusters_test

import (
	"context"
	"fmt"

	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func newManagedCluster(id string) (*managedClusters.ManagedCluster, error) {
	managedClusterService := managedClusters.Service(&environments.Environment().Services)

	managedCluster := &managedClusters.ManagedCluster{
		Name:             "test-name",
		Provider:         "test-provider",
		Region:           stringPtr("test-region"),
		KubeconfigSecret: "test-kubeconfig_secret",
		Status:           stringPtr("test-status"),
		ApiServerUrl:     stringPtr("test-api_server_url"),
	}

	sub, err := managedClusterService.Create(context.Background(), managedCluster)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func newManagedClusterList(namePrefix string, count int) ([]*managedClusters.ManagedCluster, error) {
	var items []*managedClusters.ManagedCluster
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("%s_%d", namePrefix, i)
		c, err := newManagedCluster(name)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, nil
}
func stringPtr(s string) *string { return &s }
