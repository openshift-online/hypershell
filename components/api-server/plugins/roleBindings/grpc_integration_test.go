package roleBindings_test

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
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/server/grpcutil"
)

type roleBindingBearerToken struct {
	token string
}

func (b *roleBindingBearerToken) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}

func (b *roleBindingBearerToken) RequireTransportSecurity() bool {
	return false
}

func TestGRPCWatchRoleBindingsReplaysExistingBindingsAfterEachConnect(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	userService := users.Service(&environments.Environment().Services)
	userID, err := userService.UpsertByUsername(context.Background(), account.Username, nil, nil)
	Expect(err).NotTo(HaveOccurred())

	gatewayID := "gateway-before-role-binding-watch"
	rbService := roleBindings.Service(&environments.Environment().Services)
	Expect(rbService.CreateGatewayOwnerBinding(context.Background(), userID, gatewayID)).To(Succeed())

	bindings, svcErr := rbService.FindByUserID(context.Background(), userID)
	Expect(svcErr).NotTo(HaveOccurred())
	var bindingID string
	for _, binding := range bindings {
		if binding.GatewayID != nil && *binding.GatewayID == gatewayID {
			bindingID = binding.ID
			break
		}
	}
	Expect(bindingID).NotTo(BeEmpty())

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&roleBindingBearerToken{token: h.CreateJWTString(account)}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	for attempt := 0; attempt < 2; attempt++ {
		watchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		stream, watchErr := pb.NewRoleBindingServiceClient(conn).WatchRoleBindings(watchCtx, &pb.WatchRoleBindingsRequest{})
		Expect(watchErr).NotTo(HaveOccurred())
		_, headerErr := stream.Header()
		Expect(headerErr).NotTo(HaveOccurred())

		for {
			event, recvErr := stream.Recv()
			Expect(recvErr).NotTo(HaveOccurred())
			if event.ResourceId != bindingID {
				continue
			}

			Expect(event.Type).To(Equal(pb.EventType_EVENT_TYPE_UPDATED))
			Expect(event.RoleBinding).NotTo(BeNil())
			Expect(event.RoleBinding.RoleName).To(Equal("gateway:owner"))
			Expect(event.RoleBinding.Username).To(Equal(account.Username))
			Expect(event.RoleBinding.GetGatewayId()).To(Equal(gatewayID))
			break
		}
		cancel()
	}
}

// registerRoleBindingTestCluster registers a ManagedCluster the way a control
// plane does (unique name and OIDC subject) and returns its id. Gateways accept
// only a cluster_id that references a registered cluster.
func registerRoleBindingTestCluster(t *testing.T) string {
	t.Helper()
	suffix := strings.ToLower(api.NewID())
	svc := managedClusters.Service(&environments.Environment().Services)
	cluster, _, svcErr := svc.Register(context.Background(), fmt.Sprintf("mc-%s", suffix), "", "cp-"+suffix)
	if svcErr != nil {
		t.Fatalf("register test managed cluster: %v", svcErr)
	}
	return cluster.ID
}

