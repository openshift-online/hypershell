package e2e

import (
	"context"
	"time"

	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
)

// matrixInfraDriver keeps management API and identity operations on the hub,
// while Kubernetes, route, and controller operations use the selected managed
// cluster context.
type matrixInfraDriver struct {
	hub    driver.E2EInfraDriver
	target driver.E2EInfraDriver
}

func (d matrixInfraDriver) Name() string              { return d.target.Name() }
func (d matrixInfraDriver) PlatformNamespace() string { return d.target.PlatformNamespace() }
func (d matrixInfraDriver) DiscoverAPIHost(ctx context.Context) (string, error) {
	return d.hub.DiscoverAPIHost(ctx)
}
func (d matrixInfraDriver) DiscoverConsoleHost(ctx context.Context) (string, error) {
	return d.hub.DiscoverConsoleHost(ctx)
}
func (d matrixInfraDriver) DiscoverGatewayEndpoint(ctx context.Context, ref driver.GatewayRef) (string, error) {
	return d.target.DiscoverGatewayEndpoint(ctx, ref)
}
func (d matrixInfraDriver) ClusterDomain(ctx context.Context) (string, error) {
	return d.target.ClusterDomain(ctx)
}
func (d matrixInfraDriver) WaitForGatewayRoute(ctx context.Context, ref driver.GatewayRef) error {
	return d.target.WaitForGatewayRoute(ctx, ref)
}
func (d matrixInfraDriver) AcquireOIDCToken(ctx context.Context, credentials driver.Credentials) (driver.Token, error) {
	return d.hub.AcquireOIDCToken(ctx, credentials)
}
func (d matrixInfraDriver) AcquireGatewayTokenWithRole(ctx context.Context, credentials driver.Credentials, clientID, role string) (driver.Token, error) {
	return d.hub.AcquireGatewayTokenWithRole(ctx, credentials, clientID, role)
}
func (d matrixInfraDriver) ValidateGatewayDeviceAuthorization(ctx context.Context, clientID string) error {
	return d.hub.ValidateGatewayDeviceAuthorization(ctx, clientID)
}
func (d matrixInfraDriver) AcquireClientCredentialsToken(ctx context.Context, clientID, secret string) (driver.Token, error) {
	return d.hub.AcquireClientCredentialsToken(ctx, clientID, secret)
}
func (d matrixInfraDriver) APIClient(ctx context.Context, token driver.Token) (*apiclient.Client, error) {
	return d.hub.APIClient(ctx, token)
}
func (d matrixInfraDriver) AssignGatewayClientRole(ctx context.Context, user, clientID, role string) error {
	return d.hub.AssignGatewayClientRole(ctx, user, clientID, role)
}
func (d matrixInfraDriver) AssignRealmRole(ctx context.Context, user, role string) error {
	return d.hub.AssignRealmRole(ctx, user, role)
}
func (d matrixInfraDriver) CreateTestUser(ctx context.Context, username, password string) error {
	return d.hub.CreateTestUser(ctx, username, password)
}
func (d matrixInfraDriver) DeleteTestUser(ctx context.Context, username string) error {
	return d.hub.DeleteTestUser(ctx, username)
}
func (d matrixInfraDriver) ConfigureNamespaceGCTiming(ctx context.Context, interval, grace time.Duration) error {
	return d.target.ConfigureNamespaceGCTiming(ctx, interval, grace)
}
func (d matrixInfraDriver) RestoreNamespaceGCTiming(ctx context.Context) error {
	return d.target.RestoreNamespaceGCTiming(ctx)
}
func (d matrixInfraDriver) DeSeedTestUsers(ctx context.Context) error {
	return d.hub.DeSeedTestUsers(ctx)
}
