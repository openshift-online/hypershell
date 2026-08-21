package serviceaccountprovisioner

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

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
	certificate, err := tls.LoadX509KeyPair(config.CertificateFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load service-account provisioner server certificate: %w", err)
	}
	caPEM, err := os.ReadFile(config.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("read service-account provisioner client CA: %w", err)
	}
	clientRoots := x509.NewCertPool()
	if !clientRoots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("service-account provisioner client CA contains no certificates")
	}
	return credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    clientRoots,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 {
				return errors.New("service-account provisioner client certificate was not verified")
			}
			if state.VerifiedChains[0][0].Subject.CommonName != config.ExpectedClientCommonName {
				return errors.New("service-account provisioner client identity is not allowed")
			}
			return nil
		},
	}), nil
}
