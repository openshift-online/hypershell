package defaultworkspace

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"strings"

	datamodelv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/datamodelv1"
	openshellv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/openshellv1"
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
		WorkspaceScope:   &datamodelv1.WorkspaceSelector{Selection: &datamodelv1.WorkspaceSelector_Workspace{Workspace: "default"}},
		PrincipalSubject: subject, Role: workspaceRole,
		RequestId: bootstrapRequestID(namespace, subject, role),
	})
	if status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

func (c *GRPCOpenShellClient) HasWorkspaceMember(
	ctx context.Context, endpoint, namespace, accessToken, workspace, subject string,
) (bool, error) {
	target, transportCredentials, err := c.grpcTarget(ctx, endpoint, namespace)
	if err != nil {
		return false, err
	}
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return false, err
	}
	defer func() { _ = connection.Close() }()

	client := openshellv1.NewOpenShellClient(connection)
	callCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+accessToken)
	workspaceScope := &datamodelv1.WorkspaceSelector{
		Selection: &datamodelv1.WorkspaceSelector_Workspace{Workspace: workspace},
	}
	for pageToken := ""; ; {
		response, err := client.ListWorkspaceMembers(callCtx, &openshellv1.ListWorkspaceMembersRequest{
			WorkspaceScope: workspaceScope, PageSize: 1000, PageToken: pageToken,
		})
		if err != nil {
			return false, fmt.Errorf("list members of workspace %q: %w", workspace, err)
		}
		for _, member := range response.GetMembers() {
			if member.GetPrincipalSubject() == subject {
				return true, nil
			}
		}
		pageToken = response.GetNextPageToken()
		if pageToken == "" {
			return false, nil
		}
	}
}

func (c *GRPCOpenShellClient) ListWorkspaceVisibility(ctx context.Context, endpoint, namespace, accessToken string) (WorkspaceVisibility, error) {
	target, transportCredentials, err := c.grpcTarget(ctx, endpoint, namespace)
	if err != nil {
		return WorkspaceVisibility{}, err
	}
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return WorkspaceVisibility{}, err
	}
	defer func() { _ = connection.Close() }()
	client := openshellv1.NewOpenShellClient(connection)
	callCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+accessToken)

	workspaces := make([]string, 0)
	for pageToken := ""; ; {
		response, err := client.ListWorkspaces(callCtx, &openshellv1.ListWorkspacesRequest{
			PageSize: 1000, PageToken: pageToken,
		})
		if err != nil {
			return WorkspaceVisibility{}, fmt.Errorf("list workspaces: %w", err)
		}
		for _, workspace := range response.GetWorkspaces() {
			if name := workspace.GetMetadata().GetName(); name != "" {
				workspaces = append(workspaces, name)
			}
		}
		pageToken = response.GetNextPageToken()
		if pageToken == "" {
			break
		}
	}

	visibility := WorkspaceVisibility{Workspaces: workspaces, Memberships: make([]WorkspaceMembership, 0)}
	for _, workspace := range workspaces {
		for pageToken := ""; ; {
			response, err := client.ListWorkspaceMembers(callCtx, &openshellv1.ListWorkspaceMembersRequest{
				WorkspaceScope: &datamodelv1.WorkspaceSelector{Selection: &datamodelv1.WorkspaceSelector_Workspace{Workspace: workspace}},
				PageSize:       1000, PageToken: pageToken,
			})
			if err != nil {
				return WorkspaceVisibility{}, fmt.Errorf("list members of workspace %q: %w", workspace, err)
			}
			for _, member := range response.GetMembers() {
				if subject := member.GetPrincipalSubject(); subject != "" {
					visibility.Memberships = append(visibility.Memberships, WorkspaceMembership{Workspace: workspace, Subject: subject})
				}
			}
			pageToken = response.GetNextPageToken()
			if pageToken == "" {
				break
			}
		}
	}
	return visibility, nil
}

func bootstrapRequestID(namespace, subject, role string) string {
	digest := sha256.Sum256([]byte("hypershell:default-workspace:" + namespace + ":" + subject + ":" + role))
	id := digest[:16]
	id[6] = (id[6] & 0x0f) | 0x50
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		id[0:4], id[4:6], id[6:8], id[8:10], id[10:16],
	)
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
