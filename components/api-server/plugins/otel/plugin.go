// Package otel wires OpenTelemetry tracing and metrics into the HyperShell API
// server. It is a cross-cutting plugin (like plugins/rbac): its init() registers
// HTTP middleware and gRPC interceptors with the framework and reads its
// configuration from the environment. Instrumentation is applied once at the
// server layer, so plugin authors get spans and metrics without per-handler
// changes.
//
// Telemetry is opt-in: when OTEL_EXPORTER_OTLP_ENDPOINT is unset the SDK is not
// initialized, no middleware or interceptors are registered, and the server runs
// with no observability overhead. See specs/platform/api-server-observability.spec.md
// (HYPERSHELL-26).
package otel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/golang/glog"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// tracerName and meterName identify this instrumentation library in exported
// telemetry. They are the scope, not the service name (which is a resource
// attribute set from OTEL_SERVICE_NAME).
const (
	tracerName       = "github.com/openshift-online/hypershell/components/api-server/plugins/otel"
	defaultServiceNm = "hypershell-api-server"
	shutdownGracePrd = 5 * time.Second
	defaultSampleArg = 1.0
)

func init() {
	// Opt-in: with no collector endpoint there is nothing to export to, so skip
	// initialization entirely and leave the server free of any overhead.
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return
	}

	shutdown, err := setupOTel(context.Background())
	if err != nil {
		// Degrade, do not crash: a telemetry setup failure must not stop the API
		// server from serving requests.
		glog.Errorf("OpenTelemetry initialization failed, continuing without telemetry: %v", err)
		return
	}

	if err := registerMetrics(); err != nil {
		glog.Errorf("OpenTelemetry metric instrument setup failed, continuing without gRPC metrics: %v", err)
	}

	// HTTP: the outer handler creates the server span and extracts inbound W3C
	// trace context; the in-router middleware renames the span to a templated
	// route and records the operation id (see http.go).
	pkgserver.RegisterPreAuthMiddleware(otelHTTPHandler)
	pkgserver.RegisterRoutes("otel", registerHTTPRouteMiddleware)

	// gRPC: pre-auth interceptors so a propagated trace joins regardless of the
	// auth outcome (see grpc.go).
	pkgserver.RegisterPreAuthGRPCUnaryInterceptor(otelUnaryServerInterceptor())
	pkgserver.RegisterPreAuthGRPCStreamInterceptor(otelStreamServerInterceptor())

	// Database: register the GORM query-tracing plugin now that the global
	// TracerProvider is installed (see db.go). The framework applies it once when
	// the session factory opens its base connection.
	registerDBTracing()

	go awaitShutdown(shutdown)

	glog.Infof("OpenTelemetry instrumentation enabled, exporting to %s", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
}

// setupOTel initializes the trace and metric providers, the sampler, and the
// global W3C propagator, and returns a shutdown function that flushes both
// providers. Exporter endpoint and protocol are read from the standard OTEL_*
// environment variables by the exporter constructors.
func setupOTel(ctx context.Context) (func(context.Context) error, error) {
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = defaultServiceNm
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("creating telemetry resource: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		// Parent-based so a child inherits the upstream (browser -> BFF)
		// sampling decision; only a root trace is subject to the ratio.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplerRatio()))),
	)
	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Metrics are opt-out via the standard OTEL_METRICS_EXPORTER=none. A
	// trace-only backend (Jaeger in the dev cluster) rejects the OTLP metrics
	// service, so this lets an operator disable metric export without losing
	// tracing. When metrics are off the global meter stays the no-op, and the
	// gRPC interceptors' recordRPCDuration becomes a no-op.
	if os.Getenv("OTEL_METRICS_EXPORTER") == "none" {
		return tp.Shutdown, nil
	}

	metricExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		// Flush the already-started trace provider before returning so a
		// half-initialized SDK is not left running.
		return nil, errors.Join(fmt.Errorf("creating metric exporter: %w", err), tp.Shutdown(ctx))
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}, nil
}

// samplerRatio reads OTEL_TRACES_SAMPLER_ARG, defaulting to 1.0 (sample all root
// traces) when unset or unparseable.
func samplerRatio() float64 {
	v := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	if v == "" {
		return defaultSampleArg
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		glog.Warningf("invalid OTEL_TRACES_SAMPLER_ARG %q, defaulting to %v: %v", v, defaultSampleArg, err)
		return defaultSampleArg
	}
	return parsed
}

// awaitShutdown flushes buffered spans and metrics on SIGTERM/SIGINT, bounded by
// a timeout so shutdown never blocks indefinitely.
func awaitShutdown(shutdown func(context.Context) error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), shutdownGracePrd)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		glog.Errorf("OpenTelemetry shutdown error: %v", err)
	}
}
