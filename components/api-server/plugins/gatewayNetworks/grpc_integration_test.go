package gatewayNetworks_test

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
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
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

func TestGRPCGatewayNetworkCRUD(t *testing.T) {
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

	grpcClient := pb.NewGatewayNetworkServiceClient(conn)

	createReq := &pb.CreateGatewayNetworkRequest{
		Name:         "TestName",
		Topology:     func() *string { s := "TestTopology"; return &s }(),
		TunnelMode:   func() *string { s := "TestTunnelMode"; return &s }(),
		HubGatewayId: func() *string { s := "TestHubGatewayId"; return &s }(),
		Status:       func() *string { s := "TestStatus"; return &s }(),
	}
	created, err := grpcClient.CreateGatewayNetwork(ctx, createReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.GatewayNetwork.Metadata.Id).NotTo(BeEmpty())

	gatewayNetworkID := created.GatewayNetwork.Metadata.Id

	getReq := &pb.GetGatewayNetworkRequest{Id: gatewayNetworkID}
	retrieved, err := grpcClient.GetGatewayNetwork(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved.GatewayNetwork.Metadata.Id).To(Equal(gatewayNetworkID))

	updateReq := &pb.UpdateGatewayNetworkRequest{
		Id:           gatewayNetworkID,
		Name:         func() *string { s := "UpdatedName"; return &s }(),
		Topology:     func() *string { s := "UpdatedTopology"; return &s }(),
		TunnelMode:   func() *string { s := "UpdatedTunnelMode"; return &s }(),
		HubGatewayId: func() *string { s := "UpdatedHubGatewayId"; return &s }(),
		Status:       func() *string { s := "UpdatedStatus"; return &s }(),
	}
	updated, err := grpcClient.UpdateGatewayNetwork(ctx, updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.GatewayNetwork.Metadata.Id).To(Equal(gatewayNetworkID))

	listReq := &pb.ListGatewayNetworksRequest{
		Page: 1,
		Size: 10,
	}
	listResp, err := grpcClient.ListGatewayNetworks(ctx, listReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.Metadata.Total).To(BeNumerically(">=", 1))

	deleteReq := &pb.DeleteGatewayNetworkRequest{Id: gatewayNetworkID}
	_, err = grpcClient.DeleteGatewayNetwork(ctx, deleteReq)
	Expect(err).NotTo(HaveOccurred())

	_, err = grpcClient.GetGatewayNetwork(ctx, getReq)
	Expect(err).To(HaveOccurred())
}

func TestGRPCWatchGatewayNetworks(t *testing.T) {
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

	grpcClient := pb.NewGatewayNetworkServiceClient(conn)

	itemNames := make(map[string]bool, totalItems)
	for i := 0; i < totalItems; i++ {
		itemNames[fmt.Sprintf("watch_test_%d", i)] = true
	}

	var sourceErr error
	var sinkErr error
	var wg sync.WaitGroup
	wg.Add(2)

	sinkReady := make(chan struct{})

	go func() {
		defer wg.Done()
		<-sinkReady
		time.Sleep(100 * time.Millisecond)

		for name := range itemNames {
			gatewayNetworkInput := openapi.GatewayNetwork{
				Name: name,
			}
			_, resp, postErr := client.DefaultAPI.CreateGatewayNetwork(ctx).GatewayNetwork(gatewayNetworkInput).Execute()
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

		stream, streamErr := grpcClient.WatchGatewayNetworks(watchCtx, &pb.WatchGatewayNetworksRequest{})
		if streamErr != nil {
			sinkErr = fmt.Errorf("WatchGatewayNetworks failed: %v", streamErr)
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

	listResp, listErr := grpcClient.ListGatewayNetworks(context.Background(), &pb.ListGatewayNetworksRequest{
		Page: 1,
		Size: 100,
	})
	Expect(listErr).NotTo(HaveOccurred())
	Expect(int(listResp.Metadata.Total)).To(BeNumerically(">=", totalItems))
}

func TestGRPCGatewayNetworkErrorHandling(t *testing.T) {
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

	grpcClient := pb.NewGatewayNetworkServiceClient(conn)

	getReq := &pb.GetGatewayNetworkRequest{Id: "nonexistent"}
	_, err = grpcClient.GetGatewayNetwork(context.Background(), getReq)
	Expect(err).To(HaveOccurred())

	deleteReq := &pb.DeleteGatewayNetworkRequest{Id: "nonexistent"}
	_, err = grpcClient.DeleteGatewayNetwork(context.Background(), deleteReq)
	Expect(err).To(HaveOccurred())
}
