package managedDatabases_test

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

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

func TestGRPCManagedDatabaseCRUD(t *testing.T) {
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

	grpcClient := pb.NewManagedDatabaseServiceClient(conn)

	_, err = grpcClient.CreateManagedDatabase(ctx, &pb.CreateManagedDatabaseRequest{Name: "invalid-provider", FleetId: "TestFleetId", Provider: "unsupported"})
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument))

	createReq := &pb.CreateManagedDatabaseRequest{
		Name:             "TestName",
		FleetId:          "TestFleetId",
		Provider:         "deployment",
		Region:           func() *string { s := "TestRegion"; return &s }(),
		Engine:           func() *string { s := "TestEngine"; return &s }(),
		EngineVersion:    func() *string { s := "TestEngineVersion"; return &s }(),
		InstanceClass:    func() *string { s := "TestInstanceClass"; return &s }(),
		ConnectionSecret: func() *string { s := "TestConnectionSecret"; return &s }(),
		Status:           func() *string { s := "TestStatus"; return &s }(),
	}
	created, err := grpcClient.CreateManagedDatabase(ctx, createReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.ManagedDatabase.Metadata.Id).NotTo(BeEmpty())

	managedDatabaseID := created.ManagedDatabase.Metadata.Id

	getReq := &pb.GetManagedDatabaseRequest{Id: managedDatabaseID}
	retrieved, err := grpcClient.GetManagedDatabase(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved.ManagedDatabase.Metadata.Id).To(Equal(managedDatabaseID))

	updateReq := &pb.UpdateManagedDatabaseRequest{
		Id:               managedDatabaseID,
		Name:             func() *string { s := "UpdatedName"; return &s }(),
		FleetId:          func() *string { s := "UpdatedFleetId"; return &s }(),
		Provider:         func() *string { s := "deployment"; return &s }(),
		Region:           func() *string { s := "UpdatedRegion"; return &s }(),
		Engine:           func() *string { s := "UpdatedEngine"; return &s }(),
		EngineVersion:    func() *string { s := "UpdatedEngineVersion"; return &s }(),
		InstanceClass:    func() *string { s := "UpdatedInstanceClass"; return &s }(),
		ConnectionSecret: func() *string { s := "UpdatedConnectionSecret"; return &s }(),
		Status:           func() *string { s := "UpdatedStatus"; return &s }(),
	}
	updated, err := grpcClient.UpdateManagedDatabase(ctx, updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.ManagedDatabase.Metadata.Id).To(Equal(managedDatabaseID))

	unsupportedProvider := "unsupported"
	_, err = grpcClient.UpdateManagedDatabase(ctx, &pb.UpdateManagedDatabaseRequest{Id: managedDatabaseID, Provider: &unsupportedProvider})
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument))

	changedProvider := "cnpg"
	_, err = grpcClient.UpdateManagedDatabase(ctx, &pb.UpdateManagedDatabaseRequest{Id: managedDatabaseID, Provider: &changedProvider})
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
	retrieved, err = grpcClient.GetManagedDatabase(ctx, getReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved.ManagedDatabase.Provider).To(Equal("deployment"))

	readyStatus := "Ready"
	updated, err = grpcClient.UpdateManagedDatabase(ctx, &pb.UpdateManagedDatabaseRequest{Id: managedDatabaseID, Status: &readyStatus})
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.ManagedDatabase.Provider).To(Equal("deployment"))
	Expect(updated.ManagedDatabase.Status).NotTo(BeNil())
	Expect(*updated.ManagedDatabase.Status).To(Equal("Ready"))

	listReq := &pb.ListManagedDatabasesRequest{
		Page: 1,
		Size: 10,
	}
	listResp, err := grpcClient.ListManagedDatabases(ctx, listReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.Metadata.Total).To(BeNumerically(">=", 1))

	deleteReq := &pb.DeleteManagedDatabaseRequest{Id: managedDatabaseID}
	_, err = grpcClient.DeleteManagedDatabase(ctx, deleteReq)
	Expect(err).NotTo(HaveOccurred())

	_, err = grpcClient.GetManagedDatabase(ctx, getReq)
	Expect(status.Code(err)).To(Equal(codes.NotFound))
}

