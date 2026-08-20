// Package otel bootstraps OpenTelemetry tracing and metrics for the control
// plane. Telemetry is opt-in: when OTEL_EXPORTER_OTLP_ENDPOINT is unset, Init
// returns a no-op shutdown and installs no providers. See
// specs/platform/control-plane-observability.spec.md (CP-OBS-01).
package otel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	TracerName       = "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	defaultServiceNm = "hypershell-controller"
	shutdownGracePrd = 5 * time.Second
	defaultSampleArg = 1.0
)

// Init initializes the OTel SDK when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// It returns a shutdown function that flushes providers, bounded by a timeout.
// When telemetry is disabled the returned function is a no-op.
func Init(ctx context.Context) (shutdown func(context.Context) error, err error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}

	shutdown, err = setup(ctx)
	if err != nil {
		return func(context.Context) error { return nil }, fmt.Errorf("otel init: %w", err)
	}

	if err := registerMetrics(); err != nil {
		log.Printf("WARN OpenTelemetry metric instrument setup failed: %v", err)
	}

	log.Printf("INFO OpenTelemetry instrumentation enabled, exporting to %s", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	return shutdown, nil
}

// Shutdown flushes and shuts down the OTel SDK with a bounded timeout.
func Shutdown(shutdown func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGracePrd)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		log.Printf("ERROR OpenTelemetry shutdown: %v", err)
	}
}

func setup(ctx context.Context) (func(context.Context) error, error) {
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
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplerRatio()))),
	)
	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if os.Getenv("OTEL_METRICS_EXPORTER") == "none" {
		return tp.Shutdown, nil
	}

	metricExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
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

func samplerRatio() float64 {
	v := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	if v == "" {
		return defaultSampleArg
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		log.Printf("WARN invalid OTEL_TRACES_SAMPLER_ARG %q, defaulting to %v: %v", v, defaultSampleArg, err)
		return defaultSampleArg
	}
	return parsed
}
