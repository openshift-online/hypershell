package watcher

import (
	"context"
	"errors"
	"io"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type gatewayDeletionStream struct {
	grpc.ClientStream
	header metadata.MD
	events []*pb.WatchGatewaysResponse
	err    error
}

func (s *gatewayDeletionStream) Header() (metadata.MD, error) { return s.header, nil }
func (s *gatewayDeletionStream) Recv() (*pb.WatchGatewaysResponse, error) {
	if len(s.events) > 0 {
		event := s.events[0]
		s.events = s.events[1:]
		return event, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return nil, io.EOF
}

type gatewayDeletionClient struct {
	pb.GatewayServiceClient
	stream     *gatewayDeletionStream
	gotCluster string
	gotMode    []string
}

func (c *gatewayDeletionClient) WatchGateways(ctx context.Context, req *pb.WatchGatewaysRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.WatchGatewaysResponse], error) {
	c.gotCluster = req.GetClusterId()
	md, _ := metadata.FromOutgoingContext(ctx)
	c.gotMode = md.Get("hypershell-gateway-replay")
	return c.stream, nil
}
func TestGatewayDeletionReplayRecoversPendingEvents(t *testing.T) {
	stream := &gatewayDeletionStream{header: metadata.Pairs("hypershell-gateway-delete-tombstones", "v1"), events: []*pb.WatchGatewaysResponse{{Type: pb.EventType_EVENT_TYPE_DELETED, ResourceId: "gateway", Gateway: gw("gateway", "Running")}}}
	client := &gatewayDeletionClient{stream: stream}
	sink := newRecordingSink(nil)
	if err := replayDeletedGateways(context.Background(), client, sink, "cluster-a"); err != nil {
		t.Fatal(err)
	}
	if client.gotCluster != "cluster-a" || len(client.gotMode) != 1 || client.gotMode[0] != "deleted-v1" {
		t.Fatal("replay request lost cluster or mode")
	}
	if len(sink.enqueued) != 1 || sink.enqueued["gateway"].Type != EventDeleted {
		t.Fatal("pending deletion was not recovered")
	}
	for _, invalid := range []*gatewayDeletionStream{
		{},
		{header: stream.header, events: []*pb.WatchGatewaysResponse{{Type: pb.EventType_EVENT_TYPE_UPDATED, ResourceId: "gateway", Gateway: gw("gateway", "Running")}}},
		{header: stream.header, err: errors.New("connection lost")},
	} {
		if err := replayDeletedGateways(context.Background(), &gatewayDeletionClient{stream: invalid}, newRecordingSink(nil), ""); err == nil {
			t.Fatal("invalid replay was accepted")
		}
	}
}
