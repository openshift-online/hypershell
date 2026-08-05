package fleets_test

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

func TestGRPCFleetCRUD(t *testing.T) {
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

	grpcClient := pb.NewFleetServiceClient(conn)

	createReq := &pb.CreateFleetRequest{
		Name:        "TestName",
		Description: func() *string { s := "TestDescription"; return &s }(),
		Status:      func() *string { s := "TestStatus"; return &s }(),
	}
	created, err := grpcClient.CreateFleet(ctx, createReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Fleet.Metadata.Id).NotTo(BeEmpty())

	fleetID := created.Fleet.Metadata.Id

	getReq := &pb.GetFleetRequest{Id: fleetID}
	retrieved, err := grpcClient.GetFleet(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved.Fleet.Metadata.Id).To(Equal(fleetID))

	updateReq := &pb.UpdateFleetRequest{
		Id:          fleetID,
		Name:        func() *string { s := "UpdatedName"; return &s }(),
		Description: func() *string { s := "UpdatedDescription"; return &s }(),
		Status:      func() *string { s := "UpdatedStatus"; return &s }(),
	}
	updated, err := grpcClient.UpdateFleet(ctx, updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.Fleet.Metadata.Id).To(Equal(fleetID))

	listReq := &pb.ListFleetsRequest{
		Page: 1,
		Size: 10,
	}
	listResp, err := grpcClient.ListFleets(ctx, listReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.Metadata.Total).To(BeNumerically(">=", 1))

	deleteReq := &pb.DeleteFleetRequest{Id: fleetID}
	_, err = grpcClient.DeleteFleet(ctx, deleteReq)
	Expect(err).NotTo(HaveOccurred())

	_, err = grpcClient.GetFleet(ctx, getReq)
	Expect(err).To(HaveOccurred())
}

func TestGRPCWatchFleets(t *testing.T) {
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

	grpcClient := pb.NewFleetServiceClient(conn)

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
			fleetInput := openapi.Fleet{
				Name: name,
			}
			_, resp, postErr := client.DefaultAPI.CreateFleet(ctx).Fleet(fleetInput).Execute()
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

		stream, streamErr := grpcClient.WatchFleets(watchCtx, &pb.WatchFleetsRequest{})
		if streamErr != nil {
			sinkErr = fmt.Errorf("WatchFleets failed: %v", streamErr)
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

	listResp, listErr := grpcClient.ListFleets(context.Background(), &pb.ListFleetsRequest{
		Page: 1,
		Size: 100,
	})
	Expect(listErr).NotTo(HaveOccurred())
	Expect(int(listResp.Metadata.Total)).To(BeNumerically(">=", totalItems))
}

func TestGRPCFleetErrorHandling(t *testing.T) {
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

	grpcClient := pb.NewFleetServiceClient(conn)

	getReq := &pb.GetFleetRequest{Id: "nonexistent"}
	_, err = grpcClient.GetFleet(context.Background(), getReq)
	Expect(err).To(HaveOccurred())

	deleteReq := &pb.DeleteFleetRequest{Id: "nonexistent"}
	_, err = grpcClient.DeleteFleet(context.Background(), deleteReq)
	Expect(err).To(HaveOccurred())
}
