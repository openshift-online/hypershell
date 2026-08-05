package managedClusters_test

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

func TestGRPCManagedClusterCRUD(t *testing.T) {
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

	grpcClient := pb.NewManagedClusterServiceClient(conn)

	createReq := &pb.CreateManagedClusterRequest{
		Name:             "TestName",
		FleetId:          "TestFleetId",
		Provider:         "TestProvider",
		Region:           func() *string { s := "TestRegion"; return &s }(),
		KubeconfigSecret: "TestKubeconfigSecret",
		Status:           func() *string { s := "TestStatus"; return &s }(),
		ApiServerUrl:     func() *string { s := "TestApiServerUrl"; return &s }(),
	}
	created, err := grpcClient.CreateManagedCluster(ctx, createReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.ManagedCluster.Metadata.Id).NotTo(BeEmpty())

	managedClusterID := created.ManagedCluster.Metadata.Id

	getReq := &pb.GetManagedClusterRequest{Id: managedClusterID}
	retrieved, err := grpcClient.GetManagedCluster(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved.ManagedCluster.Metadata.Id).To(Equal(managedClusterID))

	updateReq := &pb.UpdateManagedClusterRequest{
		Id:               managedClusterID,
		Name:             func() *string { s := "UpdatedName"; return &s }(),
		FleetId:          func() *string { s := "UpdatedFleetId"; return &s }(),
		Provider:         func() *string { s := "UpdatedProvider"; return &s }(),
		Region:           func() *string { s := "UpdatedRegion"; return &s }(),
		KubeconfigSecret: func() *string { s := "UpdatedKubeconfigSecret"; return &s }(),
		Status:           func() *string { s := "UpdatedStatus"; return &s }(),
		ApiServerUrl:     func() *string { s := "UpdatedApiServerUrl"; return &s }(),
	}
	updated, err := grpcClient.UpdateManagedCluster(ctx, updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.ManagedCluster.Metadata.Id).To(Equal(managedClusterID))

	listReq := &pb.ListManagedClustersRequest{
		Page: 1,
		Size: 10,
	}
	listResp, err := grpcClient.ListManagedClusters(ctx, listReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.Metadata.Total).To(BeNumerically(">=", 1))

	deleteReq := &pb.DeleteManagedClusterRequest{Id: managedClusterID}
	_, err = grpcClient.DeleteManagedCluster(ctx, deleteReq)
	Expect(err).NotTo(HaveOccurred())

	_, err = grpcClient.GetManagedCluster(ctx, getReq)
	Expect(err).To(HaveOccurred())
}

func TestGRPCWatchManagedClusters(t *testing.T) {
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

	grpcClient := pb.NewManagedClusterServiceClient(conn)

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
			managedClusterInput := openapi.ManagedCluster{
				Name: name,
			}
			_, resp, postErr := client.DefaultAPI.CreateManagedCluster(ctx).ManagedCluster(managedClusterInput).Execute()
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

		stream, streamErr := grpcClient.WatchManagedClusters(watchCtx, &pb.WatchManagedClustersRequest{})
		if streamErr != nil {
			sinkErr = fmt.Errorf("WatchManagedClusters failed: %v", streamErr)
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

	listResp, listErr := grpcClient.ListManagedClusters(context.Background(), &pb.ListManagedClustersRequest{
		Page: 1,
		Size: 100,
	})
	Expect(listErr).NotTo(HaveOccurred())
	Expect(int(listResp.Metadata.Total)).To(BeNumerically(">=", totalItems))
}

func TestGRPCManagedClusterErrorHandling(t *testing.T) {
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

	grpcClient := pb.NewManagedClusterServiceClient(conn)

	getReq := &pb.GetManagedClusterRequest{Id: "nonexistent"}
	_, err = grpcClient.GetManagedCluster(context.Background(), getReq)
	Expect(err).To(HaveOccurred())

	deleteReq := &pb.DeleteManagedClusterRequest{Id: "nonexistent"}
	_, err = grpcClient.DeleteManagedCluster(context.Background(), deleteReq)
	Expect(err).To(HaveOccurred())
}
