package rbac_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

type bearerToken struct{ token string }

func (b *bearerToken) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}

func (b *bearerToken) RequireTransportSecurity() bool { return false }

func dialGRPC(t *testing.T, h *test.Helper, token string) *grpc.ClientConn {
	t.Helper()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(&bearerToken{token: token}))
	}
	conn, err := grpc.NewClient(h.GRPCAddress(), opts...)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { Expect(conn.Close()).To(Succeed()) })
	return conn
}

func strPtr(s string) *string { return &s }

// registerTestClusterWithSubject registers a ManagedCluster as a control plane
// would (unique subject and name) and returns (cluster id, subject).
func registerTestClusterWithSubject(t *testing.T) (string, string) {
	t.Helper()
	suffix := strings.ToLower(api.NewID())
	subject := "cp-" + suffix
	svc := managedClusters.Service(&environments.Environment().Services)
	cluster, _, svcErr := svc.Register(context.Background(), fmt.Sprintf("mc-%s", suffix), "", subject)
	if svcErr != nil {
		t.Fatalf("register test managed cluster: %v", svcErr)
	}
	return cluster.ID, subject
}

func registerTestCluster(t *testing.T) string {
	t.Helper()
	id, _ := registerTestClusterWithSubject(t)
	return id
}

// Watch Stream Caller Binding (managed-cluster-registration.spec.md): a caller
// whose JWT subject is a registered ManagedCluster may watch and list only its
// own cluster. Own id is accepted and delivers only that cluster's gateways; a
// foreign id is PERMISSION_DENIED; a missing id is INVALID_ARGUMENT.
func TestGRPCClusterCallerBinding(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	ownID, subject := registerTestClusterWithSubject(t)
	foreignID := registerTestCluster(t)

	cpAccount := h.NewAccount("service-account-cp-"+subject, "cp", "")
	cpToken := h.CreateJWTStringWithClaims(cpAccount, test.ControlPlaneClaims(subject))
	cp := pb.NewGatewayServiceClient(dialGRPC(t, h, cpToken))

	userCtx := h.NewAuthenticatedContext(h.NewRandAccount())

	// ListGateways
	_, err := cp.ListGateways(context.Background(), &pb.ListGatewaysRequest{Page: 1, Size: 10, ClusterId: strPtr(ownID)})
	Expect(err).NotTo(HaveOccurred(), "own cluster list must be accepted")
	_, err = cp.ListGateways(context.Background(), &pb.ListGatewaysRequest{Page: 1, Size: 10, ClusterId: strPtr(foreignID)})
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied), "foreign cluster list: %v", err)
	_, err = cp.ListGateways(context.Background(), &pb.ListGatewaysRequest{Page: 1, Size: 10})
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "unfiltered list: %v", err)

	// WatchGateways: foreign and missing ids are rejected before the handler
	// subscribes, so the first Recv returns the status.
	for _, tc := range []struct {
		clusterID *string
		want      codes.Code
	}{
		{clusterID: strPtr(foreignID), want: codes.PermissionDenied},
		{clusterID: nil, want: codes.InvalidArgument},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		stream, err := cp.WatchGateways(ctx, &pb.WatchGatewaysRequest{ClusterId: tc.clusterID})
		if err == nil {
			_, err = stream.Recv()
		}
		cancel()
		Expect(status.Code(err)).To(Equal(tc.want), "watch with cluster_id %v: %v", tc.clusterID, err)
	}

	// Own id: accepted, and only this cluster's gateways are delivered.
	watchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream, err := cp.WatchGateways(watchCtx, &pb.WatchGatewaysRequest{ClusterId: strPtr(ownID)})
	Expect(err).NotTo(HaveOccurred())
	_, err = stream.Header()
	Expect(err).NotTo(HaveOccurred(), "own cluster watch must be accepted")

	foreignGw, _, err := client.DefaultAPI.CreateGateway(userCtx).GatewayCreateRequest(openapi.GatewayCreateRequest{
		Name: "binding-foreign", ClusterId: foreignID,
	}).Execute()
	Expect(err).NotTo(HaveOccurred())
	ownGw, _, err := client.DefaultAPI.CreateGateway(userCtx).GatewayCreateRequest(openapi.GatewayCreateRequest{
		Name: "binding-own", ClusterId: ownID,
	}).Execute()
	Expect(err).NotTo(HaveOccurred())

	for {
		evt, recvErr := stream.Recv()
		Expect(recvErr).NotTo(HaveOccurred(), "stream closed before the own-cluster event")
		Expect(evt.ResourceId).NotTo(Equal(*foreignGw.Id), "a foreign cluster's gateway was delivered")
		if evt.GetGateway() != nil {
			Expect(evt.GetGateway().GetClusterId()).To(Equal(ownID))
		}
		if evt.ResourceId == *ownGw.Id {
			break
		}
	}

	// Callers that are not a registered cluster (users, hsctl) are unaffected.
	user := pb.NewGatewayServiceClient(dialGRPC(t, h, h.CreateJWTString(h.NewRandAccount())))
	_, err = user.ListGateways(context.Background(), &pb.ListGatewaysRequest{Page: 1, Size: 10})
	Expect(err).NotTo(HaveOccurred(), "a user's unfiltered list must not be bound")
}

