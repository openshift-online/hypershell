package gatewayReleases_test

import (
	"context"
	"fmt"

	"github.com/openshift-online/hypershell/components/api-server/plugins/gatewayReleases"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func newGatewayRelease(id string) (*gatewayReleases.GatewayRelease, error) {
	gatewayReleaseService := gatewayReleases.Service(&environments.Environment().Services)

	gatewayRelease := &gatewayReleases.GatewayRelease{
		Name:            "test-name",
		Image:           "test-image",
		RolloutStrategy: stringPtr("test-rollout_strategy"),
		CanaryPercent:   intPtr(42),
		CanaryDuration:  stringPtr("test-canary_duration"),
		Status:          stringPtr("test-status"),
	}

	sub, err := gatewayReleaseService.Create(context.Background(), gatewayRelease)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func newGatewayReleaseList(namePrefix string, count int) ([]*gatewayReleases.GatewayRelease, error) {
	var items []*gatewayReleases.GatewayRelease
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("%s_%d", namePrefix, i)
		c, err := newGatewayRelease(name)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, nil
}
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }
