package gateways_test

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

type bearerToken struct {
	token string
}

func (b *bearerToken) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + b.token,
	}, nil
}

func (b *bearerToken) RequireTransportSecurity() bool {
	return false
}

func TestGRPCGatewayCRUD(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	createReq := &pb.CreateGatewayRequest{
		Name:        "TestName",
		ClusterId:   registerTestCluster(t),
		ExternalDns: func() *string { s := "TestExternalDns"; return &s }(),
		TlsMode:     func() *string { s := "TestTlsMode"; return &s }(),
		ServiceType: func() *string { s := "TestServiceType"; return &s }(),
		Status:      func() *string { s := "TestStatus"; return &s }(),
		Phase:       func() *string { s := "Provisioning"; return &s }(),
	}
	created, err := grpcClient.CreateGateway(ctx, createReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Gateway.Metadata.Id).NotTo(BeEmpty())
	Expect(created.Gateway.Namespace).To(MatchRegexp(`^openshell-[0-9a-f]{16}$`))

	gatewayID := created.Gateway.Metadata.Id
	gatewayNamespace := created.Gateway.Namespace

	getReq := &pb.GetGatewayRequest{Id: gatewayID}
	retrieved, err := grpcClient.GetGateway(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved.Gateway.Metadata.Id).To(Equal(gatewayID))

	versionResp, err := grpcClient.SetGatewayVersion(ctx, &pb.SetGatewayVersionRequest{
		Id:             gatewayID,
		GatewayVersion: "v0.0.109-rh9a8f8",
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(versionResp.GetGatewayVersion()).To(Equal("v0.0.109-rh9a8f8"))

	updateReq := &pb.UpdateGatewayRequest{
		Id:          gatewayID,
		Name:        func() *string { s := "UpdatedName"; return &s }(),
		ClusterId:   func() *string { s := registerTestCluster(t); return &s }(),
		ExternalDns: func() *string { s := "UpdatedExternalDns"; return &s }(),
		TlsMode:     func() *string { s := "UpdatedTlsMode"; return &s }(),
		ServiceType: func() *string { s := "UpdatedServiceType"; return &s }(),
		Status:      func() *string { s := "UpdatedStatus"; return &s }(),
		Phase:       func() *string { s := "Running"; return &s }(),
	}
	updated, err := grpcClient.UpdateGateway(ctx, updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.Gateway.Metadata.Id).To(Equal(gatewayID))
	Expect(updated.Gateway.Namespace).To(Equal(gatewayNamespace))

	retrieved, err = grpcClient.GetGateway(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	// A whole-row update must not overwrite the independently reconciled version.
	Expect(retrieved.Gateway.GetGatewayVersion()).To(Equal("v0.0.109-rh9a8f8"))

	listReq := &pb.ListGatewaysRequest{
		Page: 1,
		Size: 10,
	}
	listResp, err := grpcClient.ListGateways(ctx, listReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.Metadata.Total).To(BeNumerically(">=", 1))

	deleteReq := &pb.DeleteGatewayRequest{Id: gatewayID}
	_, err = grpcClient.DeleteGateway(ctx, deleteReq)
	Expect(err).NotTo(HaveOccurred())

	_, err = grpcClient.GetGateway(ctx, getReq)
	Expect(err).To(HaveOccurred())
}

func TestGRPCCapabilitySelectorPlacementSurvivesCapabilityReplacement(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)
	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { Expect(conn.Close()).To(Succeed()) })

	clusterService := managedClusters.Service(&environments.Environment().Services)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	selector := "runtime/test-" + suffix
	cluster, _, svcErr := clusterService.Register(ctx, "selector-"+suffix, []string{"network/public", "provider/aws", selector}, "cp-"+suffix)
	Expect(svcErr).To(BeNil())
	initialSeenAt := *cluster.LastSeenAt

	grpcClient := pb.NewGatewayServiceClient(conn)
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name: "selector-placement-" + suffix,
		Placement: &pb.GatewayPlacementIntent{Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
			CapabilitySelector: &pb.CapabilitySelectorPlacement{Selector: []string{selector}},
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Gateway.ClusterId).To(Equal(cluster.ID))

	time.Sleep(time.Millisecond)
	replaced, createdCluster, svcErr := clusterService.Register(ctx, cluster.Name, []string{"network/vpn", "provider/aws"}, cluster.OIDCSubject)
	Expect(svcErr).To(BeNil())
	Expect(createdCluster).To(BeFalse())
	Expect(replaced.Capabilities).To(Equal([]string{"network/vpn", "provider/aws"}))
	Expect(replaced.LastSeenAt.After(initialSeenAt)).To(BeTrue())

	beforeInvalid, svcErr := clusterService.Get(ctx, cluster.ID)
	Expect(svcErr).To(BeNil())
	beforeInvalidSeenAt := *beforeInvalid.LastSeenAt
	_, _, svcErr = clusterService.Register(ctx, cluster.Name, []string{"unknown/value"}, cluster.OIDCSubject)
	Expect(svcErr).NotTo(BeNil())
	afterInvalid, svcErr := clusterService.Get(ctx, cluster.ID)
	Expect(svcErr).To(BeNil())
	Expect(afterInvalid.Capabilities).To(Equal(replaced.Capabilities))
	Expect(*afterInvalid.LastSeenAt).To(Equal(beforeInvalidSeenAt))

	readBack, err := grpcClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: created.Gateway.Metadata.Id})
	Expect(err).NotTo(HaveOccurred())
	Expect(readBack.Gateway.ClusterId).To(Equal(cluster.ID))

	_, err = grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name: "empty-selector-" + suffix,
		Placement: &pb.GatewayPlacementIntent{Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
			CapabilitySelector: &pb.CapabilitySelectorPlacement{},
		}},
	})
	Expect(err).To(HaveOccurred())
}

