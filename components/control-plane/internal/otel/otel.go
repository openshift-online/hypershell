// Package otel bootstraps OpenTelemetry tracing and metrics for the control
// plane. Telemetry is opt-in: when OTEL_EXPORTER_OTLP_ENDPOINT is unset, Init
// returns a no-op shutdown and installs no providers. See
// specs/platform/control-plane-observability.spec.md (OBS-CPL-01).
package otel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/openshift-online/hypershell/components/control-plane/internal/logging"
)

const (
	TracerName       = "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	defaultServiceNm = "hypershell-controller"
	shutdownGracePrd = 5 * time.Second
	defaultSampleArg = 1.0
)

// enabled tracks whether the OTel SDK was successfully initialized. When
// false, GRPCDialOptions and InstrumentK8sConfig return no-ops so there is
// zero instrumentation overhead on an unconfigured or failed startup.
var enabled bool

// metricsEnabled tracks whether the SDK has an active metric exporter and all
// control-plane metric instruments were registered. Queue instrumentation uses
// it to avoid ready-time tracking and callbacks when metrics are disabled.
var metricsEnabled bool

// MetricsEnabled reports whether the control plane can export OTel metrics.
func MetricsEnabled() bool { return metricsEnabled }

// Init initializes the OTel SDK when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// namespace is the control-plane instance namespace (HYPERSHELL_NAMESPACE); when
// non-empty it is recorded as k8s.namespace.name so BFF PromQL can scope OTLP
// series the same way as provision-time metrics.
// It returns a shutdown function that flushes providers, bounded by a timeout.
// When telemetry is disabled or initialization fails, the returned function
// is a no-op and internal instrumentation stays off.
func Init(ctx context.Context, namespace string) (shutdown func(context.Context) error, err error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}

	shutdown, err = setup(ctx, namespace)
	if err != nil {
		return func(context.Context) error { return nil }, fmt.Errorf("otel init: %w", err)
	}

	if os.Getenv("OTEL_METRICS_EXPORTER") != "none" {
		if err := registerMetrics(); err != nil {
			slog.WarnContext(ctx, "OpenTelemetry metric instrument setup failed", "error", err)
		} else {
			metricsEnabled = true
		}
	}

	enabled = true
	slog.InfoContext(ctx, "OpenTelemetry instrumentation enabled", "url", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	return shutdown, nil
}

// Shutdown flushes and shuts down the OTel SDK with a bounded timeout.
func Shutdown(shutdown func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGracePrd)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		slog.ErrorContext(ctx, "OpenTelemetry shutdown failed", "error", err)
	}
}

func setup(ctx context.Context, namespace string) (func(context.Context) error, error) {
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = defaultServiceNm
	}

	attrs := []attribute.KeyValue{semconv.ServiceName(serviceName)}
	if namespace != "" {
		attrs = append(attrs, semconv.K8SNamespaceName(namespace))
	}
	res, err := resource.New(ctx, resource.WithAttributes(attrs...))
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
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplerRatio()))),
	)
	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdowns := []func(context.Context) error{tp.Shutdown}
	shutdownAll := func(ctx context.Context) error {
		var errs []error
		for i := len(shutdowns) - 1; i >= 0; i-- {
			errs = append(errs, shutdowns[i](ctx))
		}
		return errors.Join(errs...)
	}

	if os.Getenv("OTEL_METRICS_EXPORTER") != "none" {
		metricExporter, err := otlpmetricgrpc.New(ctx)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("creating metric exporter: %w", err), shutdownAll(ctx))
		}
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
			sdkmetric.WithResource(res),
		)
		otel.SetMeterProvider(mp)
		shutdowns = append(shutdowns, mp.Shutdown)
	}

	// Logs are opt-out the same way, with OTEL_LOGS_EXPORTER=none
	// (OBS-CPL-11). The slog bridge becomes a sink of the default logger, so
	// every record at HYPERSHELL_LOG_LEVEL is exported with the trace and
	// span IDs of the context it was logged with.
	if os.Getenv("OTEL_LOGS_EXPORTER") != "none" {
		logExporter, err := otlploggrpc.New(ctx)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("creating log exporter: %w", err), shutdownAll(ctx))
		}
		lp := sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
			sdklog.WithResource(res),
		)
		logging.AddSink(otelslog.NewHandler(TracerName, otelslog.WithLoggerProvider(lp)))
		shutdowns = append(shutdowns, lp.Shutdown)
	}

	return shutdownAll, nil
}

func samplerRatio() float64 {
	v := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	if v == "" {
		return defaultSampleArg
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 1 {
		slog.Warn("Invalid OTEL_TRACES_SAMPLER_ARG, using default", "value", v, "default", defaultSampleArg)
		return defaultSampleArg
	}
	return parsed
}
