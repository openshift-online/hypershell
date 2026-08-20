package gateways_test

import (
	"context"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// newSandboxCountClient spins up the integration server and returns an
// authenticated gateway gRPC client plus the authenticated context.
func newSandboxCountClient(t *testing.T) (pb.GatewayServiceClient, context.Context) {
	t.Helper()
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

	return pb.NewGatewayServiceClient(conn), ctx
}

func createGatewayForSandboxCount(ctx context.Context, client pb.GatewayServiceClient, name string) *pb.Gateway {
	created, err := client.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       name,
		FleetId:    "test-fleet",
		ClusterId:  "test-cluster",
		ReleaseId:  "test-release",
		DatabaseId: "test-db",
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Gateway.Namespace).To(MatchRegexp(`^openshell-[0-9a-f]{16}$`))
	return created.Gateway
}

func TestGRPCAdjustActiveSandboxCount(t *testing.T) {
	client, ctx := newSandboxCountClient(t)
	gw := createGatewayForSandboxCount(ctx, client, "adjust-count")
	ns := gw.Namespace

	// Increments accumulate.
	resp, err := client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: ns, Delta: 1})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(1)))

	resp, err = client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: ns, Delta: 1})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(2)))

	// A decrement reduces the count.
	resp, err = client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: ns, Delta: -1})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(1)))

	// The stored value is observable on the gateway.
	got, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gw.Metadata.Id})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.GetActiveSandboxCount()).To(Equal(int32(1)))
}

func TestGRPCActiveSandboxCountFloorsAtZero(t *testing.T) {
	client, ctx := newSandboxCountClient(t)
	gw := createGatewayForSandboxCount(ctx, client, "floor-count")
	ns := gw.Namespace

	// A decrement against an unset (NULL) count floors at zero, never negative.
	resp, err := client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: ns, Delta: -1})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(0)))

	// A large decrement from a small positive value also floors at zero.
	_, err = client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: ns, Delta: 2})
	Expect(err).NotTo(HaveOccurred())
	resp, err = client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: ns, Delta: -10})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(0)))
}

func TestGRPCSetActiveSandboxCount(t *testing.T) {
	client, ctx := newSandboxCountClient(t)
	gw := createGatewayForSandboxCount(ctx, client, "set-count")
	ns := gw.Namespace

	// Absolute set (self-heal path).
	resp, err := client.SetActiveSandboxCount(ctx, &pb.SetActiveSandboxCountRequest{Namespace: ns, Count: 5})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(5)))

	// Set can lower the value too.
	resp, err = client.SetActiveSandboxCount(ctx, &pb.SetActiveSandboxCountRequest{Namespace: ns, Count: 2})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(2)))

	// A negative absolute set is floored at zero.
	resp, err = client.SetActiveSandboxCount(ctx, &pb.SetActiveSandboxCountRequest{Namespace: ns, Count: -3})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(0)))
}

func TestGRPCConcurrentActiveSandboxIncrements(t *testing.T) {
	client, ctx := newSandboxCountClient(t)
	gw := createGatewayForSandboxCount(ctx, client, "concurrent-count")
	ns := gw.Namespace

	const workers = 20
	var wg sync.WaitGroup
	errs := make([]error, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			_, err := client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: ns, Delta: 1})
			errs[idx] = err
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		Expect(err).NotTo(HaveOccurred())
	}

	// No update is lost under concurrency: the atomic adjustment yields exactly
	// `workers`, never fewer.
	got, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gw.Metadata.Id})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.GetActiveSandboxCount()).To(Equal(int32(workers)))
}

func TestGRPCAdjustActiveSandboxCountUnknownNamespace(t *testing.T) {
	client, ctx := newSandboxCountClient(t)

	// A namespace with no live gateway is a no-op returning zero, not an error:
	// the count is advisory and a just-deleted gateway must not spam failures.
	resp, err := client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{
		Namespace: "openshell-0000000000000000",
		Delta:     1,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.ActiveSandboxCount).To(Equal(int32(0)))
}

func TestGRPCAdjustActiveSandboxCountRequiresNamespace(t *testing.T) {
	client, ctx := newSandboxCountClient(t)

	_, err := client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: "", Delta: 1})
	Expect(err).To(HaveOccurred())

	_, err = client.SetActiveSandboxCount(ctx, &pb.SetActiveSandboxCountRequest{Namespace: "", Count: 1})
	Expect(err).To(HaveOccurred())
}

// TestGRPCUpdateGatewayDoesNotClobberSandboxCount verifies the separate-writer
// invariant: a whole-row UpdateGateway (phase/status) must not overwrite the
// control-plane-owned active_sandbox_count.
func TestGRPCUpdateGatewayDoesNotClobberSandboxCount(t *testing.T) {
	client, ctx := newSandboxCountClient(t)
	gw := createGatewayForSandboxCount(ctx, client, "no-clobber-count")

	// Establish a count via the dedicated path.
	setResp, err := client.SetActiveSandboxCount(ctx, &pb.SetActiveSandboxCountRequest{Namespace: gw.Namespace, Count: 3})
	Expect(err).NotTo(HaveOccurred())
	Expect(setResp.ActiveSandboxCount).To(Equal(int32(3)))

	// A subsequent whole-row update of unrelated fields must leave the count intact.
	phase := "Running"
	status := "Healthy"
	_, err = client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:     gw.Metadata.Id,
		Phase:  &phase,
		Status: &status,
	})
	Expect(err).NotTo(HaveOccurred())

	got, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gw.Metadata.Id})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.GetActiveSandboxCount()).To(Equal(int32(3)))
	Expect(got.Gateway.GetPhase()).To(Equal("Running"))
}
