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
		FleetId:     "TestFleetId",
		ClusterId:   "TestClusterId",
		ReleaseId:   "TestReleaseId",
		DatabaseId:  "TestDatabaseId",
		Namespace:   "TestNamespace",
		ExternalDns: func() *string { s := "TestExternalDns"; return &s }(),
		TlsMode:     func() *string { s := "TestTlsMode"; return &s }(),
		ServiceType: func() *string { s := "TestServiceType"; return &s }(),
		Status:      func() *string { s := "TestStatus"; return &s }(),
		Phase:       func() *string { s := "TestPhase"; return &s }(),
	}
	created, err := grpcClient.CreateGateway(ctx, createReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Metadata.Id).NotTo(BeEmpty())

	gatewayID := created.Metadata.Id

	getReq := &pb.GetGatewayRequest{Id: gatewayID}
	retrieved, err := grpcClient.GetGateway(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved.Metadata.Id).To(Equal(gatewayID))

	updateReq := &pb.UpdateGatewayRequest{
		Id:          gatewayID,
		Name:        func() *string { s := "UpdatedName"; return &s }(),
		FleetId:     func() *string { s := "UpdatedFleetId"; return &s }(),
		ClusterId:   func() *string { s := "UpdatedClusterId"; return &s }(),
		ReleaseId:   func() *string { s := "UpdatedReleaseId"; return &s }(),
		DatabaseId:  func() *string { s := "UpdatedDatabaseId"; return &s }(),
		Namespace:   func() *string { s := "UpdatedNamespace"; return &s }(),
		ExternalDns: func() *string { s := "UpdatedExternalDns"; return &s }(),
		TlsMode:     func() *string { s := "UpdatedTlsMode"; return &s }(),
		ServiceType: func() *string { s := "UpdatedServiceType"; return &s }(),
		Status:      func() *string { s := "UpdatedStatus"; return &s }(),
		Phase:       func() *string { s := "UpdatedPhase"; return &s }(),
	}
	updated, err := grpcClient.UpdateGateway(ctx, updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.Metadata.Id).To(Equal(gatewayID))

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

	go func() {
		defer wg.Done()
		<-sinkReady
		time.Sleep(100 * time.Millisecond)

		for name := range itemNames {
			gatewayInput := openapi.Gateway{
				Name: name,
			}
			_, resp, postErr := client.DefaultAPI.ApiHypershellV1GatewaysPost(ctx).Gateway(gatewayInput).Execute()
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
