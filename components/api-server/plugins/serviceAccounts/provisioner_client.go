package serviceAccounts

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

const defaultProvisionerCallTimeout = 60 * time.Second

// certificateReloader loads a TLS key pair from disk and reloads it whenever the
// underlying files change. cert-manager rotates the mounted Secret in place, so
// caching by modification time lets renewed certificates be picked up without a
// process restart while avoiding a disk read on every TLS handshake.
type certificateReloader struct {
	certFile string
	keyFile  string

	mutex     sync.Mutex
	cached    *tls.Certificate
	certMtime time.Time
	keyMtime  time.Time
}

func newCertificateReloader(certFile, keyFile string) *certificateReloader {
	return &certificateReloader{certFile: certFile, keyFile: keyFile}
}

// currentCertificate returns the most recent key pair, reloading from disk when
// either backing file has changed since the last load.
func (r *certificateReloader) currentCertificate() (*tls.Certificate, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	certInfo, err := os.Stat(r.certFile)
	if err != nil {
		return nil, fmt.Errorf("stat certificate %q: %w", r.certFile, err)
	}
	keyInfo, err := os.Stat(r.keyFile)
	if err != nil {
		return nil, fmt.Errorf("stat key %q: %w", r.keyFile, err)
	}

	if r.cached != nil && certInfo.ModTime().Equal(r.certMtime) && keyInfo.ModTime().Equal(r.keyMtime) {
		return r.cached, nil
	}

	certificate, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}
	r.cached = &certificate
	r.certMtime = certInfo.ModTime()
	r.keyMtime = keyInfo.ModTime()
	return r.cached, nil
}

type controlPlaneProvisioner struct {
	client pb.OpenShellGatewayServiceAccountProvisionerServiceClient
}

func newControlPlaneProvisionerFromEnvironment() (ServiceAccountProvisioner, error) {
	address := os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR")
	if address == "" {
		return nil, nil
	}
	transportCredentials, err := loadProvisionerClientCredentials(
		os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_TLS_CERT"),
		os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_TLS_KEY"),
		os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_TLS_CA"),
		os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_SERVER_NAME"),
	)
	if err != nil {
		return nil, err
	}
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return nil, fmt.Errorf("create control-plane provisioner client: %w", err)
	}
	return &controlPlaneProvisioner{client: pb.NewOpenShellGatewayServiceAccountProvisionerServiceClient(connection)}, nil
}

func loadProvisionerClientCredentials(certFile, keyFile, caFile, serverName string) (credentials.TransportCredentials, error) {
	if certFile == "" || keyFile == "" || caFile == "" {
		return nil, errors.New("control-plane provisioner mTLS files are required")
	}
	reloader := newCertificateReloader(certFile, keyFile)
	// Validate the key pair eagerly so misconfiguration fails at startup.
	if _, err := reloader.currentCertificate(); err != nil {
		return nil, fmt.Errorf("load control-plane provisioner client certificate: %w", err)
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read control-plane provisioner CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("control-plane provisioner CA contains no certificates")
	}
	return credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13,
		// GetClientCertificate reloads the rotated key pair per handshake; do not
		// also set a static Certificates slice, which would take precedence.
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return reloader.currentCertificate()
		},
		RootCAs:    roots,
		ServerName: serverName,
	}), nil
}

func (p *controlPlaneProvisioner) Configured() bool {
	return p != nil && p.client != nil
}

func (p *controlPlaneProvisioner) ProvisionServiceAccount(ctx context.Context, spec ProvisioningSpec) (*ProvisionedServiceAccount, error) {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	response, err := p.client.Provision(callCtx, &pb.ProvisionRequest{Spec: provisioningSpecToProto(spec)})
	if err != nil {
		return nil, mapProvisionerError(err)
	}
	return &ProvisionedServiceAccount{
		ClientUUID: response.GetClientUuid(), ClientID: response.GetClientId(),
		ClientSecret: response.GetClientSecret(), Subject: response.GetSubject(),
	}, nil
}

func (p *controlPlaneProvisioner) ReconcileServiceAccount(ctx context.Context, spec ProvisioningSpec, clientUUID, expectedSubject string, enabled bool) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.Reconcile(callCtx, &pb.ReconcileRequest{
		Spec: provisioningSpecToProto(spec), ClientUuid: clientUUID,
		ExpectedSubject: expectedSubject, Enabled: enabled,
	})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) DisableServiceAccount(ctx context.Context, clientUUID, gatewayID, serviceAccountID string) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.Disable(callCtx, &pb.DisableRequest{
		ClientUuid: clientUUID, GatewayId: gatewayID, ServiceAccountId: serviceAccountID,
	})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) DeleteServiceAccount(ctx context.Context, clientUUID, gatewayID, serviceAccountID string) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.Delete(callCtx, &pb.DeleteRequest{
		ClientUuid: clientUUID, GatewayId: gatewayID, ServiceAccountId: serviceAccountID,
	})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) DeleteManagedServiceAccount(ctx context.Context, gatewayID, serviceAccountID string) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.DeleteManaged(callCtx, &pb.DeleteManagedRequest{
		GatewayId: gatewayID, ServiceAccountId: serviceAccountID,
	})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) DeleteGatewayServiceAccounts(ctx context.Context, gatewayID string) error {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	_, err := p.client.DeleteGateway(callCtx, &pb.DeleteGatewayRequest{GatewayId: gatewayID})
	return mapProvisionerError(err)
}

func (p *controlPlaneProvisioner) ListManagedClients(ctx context.Context, gatewayID string) ([]ManagedClient, error) {
	callCtx, cancel := provisionerCallContext(ctx)
	defer cancel()
	response, err := p.client.ListManaged(callCtx, &pb.ListManagedRequest{GatewayId: gatewayID})
	if err != nil {
		return nil, mapProvisionerError(err)
	}
	clients := make([]ManagedClient, 0, len(response.GetClients()))
	for _, client := range response.GetClients() {
		clients = append(clients, ManagedClient{
			UUID: client.GetUuid(), ClientID: client.GetClientId(), GatewayID: client.GetGatewayId(),
			ServiceAccountID: client.GetServiceAccountId(),
		})
	}
	return clients, nil
}

func provisioningSpecToProto(spec ProvisioningSpec) *pb.ServiceAccountSpec {
	return &pb.ServiceAccountSpec{
		ClientId: spec.ClientID, DisplayName: spec.DisplayName,
		GatewayClientId: spec.GatewayClientID, GatewayId: spec.GatewayID,
		ServiceAccountId: spec.ServiceAccountID, CreatorUserId: spec.CreatorUserID,
		Role: spec.Role, ExpectedIssuer: spec.ExpectedIssuer,
		AccessTokenLifetimeSeconds: int32(spec.AccessTokenLifetimeSeconds),
	}
}

func provisionerCallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, defaultProvisionerCallTimeout)
}

func mapProvisionerError(err error) error {
	if err == nil {
		return nil
	}
	if status.Code(err) == codes.NotFound {
		return fmt.Errorf("%w: %v", ErrProvisionerNotFound, err)
	}
	return fmt.Errorf("control-plane provisioner request failed: %w", err)
}
