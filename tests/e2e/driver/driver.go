// Package driver defines the E2EInfraDriver interface that isolates all
// infrastructure-specific mechanics from the e2e suite, plus the registry and
// KUBECONFIG-context auto-detection that resolve a driver by name. The suite, the
// performance harness, and the matrix runner depend only on this interface for
// infra-specific operations; they reach the Kubernetes API through the shared
// clients in package harness and never shell out to kubectl or oc. This package
// owns both the kind and openshift implementations (kind.go, openshift.go).
package driver

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// GatewayRef identifies a gateway for the discovery and routing methods.
type GatewayRef struct {
	Name      string
	Namespace string
	ID        string
}

// Credentials is an OIDC user identity for a token grant. For the password grant
// both fields are used; for the client-credentials grant they are ignored and the
// configured service-account client is used instead (see E2E_OIDC_GRANT).
type Credentials struct {
	Username string
	Password string
}

// Token is an acquired OIDC access token. The suite carries AccessToken as the
// bearer credential on every API call through apiclient.
type Token struct {
	AccessToken string
}

// E2EInfraDriver isolates infrastructure-specific mechanics behind a fixed set of
// methods. Each target (kind, openshift) implements it; the suite calls only
// these methods for infra-specific operations. Every method accepts a context and
// returns an error rather than panicking, so a method that cannot meet its
// contract surfaces as a failed testify assertion with context.
type E2EInfraDriver interface {
	// Name reports the registered driver name ("kind", "openshift").
	Name() string
	// PlatformNamespace is where the api-server, controller, and Keycloak run:
	// hypershell-system / E2E_NAMESPACE on kind, OPENSHIFT_NAMESPACE on openshift.
	PlatformNamespace() string

	// Discovery.
	DiscoverAPIHost(ctx context.Context) (string, error)
	DiscoverConsoleHost(ctx context.Context) (string, error)
	DiscoverGatewayEndpoint(ctx context.Context, gw GatewayRef) (string, error)
	ClusterDomain(ctx context.Context) (string, error)
	WaitForGatewayRoute(ctx context.Context, gw GatewayRef) error

	// OIDC + authenticated API access. Grant selected by E2E_OIDC_GRANT.
	AcquireOIDCToken(ctx context.Context, user Credentials) (Token, error)
	AcquireGatewayTokenWithRole(ctx context.Context, user Credentials, clientID, role string) (Token, error)
	// ValidateGatewayDeviceAuthorization exercises the public OAuth device flow
	// with PKCE for a reconciled per-gateway client.
	ValidateGatewayDeviceAuthorization(ctx context.Context, clientID string) error
	// AcquireClientCredentialsToken mints a token via the client-credentials grant
	// for a confidential OIDC client, for example the control-plane registrar
	// (E2E_REGISTRAR_CLIENT_ID) used by the ManagedCluster /registration checks.
	AcquireClientCredentialsToken(ctx context.Context, clientID, clientSecret string) (Token, error)
	APIClient(ctx context.Context, tok Token) (*apiclient.Client, error)

	// Keycloak role helpers (idempotent).
	AssignGatewayClientRole(ctx context.Context, user, clientID, role string) error
	AssignRealmRole(ctx context.Context, user, role string) error
	CreateTestUser(ctx context.Context, username, password string) error
	DeleteTestUser(ctx context.Context, username string) error

	// Namespace-GC timing override for the orphan-reaper assertion (long mode only);
	// patches the controller Deployment through the shared kube client.
	ConfigureNamespaceGCTiming(ctx context.Context, interval, grace time.Duration) error
	RestoreNamespaceGCTiming(ctx context.Context) error

	// Cleanup hook, invoked unconditionally from TearDownSuite. No-op on kind and openshift.
	DeSeedTestUsers(ctx context.Context) error
}

// Factory builds a driver from the shared Kubernetes clients.
type Factory func(clients *harness.Clients) (E2EInfraDriver, error)

var registry = map[string]Factory{}

// Register adds a driver factory under name. Called from each driver's init().
// Registering the same name twice panics, which is a programming error surfaced at
// package-load time, not a runtime test failure.
func Register(name string, f Factory) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("driver %q already registered", name))
	}
	registry[name] = f
}

// Names returns the registered driver names, sorted, for error messages.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Resolve selects and builds a driver. When override is non-empty it names the
// driver directly, bypassing auto-detection; an unknown name is an error listing
// the registered names. When override is empty the driver is auto-detected from
// the current KUBECONFIG context: openshift when the cluster serves
// route.openshift.io, kind otherwise.
func Resolve(ctx context.Context, clients *harness.Clients, override string) (E2EInfraDriver, error) {
	name := override
	if name == "" {
		detected, err := autoDetect(clients)
		if err != nil {
			return nil, err
		}
		name = detected
	}

	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown driver %q; registered drivers: %v", name, Names())
	}
	return factory(clients)
}

// autoDetect returns "openshift" when the cluster serves the route.openshift.io
// API group and "kind" otherwise.
func autoDetect(clients *harness.Clients) (string, error) {
	isOpenShift, err := clients.ServesAPIGroup("route.openshift.io")
	if err != nil {
		return "", fmt.Errorf("auto-detect driver from KUBECONFIG context: %w", err)
	}
	if isOpenShift {
		return "openshift", nil
	}
	return "kind", nil
}
