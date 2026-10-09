package gatewayAccess

import (
	"context"
	"fmt"
	"time"

	provisionerpb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const directoryCallTimeout = 30 * time.Second

// grpcDirectoryResolver reads the control-plane Keycloak directory projection
// over the private in-cluster gRPC channel (GAM-09). It mirrors the
// service-account provisioner client's plaintext in-cluster dial.
type grpcDirectoryResolver struct {
	client provisionerpb.DirectoryServiceClient
}

func newGRPCDirectoryResolver(address string) (DirectoryResolver, error) {
	// grpc-go DNS does not apply kube-DNS search domains; callers pass a
	// cluster-local FQDN (see deploy/base/api-server.yaml).
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("create directory client: %w", err)
	}
	return &grpcDirectoryResolver{client: provisionerpb.NewDirectoryServiceClient(conn)}, nil
}

func (r *grpcDirectoryResolver) Search(ctx context.Context, query string, limit int) ([]DirectoryUser, error) {
	callCtx, cancel := context.WithTimeout(ctx, directoryCallTimeout)
	defer cancel()
	resp, err := r.client.SearchDirectory(callCtx, &provisionerpb.SearchDirectoryRequest{Query: query, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]DirectoryUser, 0, len(resp.GetUsers()))
	for _, u := range resp.GetUsers() {
		out = append(out, DirectoryUser{Username: u.GetUsername(), Name: u.GetName(), Email: u.GetEmail(), Subject: u.GetSubject()})
	}
	return out, nil
}

func (r *grpcDirectoryResolver) Resolve(ctx context.Context, username string) (DirectoryUser, bool, error) {
	callCtx, cancel := context.WithTimeout(ctx, directoryCallTimeout)
	defer cancel()
	resp, err := r.client.ResolveUser(callCtx, &provisionerpb.ResolveUserRequest{Username: username})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return DirectoryUser{}, false, nil
		}
		return DirectoryUser{}, false, err
	}
	if !resp.GetFound() || resp.GetUser() == nil {
		return DirectoryUser{}, false, nil
	}
	u := resp.GetUser()
	return DirectoryUser{Username: u.GetUsername(), Name: u.GetName(), Email: u.GetEmail(), Subject: u.GetSubject()}, true, nil
}
