package defaultworkspace

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	openshellv1 "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/openshell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type GRPCOpenShellClient struct {
	kube kubernetes.Interface
}

func NewGRPCOpenShellClient(kube kubernetes.Interface) *GRPCOpenShellClient {
	return &GRPCOpenShellClient{kube: kube}
}

func (c *GRPCOpenShellClient) AddMember(ctx context.Context, endpoint, namespace, accessToken, subject, role string) error {
	target, transportCredentials, err := c.grpcTarget(ctx, endpoint, namespace)
	if err != nil {
		return err
	}
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()

	workspaceRole := openshellv1.WorkspaceRole_WORKSPACE_ROLE_USER
	if role == "admin" {
		workspaceRole = openshellv1.WorkspaceRole_WORKSPACE_ROLE_ADMIN
	}
	callCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+accessToken)
	_, err = openshellv1.NewOpenShellClient(connection).AddWorkspaceMember(callCtx, &openshellv1.AddWorkspaceMemberRequest{
		WorkspaceScope:   &openshellv1.WorkspaceSelector{Workspace: "default"},
		PrincipalSubject: subject, Role: workspaceRole,
		RequestId: bootstrapRequestID(namespace, subject, role),
	})
	if status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

func bootstrapRequestID(namespace, subject, role string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("hypershell:default-workspace:"+namespace+":"+subject+":"+role)).String()
}

func (c *GRPCOpenShellClient) grpcTarget(ctx context.Context, endpoint, namespace string) (string, credentials.TransportCredentials, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" {
		return "", nil, errInvalidGatewayEndpoint()
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		clientCertificate, rootCAs, err := c.clientTLS(ctx, namespace)
		if err != nil {
			return "", nil, err
		}
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12, RootCAs: rootCAs, Certificates: []tls.Certificate{clientCertificate},
		}
		return "passthrough:///" + parsed.Host, credentials.NewTLS(tlsConfig), nil
	case "http":
		return "passthrough:///" + parsed.Host, insecure.NewCredentials(), nil
	default:
		return "", nil, errInvalidGatewayEndpoint()
	}
}

func (c *GRPCOpenShellClient) clientTLS(ctx context.Context, namespace string) (tls.Certificate, *x509.CertPool, error) {
	secret, err := c.kube.CoreV1().Secrets(namespace).Get(ctx, clientTLSSecretName, metav1.GetOptions{})
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("read gateway client TLS secret: %w", err)
	}
	clientCertificate, err := tls.X509KeyPair(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey])
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("load gateway client TLS certificate: %w", err)
	}
	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM(secret.Data[corev1.ServiceAccountRootCAKey]) {
		return tls.Certificate{}, nil, fmt.Errorf("gateway client TLS secret %s/%s contains no valid CA certificates", namespace, clientTLSSecretName)
	}
	return clientCertificate, rootCAs, nil
}
