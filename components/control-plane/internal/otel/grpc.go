package otel

import (
	"os"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// GRPCDialOptions returns gRPC dial options that install OpenTelemetry client
// interceptors for unary and streaming RPCs. When telemetry is disabled
// (OTEL_EXPORTER_OTLP_ENDPOINT unset), it returns nil so the caller can
// append it unconditionally.
func GRPCDialOptions() []grpc.DialOption {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return nil
	}
	return []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
}