func TestGRPCWatchManagedDatabases(t *testing.T) {
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

	grpcClient := pb.NewManagedDatabaseServiceClient(conn)

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
			managedDatabaseInput := openapi.ManagedDatabase{
				Name:     name,
				FleetId:  "watch-test-fleet",
				Provider: "deployment",
			}
			_, resp, postErr := client.DefaultAPI.CreateManagedDatabase(ctx).ManagedDatabase(managedDatabaseInput).Execute()
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

		stream, streamErr := grpcClient.WatchManagedDatabases(watchCtx, &pb.WatchManagedDatabasesRequest{})
		if streamErr != nil {
			sinkErr = fmt.Errorf("WatchManagedDatabases failed: %v", streamErr)
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

	listResp, listErr := grpcClient.ListManagedDatabases(context.Background(), &pb.ListManagedDatabasesRequest{
		Page: 1,
		Size: 100,
	})
	Expect(listErr).NotTo(HaveOccurred())
	Expect(int(listResp.Metadata.Total)).To(BeNumerically(">=", totalItems))
}

func TestGRPCWatchManagedDatabaseDeleteIncludesResource(t *testing.T) {
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

	grpcClient := pb.NewManagedDatabaseServiceClient(conn)
	created, err := grpcClient.CreateManagedDatabase(ctx, &pb.CreateManagedDatabaseRequest{
		Name:     "delete-watch-test",
		FleetId:  "delete-watch-fleet",
		Provider: "deployment",
	})
	Expect(err).NotTo(HaveOccurred())
	databaseID := created.ManagedDatabase.Metadata.Id

	watchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stream, err := grpcClient.WatchManagedDatabases(watchCtx, &pb.WatchManagedDatabasesRequest{})
	Expect(err).NotTo(HaveOccurred())
	_, err = stream.Header()
	Expect(err).NotTo(HaveOccurred())

	_, err = grpcClient.DeleteManagedDatabase(ctx, &pb.DeleteManagedDatabaseRequest{Id: databaseID})
	Expect(err).NotTo(HaveOccurred())

	for {
		evt, recvErr := stream.Recv()
		if recvErr != nil {
			t.Fatalf("stream closed before receiving delete event: %v", recvErr)
		}
		if evt.ResourceId != databaseID || evt.Type != pb.EventType_EVENT_TYPE_DELETED {
			continue
		}
		Expect(evt.ManagedDatabase).NotTo(BeNil(), "delete event must include the ManagedDatabase tombstone")
		Expect(evt.ManagedDatabase.Metadata.Id).To(Equal(databaseID))
		Expect(evt.ManagedDatabase.Name).To(Equal("delete-watch-test"))
		Expect(evt.ManagedDatabase.FleetId).To(Equal("delete-watch-fleet"))
		Expect(evt.ManagedDatabase.Provider).To(Equal("deployment"))
		Expect(evt.ManagedDatabase.Namespace).To(Equal(created.ManagedDatabase.Namespace))
		Expect(evt.ManagedDatabase.Namespace).NotTo(BeEmpty())
		break
	}

	_, err = grpcClient.GetManagedDatabase(ctx, &pb.GetManagedDatabaseRequest{Id: databaseID})
	Expect(status.Code(err)).To(Equal(codes.NotFound))
}

// TestGRPCWatchManagedDatabasesSendsSubscriptionHeader asserts the watch RPC
// flushes its response header once the broker subscription is live. The
// control-plane watcher blocks on this header before it seeds its reconciler
// from a LIST, so the header is what closes the list-watch gap: without it,
// the client could list state and then miss an event that fires before the
// subscription registers.
func TestGRPCWatchManagedDatabasesSendsSubscriptionHeader(t *testing.T) {
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

	grpcClient := pb.NewManagedDatabaseServiceClient(conn)

	watchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := grpcClient.WatchManagedDatabases(watchCtx, &pb.WatchManagedDatabasesRequest{})
	Expect(err).NotTo(HaveOccurred())

	// Header() blocks until the server flushes its header, which it does only after
	// subscribing. It returning without error proves the handshake fired.
	_, err = stream.Header()
	Expect(err).NotTo(HaveOccurred())
}

func TestGRPCManagedDatabaseErrorHandling(t *testing.T) {
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

	grpcClient := pb.NewManagedDatabaseServiceClient(conn)

	getReq := &pb.GetManagedDatabaseRequest{Id: "nonexistent"}
	_, err = grpcClient.GetManagedDatabase(context.Background(), getReq)
	Expect(err).To(HaveOccurred())

	deleteReq := &pb.DeleteManagedDatabaseRequest{Id: "nonexistent"}
	_, err = grpcClient.DeleteManagedDatabase(context.Background(), deleteReq)
	Expect(err).To(HaveOccurred())
}