// createGatewayWithOwnerBinding creates a gateway on clusterID through the REST
// API, grants userID gateway:owner on it, and returns the gateway id and that
// binding's id.
func createGatewayWithOwnerBinding(ctx context.Context, client *openapi.APIClient, userID, name, clusterID string) (string, string) {
	gw, _, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(openapi.GatewayCreateRequest{
		Name: name, ClusterId: clusterID,
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "create gateway %s", name)
	gatewayID := *gw.Id

	// The integration environment does not provision the caller into the
	// request context, so the create path does not bootstrap the owner binding;
	// grant it the way that path would.
	rbService := roleBindings.Service(&environments.Environment().Services)
	Expect(rbService.CreateGatewayOwnerBinding(context.Background(), userID, gatewayID)).To(Succeed())
	bindings, svcErr := rbService.FindByUserID(context.Background(), userID)
	Expect(svcErr).NotTo(HaveOccurred())
	bindingID := ""
	for _, binding := range bindings {
		if binding.GatewayID != nil && *binding.GatewayID == gatewayID {
			bindingID = binding.ID
			break
		}
	}
	Expect(bindingID).NotTo(BeEmpty(), "no owner binding for gateway %s", gatewayID)
	return gatewayID, bindingID
}

// globalBindingID ensures userID holds the global gateway:creator binding and
// returns its id.
func globalBindingID(userID string) string {
	rbService := roleBindings.Service(&environments.Environment().Services)
	Expect(rbService.SyncJWTRoles(context.Background(), userID, []string{roles.RoleGatewayCreator})).To(Succeed())
	bindings, svcErr := rbService.FindByUserID(context.Background(), userID)
	Expect(svcErr).NotTo(HaveOccurred())
	bindingID := ""
	for _, binding := range bindings {
		if binding.GatewayID == nil || *binding.GatewayID == "" {
			bindingID = binding.ID
			break
		}
	}
	Expect(bindingID).NotTo(BeEmpty(), "no global binding for user %s", userID)
	return bindingID
}

// Watch Stream Caller Binding (managed-cluster-registration.spec.md): a
// WatchRoleBindings stream opened with cluster_id X delivers, in the replay and
// live, only bindings whose gateway is assigned to X. Bindings for gateways on
// another cluster and global bindings are never delivered, and a binding delete
// that follows its gateway's soft delete still reaches X.
func TestGRPCWatchRoleBindingsScopedToCluster(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	userID, err := users.Service(&environments.Environment().Services).UpsertByUsername(context.Background(), account.Username, nil, nil)
	Expect(err).NotTo(HaveOccurred())

	clusterX := registerRoleBindingTestCluster(t)
	clusterY := registerRoleBindingTestCluster(t)

	// Present before the watch opens: delivered (or not) by the replay.
	gwX1, bindingX1 := createGatewayWithOwnerBinding(ctx, client, userID, "rb-scope-x1", clusterX)
	_, bindingY1 := createGatewayWithOwnerBinding(ctx, client, userID, "rb-scope-y1", clusterY)
	globalBefore := globalBindingID(userID)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&roleBindingBearerToken{token: h.CreateJWTString(account)}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	watchCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	stream, err := pb.NewRoleBindingServiceClient(conn).WatchRoleBindings(watchCtx, &pb.WatchRoleBindingsRequest{ClusterId: &clusterX})
	Expect(err).NotTo(HaveOccurred())
	_, err = stream.Header()
	Expect(err).NotTo(HaveOccurred())

	forbidden := map[string]string{bindingY1: "cluster Y binding (replay)", globalBefore: "global binding (replay)"}

	// recvUntil reads events until wantType for wantID arrives, failing on any
	// foreign or global binding along the way.
	recvUntil := func(wantID string, wantType pb.EventType) *pb.WatchRoleBindingsResponse {
		for {
			evt, recvErr := stream.Recv()
			Expect(recvErr).NotTo(HaveOccurred(), "stream closed before binding %s", wantID)
			what, bad := forbidden[evt.ResourceId]
			Expect(bad).To(BeFalse(), "cluster-scoped watch delivered a %s: %s", what, evt.ResourceId)
			Expect(evt.GetRoleBinding()).NotTo(BeNil())
			Expect(evt.GetRoleBinding().GetGatewayId()).NotTo(BeEmpty(), "global binding %s delivered", evt.ResourceId)
			if evt.ResourceId == wantID && evt.Type == wantType {
				return evt
			}
		}
	}

	// Replay: the pre-existing cluster X binding is delivered.
	evt := recvUntil(bindingX1, pb.EventType_EVENT_TYPE_UPDATED)
	Expect(evt.GetRoleBinding().GetGatewayId()).To(Equal(gwX1))

	// Live: a global binding (new user's default role) and a cluster Y binding
	// are created before the cluster X binding; only the X binding arrives.
	other := h.NewRandAccount()
	otherID, err := users.Service(&environments.Environment().Services).UpsertByUsername(context.Background(), other.Username, nil, nil)
	Expect(err).NotTo(HaveOccurred())
	forbidden[globalBindingID(otherID)] = "global binding (live)"
	_, bindingY2 := createGatewayWithOwnerBinding(ctx, client, userID, "rb-scope-y2", clusterY)
	forbidden[bindingY2] = "cluster Y binding (live)"
	gwX2, bindingX2 := createGatewayWithOwnerBinding(ctx, client, userID, "rb-scope-x2", clusterX)
	evt = recvUntil(bindingX2, pb.EventType_EVENT_TYPE_CREATED)
	Expect(evt.GetRoleBinding().GetGatewayId()).To(Equal(gwX2))

	// Delete after the gateway is soft-deleted: resolved through the unscoped
	// gateway lookup, so the delete still reaches cluster X.
	Expect(gateways.Service(&environments.Environment().Services).Delete(context.Background(), gwX1)).To(BeNil())
	Expect(roleBindings.Service(&environments.Environment().Services).Delete(context.Background(), bindingX1)).To(BeNil())
	evt = recvUntil(bindingX1, pb.EventType_EVENT_TYPE_DELETED)
	Expect(evt.GetRoleBinding().GetGatewayId()).To(Equal(gwX1))
}

// ListRoleBindings with cluster_id returns only the user's bindings whose
// gateway is assigned to that cluster; global bindings are excluded. Without
// cluster_id every binding is returned, and an over-length cluster_id is
// rejected.
func TestGRPCListRoleBindingsScopedToCluster(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	userID, err := users.Service(&environments.Environment().Services).UpsertByUsername(context.Background(), account.Username, nil, nil)
	Expect(err).NotTo(HaveOccurred())

	clusterX := registerRoleBindingTestCluster(t)
	clusterY := registerRoleBindingTestCluster(t)
	_, bindingX := createGatewayWithOwnerBinding(ctx, client, userID, "rb-list-x", clusterX)
	_, bindingY := createGatewayWithOwnerBinding(ctx, client, userID, "rb-list-y", clusterY)
	globalID := globalBindingID(userID)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&roleBindingBearerToken{token: h.CreateJWTString(account)}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})
	rbClient := pb.NewRoleBindingServiceClient(conn)

	ids := func(resp *pb.ListRoleBindingsResponse) []string {
		out := make([]string, 0, len(resp.GetItems()))
		for _, item := range resp.GetItems() {
			out = append(out, item.GetMetadata().GetId())
		}
		return out
	}

	scoped, err := rbClient.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: &userID, ClusterId: &clusterX})
	Expect(err).NotTo(HaveOccurred())
	Expect(ids(scoped)).To(ConsistOf(bindingX))

	scopedY, err := rbClient.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: &userID, ClusterId: &clusterY})
	Expect(err).NotTo(HaveOccurred())
	Expect(ids(scopedY)).To(ConsistOf(bindingY))

	unscoped, err := rbClient.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: &userID})
	Expect(err).NotTo(HaveOccurred())
	Expect(ids(unscoped)).To(ContainElements(bindingX, bindingY, globalID))

	// The filter is a comparison, not a query fragment: an unknown or
	// quote-bearing id matches nothing rather than broadening the result.
	unknown := "x' OR '1'='1"
	none, err := rbClient.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: &userID, ClusterId: &unknown})
	Expect(err).NotTo(HaveOccurred())
	Expect(none.GetItems()).To(BeEmpty())

	tooLong := strings.Repeat("c", grpcutil.MaxStringFieldLength+1)
	_, err = rbClient.ListRoleBindings(context.Background(), &pb.ListRoleBindingsRequest{UserId: &userID, ClusterId: &tooLong})
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "over-length cluster_id: %v", err)
}
