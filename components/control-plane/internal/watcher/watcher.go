package watcher

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
)

type EventType int

const (
	EventCreated EventType = iota
	EventUpdated
	EventDeleted
)

type Event[T any] struct {
	Type       EventType
	ResourceID string
	Resource   T
}

type Handler[T any] interface {
	Handle(ctx context.Context, event Event[T]) error
}

func toEventType(t pb.EventType) EventType {
	switch t {
	case pb.EventType_EVENT_TYPE_CREATED:
		return EventCreated
	case pb.EventType_EVENT_TYPE_UPDATED:
		return EventUpdated
	case pb.EventType_EVENT_TYPE_DELETED:
		return EventDeleted
	default:
		return EventCreated
	}
}

func WatchFleets(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.Fleet]) error {
	client := pb.NewFleetServiceClient(conn)
	return watchLoop(ctx, "Fleet", func(ctx context.Context) error {
		stream, err := client.WatchFleets(ctx, &pb.WatchFleetsRequest{})
		if err != nil {
			return fmt.Errorf("starting fleet watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving fleet event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.Fleet]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.Fleet,
			}); err != nil {
				log.Printf("ERROR handling fleet %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchManagedClusters(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.ManagedCluster]) error {
	client := pb.NewManagedClusterServiceClient(conn)
	return watchLoop(ctx, "ManagedCluster", func(ctx context.Context) error {
		stream, err := client.WatchManagedClusters(ctx, &pb.WatchManagedClustersRequest{})
		if err != nil {
			return fmt.Errorf("starting managed cluster watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving managed cluster event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.ManagedCluster]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.ManagedCluster,
			}); err != nil {
				log.Printf("ERROR handling managed cluster %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchManagedDatabases(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.ManagedDatabase]) error {
	client := pb.NewManagedDatabaseServiceClient(conn)
	return watchLoop(ctx, "ManagedDatabase", func(ctx context.Context) error {
		stream, err := client.WatchManagedDatabases(ctx, &pb.WatchManagedDatabasesRequest{})
		if err != nil {
			return fmt.Errorf("starting managed database watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving managed database event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.ManagedDatabase]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.ManagedDatabase,
			}); err != nil {
				log.Printf("ERROR handling managed database %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchGatewayReleases(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.GatewayRelease]) error {
	client := pb.NewGatewayReleaseServiceClient(conn)
	return watchLoop(ctx, "GatewayRelease", func(ctx context.Context) error {
		stream, err := client.WatchGatewayReleases(ctx, &pb.WatchGatewayReleasesRequest{})
		if err != nil {
			return fmt.Errorf("starting gateway release watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving gateway release event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.GatewayRelease]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.GatewayRelease,
			}); err != nil {
				log.Printf("ERROR handling gateway release %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchGateways(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.Gateway]) error {
	client := pb.NewGatewayServiceClient(conn)
	return watchLoop(ctx, "Gateway", func(ctx context.Context) error {
		stream, err := client.WatchGateways(ctx, &pb.WatchGatewaysRequest{})
		if err != nil {
			return fmt.Errorf("starting gateway watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving gateway event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.Gateway]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.Gateway,
			}); err != nil {
				log.Printf("ERROR handling gateway %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchGatewayNetworks(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.GatewayNetwork]) error {
	client := pb.NewGatewayNetworkServiceClient(conn)
	return watchLoop(ctx, "GatewayNetwork", func(ctx context.Context) error {
		stream, err := client.WatchGatewayNetworks(ctx, &pb.WatchGatewayNetworksRequest{})
		if err != nil {
			return fmt.Errorf("starting gateway network watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving gateway network event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.GatewayNetwork]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.GatewayNetwork,
			}); err != nil {
				log.Printf("ERROR handling gateway network %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchRoleBindings(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.RoleBinding]) error {
	client := pb.NewRoleBindingServiceClient(conn)
	return watchLoop(ctx, "RoleBinding", func(ctx context.Context) error {
		stream, err := client.WatchRoleBindings(ctx, &pb.WatchRoleBindingsRequest{})
		if err != nil {
			return fmt.Errorf("starting role binding watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving role binding event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.RoleBinding]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.RoleBinding,
			}); err != nil {
				log.Printf("ERROR handling role binding %s: %v", event.ResourceId, err)
			}
		}
	})
}

func watchLoop(ctx context.Context, kind string, connectAndRecv func(ctx context.Context) error) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("INFO connecting %s watch stream...", kind)
		err := connectAndRecv(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		log.Printf("WARN %s watch stream disconnected: %v; reconnecting in %v", kind, err, backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