func TestGRPCWatchGateways(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	const totalItems = 25

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	itemNames := make(map[string]bool, totalItems)
	for i := 0; i < totalItems; i++ {
		itemNames[fmt.Sprintf("watch_test_%d", i)] = true
	}

	var sourceErr error
	var sinkErr error
	var wg sync.WaitGroup
	wg.Add(2)

	sinkReady := make(chan struct{})
	registerTestCluster(t)

	go func() {
		defer wg.Done()
		<-sinkReady
		time.Sleep(100 * time.Millisecond)

		for name := range itemNames {
			gatewayInput := managedGatewayCreateRequest(name)
			_, resp, postErr := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
			if postErr != nil {
				sourceErr = fmt.Errorf("REST POST failed for %s: %v", name, postErr)
				return
			}
			if resp.StatusCode != 201 {
				sourceErr = fmt.Errorf("REST POST unexpected status %d for %s", resp.StatusCode, name)
				return
			}
		}
	}()

	go func() {
		defer wg.Done()

		watchCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		stream, streamErr := grpcClient.WatchGateways(watchCtx, &pb.WatchGatewaysRequest{})
		if streamErr != nil {
			sinkErr = fmt.Errorf("WatchGateways failed: %v", streamErr)
			close(sinkReady)
			return
		}

		close(sinkReady)

		seen := make(map[string]bool)
		for {
			evt, recvErr := stream.Recv()
			if recvErr == io.EOF {
				break
			}
			if recvErr != nil {
				if watchCtx.Err() != nil {
					sinkErr = fmt.Errorf("sink timed out: saw %d/%d items", len(seen), totalItems)
				} else {
					sinkErr = fmt.Errorf("stream recv error: %v", recvErr)
				}
				return
			}

			if evt.Type != pb.EventType_EVENT_TYPE_CREATED {
				continue
			}

			if evt.ResourceId != "" {
				seen[evt.ResourceId] = true
			}

			if len(seen) == totalItems {
				return
			}
		}
	}()

	wg.Wait()

	Expect(sourceErr).NotTo(HaveOccurred(), "source goroutine error")
	Expect(sinkErr).NotTo(HaveOccurred(), "sink goroutine error")

	listResp, listErr := grpcClient.ListGateways(context.Background(), &pb.ListGatewaysRequest{
		Page: 1,
		Size: 100,
	})
	Expect(listErr).NotTo(HaveOccurred())
	Expect(int(listResp.Metadata.Total)).To(BeNumerically(">=", totalItems))
}

func TestGRPCWatchGatewayDeleteIncludesResource(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	clusterID := registerTestCluster(t)
	createReq := &pb.CreateGatewayRequest{
		Name:      "delete-watch-test",
		ClusterId: clusterID,
	}
	created, err := grpcClient.CreateGateway(ctx, createReq)
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	watchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	stream, err := grpcClient.WatchGateways(watchCtx, &pb.WatchGatewaysRequest{})
	Expect(err).NotTo(HaveOccurred())

	// Block until the subscription handshake header arrives, so the delete below
	// cannot fire before the server subscribes. This is deterministic where a
	// fixed sleep only made the race unlikely.
	_, err = stream.Header()
	Expect(err).NotTo(HaveOccurred())

	_, err = grpcClient.DeleteGateway(ctx, &pb.DeleteGatewayRequest{Id: gatewayID})
	Expect(err).NotTo(HaveOccurred())

	// Read events until we find the delete event for our gateway
	for {
		evt, recvErr := stream.Recv()
		if recvErr != nil {
			t.Fatalf("stream closed before receiving delete event: %v", recvErr)
		}

		if evt.ResourceId != gatewayID {
			continue
		}
		if evt.Type != pb.EventType_EVENT_TYPE_DELETED {
			continue
		}

		Expect(evt.Gateway).NotTo(BeNil(), "delete event must include the gateway resource")
		Expect(evt.Gateway.Name).To(Equal("delete-watch-test"))
		Expect(evt.Gateway.Namespace).To(Equal(created.Gateway.Namespace))
		Expect(evt.Gateway.ClusterId).To(Equal(clusterID))
		break
	}
}