// The same caller binding covers the RoleBinding watch and list, which are
// scoped by the binding's gateway's cluster_id.
func TestGRPCClusterCallerBindingRoleBindings(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	ownID, subject := registerTestClusterWithSubject(t)
	foreignID := registerTestCluster(t)

	cpAccount := h.NewAccount("service-account-cp-"+subject, "cp", "")
	cpToken := h.CreateJWTStringWithClaims(cpAccount, test.ControlPlaneClaims(subject))
	cp := pb.NewRoleBindingServiceClient(dialGRPC(t, h, cpToken))
	userID := "rb-binding-user"

	_, err := cp.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: strPtr(userID), ClusterId: strPtr(ownID)})
	Expect(err).NotTo(HaveOccurred(), "own cluster list must be accepted")
	_, err = cp.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: strPtr(userID), ClusterId: strPtr(foreignID)})
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied), "foreign cluster list: %v", err)
	_, err = cp.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: strPtr(userID)})
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "unfiltered list: %v", err)

	for _, tc := range []struct {
		clusterID *string
		want      codes.Code
	}{
		{clusterID: strPtr(foreignID), want: codes.PermissionDenied},
		{clusterID: nil, want: codes.InvalidArgument},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		stream, err := cp.WatchRoleBindings(ctx, &pb.WatchRoleBindingsRequest{ClusterId: tc.clusterID})
		if err == nil {
			_, err = stream.Recv()
		}
		cancel()
		Expect(status.Code(err)).To(Equal(tc.want), "watch with cluster_id %v: %v", tc.clusterID, err)
	}

	watchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream, err := cp.WatchRoleBindings(watchCtx, &pb.WatchRoleBindingsRequest{ClusterId: strPtr(ownID)})
	Expect(err).NotTo(HaveOccurred())
	_, err = stream.Header()
	Expect(err).NotTo(HaveOccurred(), "own cluster watch must be accepted")

	// Callers that are not a registered cluster are unaffected.
	user := pb.NewRoleBindingServiceClient(dialGRPC(t, h, h.CreateJWTString(h.NewRandAccount())))
	_, err = user.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: strPtr(userID)})
	Expect(err).NotTo(HaveOccurred(), "a user's unfiltered list must not be bound")
}

// Anonymous watch rejected: every Watch* RPC requires a JWT; none is exempted by
// --auth-bypass-methods.
func TestGRPCWatchRPCsRejectAnonymous(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	conn := dialGRPC(t, h, "")

	recvFirst := func(open func(ctx context.Context) (func() error, error)) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		recv, err := open(ctx)
		if err != nil {
			return err
		}
		return recv()
	}

	cases := map[string]func(ctx context.Context) (func() error, error){
		"WatchGateways": func(ctx context.Context) (func() error, error) {
			s, err := pb.NewGatewayServiceClient(conn).WatchGateways(ctx, &pb.WatchGatewaysRequest{})
			if err != nil {
				return nil, err
			}
			return func() error { _, e := s.Recv(); return e }, nil
		},
		"WatchGatewayReleases": func(ctx context.Context) (func() error, error) {
			s, err := pb.NewGatewayReleaseServiceClient(conn).WatchGatewayReleases(ctx, &pb.WatchGatewayReleasesRequest{})
			if err != nil {
				return nil, err
			}
			return func() error { _, e := s.Recv(); return e }, nil
		},
		"WatchManagedClusters": func(ctx context.Context) (func() error, error) {
			s, err := pb.NewManagedClusterServiceClient(conn).WatchManagedClusters(ctx, &pb.WatchManagedClustersRequest{})
			if err != nil {
				return nil, err
			}
			return func() error { _, e := s.Recv(); return e }, nil
		},
		"WatchGatewayNetworks": func(ctx context.Context) (func() error, error) {
			s, err := pb.NewGatewayNetworkServiceClient(conn).WatchGatewayNetworks(ctx, &pb.WatchGatewayNetworksRequest{})
			if err != nil {
				return nil, err
			}
			return func() error { _, e := s.Recv(); return e }, nil
		},
		"WatchRoleBindings": func(ctx context.Context) (func() error, error) {
			s, err := pb.NewRoleBindingServiceClient(conn).WatchRoleBindings(ctx, &pb.WatchRoleBindingsRequest{})
			if err != nil {
				return nil, err
			}
			return func() error { _, e := s.Recv(); return e }, nil
		},
	}
	for name, open := range cases {
		err := recvFirst(open)
		Expect(status.Code(err)).To(Equal(codes.Unauthenticated), "%s without a bearer token: %v", name, err)
	}
}
