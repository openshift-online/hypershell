package gatewayReleases_test

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

func TestGRPCGatewayReleaseCRUD(t *testing.T) {
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

	grpcClient := pb.NewGatewayReleaseServiceClient(conn)

	createReq := &pb.CreateGatewayReleaseRequest{
		Name:            "TestName",
		Image:           "TestImage",
		RolloutStrategy: func() *string { s := "TestRolloutStrategy"; return &s }(),
		CanaryPercent:   func() *int32 { v := int32(42); return &v }(),
		CanaryDuration:  func() *string { s := "TestCanaryDuration"; return &s }(),
		Status:          func() *string { s := "TestStatus"; return &s }(),
	}
	created, err := grpcClient.CreateGatewayRelease(ctx, createReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.GatewayRelease.Metadata.Id).NotTo(BeEmpty())

	gatewayReleaseID := created.GatewayRelease.Metadata.Id

	getReq := &pb.GetGatewayReleaseRequest{Id: gatewayReleaseID}
	retrieved, err := grpcClient.GetGatewayRelease(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved.GatewayRelease.Metadata.Id).To(Equal(gatewayReleaseID))

	updateReq := &pb.UpdateGatewayReleaseRequest{
		Id:              gatewayReleaseID,
		Name:            func() *string { s := "UpdatedName"; return &s }(),
		Image:           func() *string { s := "UpdatedImage"; return &s }(),
		RolloutStrategy: func() *string { s := "UpdatedRolloutStrategy"; return &s }(),
		CanaryPercent:   func() *int32 { v := int32(99); return &v }(),
		CanaryDuration:  func() *string { s := "UpdatedCanaryDuration"; return &s }(),
		Status:          func() *string { s := "UpdatedStatus"; return &s }(),
	}
	updated, err := grpcClient.UpdateGatewayRelease(ctx, updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.GatewayRelease.Metadata.Id).To(Equal(gatewayReleaseID))

	listReq := &pb.ListGatewayReleasesRequest{
		Page: 1,
		Size: 10,
	}
	listResp, err := grpcClient.ListGatewayReleases(ctx, listReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.Metadata.Total).To(BeNumerically(">=", 1))

	deleteReq := &pb.DeleteGatewayReleaseRequest{Id: gatewayReleaseID}
	_, err = grpcClient.DeleteGatewayRelease(ctx, deleteReq)
	Expect(err).NotTo(HaveOccurred())

	_, err = grpcClient.GetGatewayRelease(ctx, getReq)
	Expect(err).To(HaveOccurred())
}

func TestGRPCWatchGatewayReleases(t *testing.T) {
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

	grpcClient := pb.NewGatewayReleaseServiceClient(conn)

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
			gatewayReleaseInput := openapi.GatewayRelease{
				Name: name,
			}
			_, resp, postErr := client.DefaultAPI.CreateGatewayRelease(ctx).GatewayRelease(gatewayReleaseInput).Execute()
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

		stream, streamErr := grpcClient.WatchGatewayReleases(watchCtx, &pb.WatchGatewayReleasesRequest{})
		if streamErr != nil {
			sinkErr = fmt.Errorf("WatchGatewayReleases failed: %v", streamErr)
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

	listResp, listErr := grpcClient.ListGatewayReleases(context.Background(), &pb.ListGatewayReleasesRequest{
		Page: 1,
		Size: 100,
	})
	Expect(listErr).NotTo(HaveOccurred())
	Expect(int(listResp.Metadata.Total)).To(BeNumerically(">=", totalItems))
}

func TestGRPCGatewayReleaseErrorHandling(t *testing.T) {
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

	grpcClient := pb.NewGatewayReleaseServiceClient(conn)

	getReq := &pb.GetGatewayReleaseRequest{Id: "nonexistent"}
	_, err = grpcClient.GetGatewayRelease(context.Background(), getReq)
	Expect(err).To(HaveOccurred())

	deleteReq := &pb.DeleteGatewayReleaseRequest{Id: "nonexistent"}
	_, err = grpcClient.DeleteGatewayRelease(context.Background(), deleteReq)
	Expect(err).To(HaveOccurred())
}
