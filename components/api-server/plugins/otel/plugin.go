// Package otel wires OpenTelemetry tracing, metrics and logs into the HyperShell API
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
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/pkg/logging"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
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
		slog.Error("OpenTelemetry initialization failed, continuing without telemetry", "error", err)
		return
	}

	if err := registerMetrics(); err != nil {
		slog.Error("OpenTelemetry metric instrument setup failed, continuing without gRPC metrics", "error", err)
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

	slog.Info("OpenTelemetry instrumentation enabled", "endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
}

// setupOTel initializes the trace, metric and log providers, the sampler, and
// the global W3C propagator, and returns a shutdown function that flushes every
// provider. Exporter endpoint and protocol are read from the standard OTEL_*
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

	shutdowns := []func(context.Context) error{tp.Shutdown}
	shutdown := func(ctx context.Context) error {
		var errs []error
		for _, s := range shutdowns {
			errs = append(errs, s(ctx))
		}
		return errors.Join(errs...)
	}

	// Metrics are opt-out via the standard OTEL_METRICS_EXPORTER=none. A
	// trace-only backend (Jaeger in the dev cluster) rejects the OTLP metrics
	// service, so this lets an operator disable metric export without losing
	// tracing. When metrics are off the global meter stays the no-op, and the
	// gRPC interceptors' recordRPCDuration becomes a no-op.
	if os.Getenv("OTEL_METRICS_EXPORTER") != "none" {
		metricExporter, err := otlpmetricgrpc.New(ctx)
		if err != nil {
			// Flush the already-started providers before returning so a
			// half-initialized SDK is not left running.
			return nil, errors.Join(fmt.Errorf("creating metric exporter: %w", err), shutdown(ctx))
		}
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
			sdkmetric.WithResource(res),
		)
		otel.SetMeterProvider(mp)
		shutdowns = append(shutdowns, mp.Shutdown)
	}

	// Logs are opt-out the same way, with OTEL_LOGS_EXPORTER=none. The slog
	// bridge is registered as a sink of the server's logger (pkg/logging), so
	// every record, including the framework's, is exported at the configured
	// --log-level with the trace and span IDs of the context it was logged with.
	if os.Getenv("OTEL_LOGS_EXPORTER") != "none" {
		logExporter, err := otlploggrpc.New(ctx)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("creating log exporter: %w", err), shutdown(ctx))
		}
		lp := sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
			sdklog.WithResource(res),
		)
		logging.RegisterSink(otelslog.NewHandler(tracerName, otelslog.WithLoggerProvider(lp)))
		shutdowns = append(shutdowns, lp.Shutdown)
	}

	return shutdown, nil
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
		slog.Warn("Invalid OTEL_TRACES_SAMPLER_ARG, using the default", "value", v, "default", defaultSampleArg, "error", err)
		return defaultSampleArg
	}
	return parsed
}

// awaitShutdown flushes buffered spans, metrics and logs on SIGTERM/SIGINT, bounded by
// a timeout so shutdown never blocks indefinitely.
func awaitShutdown(shutdown func(context.Context) error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), shutdownGracePrd)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		slog.Error("OpenTelemetry shutdown failed", "error", err)
	}
}