// TestGRPCWatchGatewaysSendsSubscriptionHeader asserts the watch RPC flushes its
// response header once the broker subscription is live. The control-plane watcher
// blocks on this header before it seeds its reconcile queue from a LIST, so the
// header is what closes the list-watch gap: without it, the client could list
// state and then miss an event that fires before the subscription registers.
func TestGRPCWatchGatewaysSendsSubscriptionHeader(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	watchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := grpcClient.WatchGateways(watchCtx, &pb.WatchGatewaysRequest{})
	Expect(err).NotTo(HaveOccurred())

	// Header() blocks until the server flushes its header, which it does only after
	// subscribing. It returning without error proves the handshake fired.
	_, err = stream.Header()
	Expect(err).NotTo(HaveOccurred())
}

// TestGRPCControlPlaneStartupListsExistingGateways covers what a newly started
// control plane relies on to pick up gateways created while it was down. The
// watch stream never replays existing state, so the control plane opens the
// watch, waits for its subscription header, and then lists its cluster's
// gateways to seed its reconcile queue. Gateways created before any connection
// must be returned by that list on every fresh connect, and another cluster's
// gateway must not.
func TestGRPCControlPlaneStartupListsExistingGateways(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	ownID, subject := registerTestClusterWithSubject(t)
	foreignID := registerTestCluster(t)

	userCtx := h.NewAuthenticatedContext(h.NewRandAccount())
	service := gateways.Service(&environments.Environment().Services)
	want := make(map[string]bool, 2)
	for _, name := range []string{"startup-seed-a", "startup-seed-b"} {
		gw, svcErr := service.Create(userCtx, &gateways.Gateway{Name: name, ClusterId: ownID})
		Expect(svcErr).NotTo(HaveOccurred(), "create gateway %s", name)
		want[gw.ID] = true
	}
	foreignGw, svcErr := service.Create(userCtx, &gateways.Gateway{Name: "startup-seed-foreign", ClusterId: foreignID})
	Expect(svcErr).NotTo(HaveOccurred())

	cpAccount := h.NewAccount("service-account-cp-"+subject, "cp", "")
	cpToken := h.CreateJWTStringWithClaims(cpAccount, test.ControlPlaneClaims(subject))

	for attempt := 0; attempt < 2; attempt++ {
		conn, err := grpc.NewClient(
			h.GRPCAddress(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&bearerToken{token: cpToken}),
		)
		Expect(err).NotTo(HaveOccurred())
		client := pb.NewGatewayServiceClient(conn)

		watchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		stream, err := client.WatchGateways(watchCtx, &pb.WatchGatewaysRequest{ClusterId: &ownID})
		Expect(err).NotTo(HaveOccurred())
		_, err = stream.Header()
		Expect(err).NotTo(HaveOccurred(), "connect %d: watch subscription header", attempt+1)

		listResp, err := client.ListGateways(watchCtx, &pb.ListGatewaysRequest{Page: 1, Size: 100, ClusterId: &ownID})
		Expect(err).NotTo(HaveOccurred(), "connect %d: list gateways", attempt+1)
		got := make(map[string]bool, len(listResp.Items))
		for _, item := range listResp.Items {
			Expect(item.GetClusterId()).To(Equal(ownID), "connect %d: listed another cluster's gateway", attempt+1)
			got[item.GetMetadata().GetId()] = true
		}
		for id := range want {
			Expect(got).To(HaveKey(id), "connect %d: pre-existing gateway missing from startup list", attempt+1)
		}
		Expect(got).NotTo(HaveKey(foreignGw.ID), "connect %d: foreign gateway listed", attempt+1)

		cancel()
		Expect(conn.Close()).To(Succeed())
	}
}

func TestGRPCGatewayErrorHandling(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	getReq := &pb.GetGatewayRequest{Id: "nonexistent"}
	_, err = grpcClient.GetGateway(context.Background(), getReq)
	Expect(err).To(HaveOccurred())

	deleteReq := &pb.DeleteGatewayRequest{Id: "nonexistent"}
	_, err = grpcClient.DeleteGateway(context.Background(), deleteReq)
	Expect(err).To(HaveOccurred())
}
