package serviceaccountprovisioner

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

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

// caPoolReloader loads a PEM trust bundle from disk and reloads it whenever the
// backing file changes. cert-manager rotates the mounted CA in place, so caching
// by modification time lets a rotated issuing CA be trusted without a process
// restart while avoiding a disk read on every TLS handshake.
type caPoolReloader struct {
	caFile string

	mutex  sync.Mutex
	cached *x509.CertPool
	mtime  time.Time
}

func newCAPoolReloader(caFile string) *caPoolReloader {
	return &caPoolReloader{caFile: caFile}
}

// currentPool returns the most recent trust bundle, reloading from disk when the
// backing file has changed since the last load.
func (r *caPoolReloader) currentPool() (*x509.CertPool, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	info, err := os.Stat(r.caFile)
	if err != nil {
		return nil, fmt.Errorf("stat CA %q: %w", r.caFile, err)
	}
	if r.cached != nil && info.ModTime().Equal(r.mtime) {
		return r.cached, nil
	}

	pemBytes, err := os.ReadFile(r.caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA %q: %w", r.caFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("CA %q contains no certificates", r.caFile)
	}
	r.cached = pool
	r.mtime = info.ModTime()
	return pool, nil
}

type TransportConfig struct {
	Address                  string
	CertificateFile          string
	KeyFile                  string
	ClientCAFile             string
	ExpectedClientCommonName string
}

// ListenAndServe runs the internal provisioner until ctx is canceled. Mutual
// TLS is mandatory; there is no plaintext mode.
func ListenAndServe(ctx context.Context, config TransportConfig, handler *Server) error {
	serverCredentials, err := loadServerCredentials(config)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		return fmt.Errorf("listen for service-account provisioner: %w", err)
	}
	grpcServer := grpc.NewServer(grpc.Creds(serverCredentials))
	pb.RegisterOpenShellGatewayServiceAccountProvisionerServiceServer(grpcServer, handler)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		<-ctx.Done()
		healthServer.Shutdown()
		grpcServer.GracefulStop()
	}()
	if err := grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("serve service-account provisioner: %w", err)
	}
	return nil
}

func loadServerCredentials(config TransportConfig) (credentials.TransportCredentials, error) {
	if config.Address == "" || config.CertificateFile == "" || config.KeyFile == "" ||
		config.ClientCAFile == "" || config.ExpectedClientCommonName == "" {
		return nil, errors.New("complete service-account provisioner mTLS configuration is required")
	}
	reloader := newCertificateReloader(config.CertificateFile, config.KeyFile)
	// Validate the key pair eagerly so misconfiguration fails at startup.
	if _, err := reloader.currentCertificate(); err != nil {
		return nil, fmt.Errorf("load service-account provisioner server certificate: %w", err)
	}
	caReloader := newCAPoolReloader(config.ClientCAFile)
	// Validate the client trust bundle eagerly so misconfiguration fails at startup.
	if _, err := caReloader.currentPool(); err != nil {
		return nil, fmt.Errorf("load service-account provisioner client CA: %w", err)
	}
	base := &tls.Config{
		MinVersion: tls.VersionTLS13,
		// GetCertificate reloads the rotated key pair per handshake; do not also
		// set a static Certificates slice, which would be preferred over this.
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return reloader.currentCertificate()
		},
		ClientAuth: tls.RequireAndVerifyClientCert,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 {
				return errors.New("service-account provisioner client certificate was not verified")
			}
			if state.VerifiedChains[0][0].Subject.CommonName != config.ExpectedClientCommonName {
				return errors.New("service-account provisioner client identity is not allowed")
			}
			return nil
		},
	}
	// GetConfigForClient runs per handshake, so reloading ClientCAs here lets a
	// rotated client-issuing CA be trusted without a process restart. The base
	// config carries the reloading leaf certificate and identity check.
	base.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
		clientRoots, err := caReloader.currentPool()
		if err != nil {
			return nil, fmt.Errorf("load service-account provisioner client CA: %w", err)
		}
		handshakeConfig := base.Clone()
		handshakeConfig.GetConfigForClient = nil
		handshakeConfig.ClientCAs = clientRoots
		return handshakeConfig, nil
	}
	return credentials.NewTLS(base), nil
}
