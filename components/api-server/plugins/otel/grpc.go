package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// rpcDuration records inbound gRPC call latency. otelhttp emits HTTP metrics on
// its own, but the hand-written gRPC interceptors must record their own duration
// histogram to satisfy the request-metrics requirement.
var rpcDuration metric.Int64Histogram

// registerMetrics creates the gRPC metric instruments against the global meter
// provider configured in setupOTel.
func registerMetrics() error {
	meter := otel.Meter(meterName())
	var err error
	rpcDuration, err = meter.Int64Histogram(
		"rpc.server.duration",
		metric.WithUnit("ms"),
		metric.WithDescription("Latency of inbound gRPC calls"),
	)
	return err
}

func meterName() string { return tracerName }

// otelUnaryServerInterceptor creates a span per unary RPC, continuing an inbound
// trace extracted from gRPC metadata, and records the call duration.
func otelUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	tracer := otel.Tracer(tracerName)
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = extractGRPCPropagation(ctx)
		ctx, span := tracer.Start(ctx, info.FullMethod)
		defer span.End()

		start := time.Now()
		span.SetAttributes(
			semconv.RPCSystemGRPC,
			semconv.RPCService(serviceFromMethod(info.FullMethod)),
			semconv.RPCMethod(methodFromFullMethod(info.FullMethod)),
		)

		resp, err := handler(ctx, req)

		code := grpcStatusCode(err)
		span.SetAttributes(semconv.RPCGRPCStatusCodeKey.Int(code))
		recordRPCDuration(ctx, info.FullMethod, code, start)
		return resp, err
	}
}

// otelStreamServerInterceptor creates a span covering the lifetime of a streaming
// RPC, continuing an inbound trace extracted from gRPC metadata.
func otelStreamServerInterceptor() grpc.StreamServerInterceptor {
	tracer := otel.Tracer(tracerName)
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := extractGRPCPropagation(ss.Context())
		ctx, span := tracer.Start(ctx, info.FullMethod)
		defer span.End()

		start := time.Now()
		span.SetAttributes(
			semconv.RPCSystemGRPC,
			semconv.RPCService(serviceFromMethod(info.FullMethod)),
			semconv.RPCMethod(methodFromFullMethod(info.FullMethod)),
		)

		err := handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})

		code := grpcStatusCode(err)
		span.SetAttributes(semconv.RPCGRPCStatusCodeKey.Int(code))
		recordRPCDuration(ctx, info.FullMethod, code, start)
		return err
	}
}

// recordRPCDuration records the elapsed time of an RPC, labeled by service,
// method, and status, when the metric instrument is available.
func recordRPCDuration(ctx context.Context, fullMethod string, code int, start time.Time) {
	if rpcDuration == nil {
		return
	}
	rpcDuration.Record(ctx, time.Since(start).Milliseconds(), metric.WithAttributes(
		semconv.RPCSystemGRPC,
		semconv.RPCService(serviceFromMethod(fullMethod)),
		semconv.RPCMethod(methodFromFullMethod(fullMethod)),
		semconv.RPCGRPCStatusCodeKey.Int(code),
	))
}

func grpcStatusCode(err error) int {
	if err == nil {
		return 0
	}
	s, _ := status.FromError(err)
	return int(s.Code())
}

// extractGRPCPropagation reads W3C trace context from inbound gRPC metadata and
// returns a context carrying the extracted remote span context, so the RPC span
// continues the caller's trace.
func extractGRPCPropagation(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, metadataCarrier(md))
}

// metadataCarrier adapts gRPC metadata to the OTel TextMapCarrier interface.
type metadataCarrier metadata.MD

func (mc metadataCarrier) Get(key string) string {
	vals := metadata.MD(mc).Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (mc metadataCarrier) Set(key, val string) {
	metadata.MD(mc).Set(key, val)
}

func (mc metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(mc))
	for k := range mc {
		keys = append(keys, k)
	}
	return keys
}

// wrappedStream carries the span-enriched context into the stream handler so
// downstream code sees the RPC span.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// serviceFromMethod extracts the fully qualified service name from a gRPC
// "/pkg.Service/Method" full method string.
func serviceFromMethod(fullMethod string) string {
	fullMethod = trimLeadingSlash(fullMethod)
	for i := len(fullMethod) - 1; i >= 0; i-- {
		if fullMethod[i] == '/' {
			return fullMethod[:i]
		}
	}
	return fullMethod
}

// methodFromFullMethod extracts the method name from a gRPC full method string.
func methodFromFullMethod(fullMethod string) string {
	for i := len(fullMethod) - 1; i >= 0; i-- {
		if fullMethod[i] == '/' {
			return fullMethod[i+1:]
		}
	}
	return fullMethod
}

func trimLeadingSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}
