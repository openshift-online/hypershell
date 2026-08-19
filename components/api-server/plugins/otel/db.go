package otel

import (
	"github.com/golang/glog"
	pkgdb "github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
)

// registerDBTracing registers the OpenTelemetry GORM plugin with the framework's
// session-factory plugin registry, so every database query issued while handling
// a request produces a client span nested under that request's span (API-OBS-08).
// This is a cross-cutting concern applied once on the base connection, like the
// HTTP middleware and gRPC interceptors, and is transparent to plugin authors.
//
// It must run after the global TracerProvider is installed: otelgorm captures the
// provider when the plugin is constructed, so setupOTel must have run first.
// registerDBTracing is therefore called from the otel plugin's init() after
// setupOTel succeeds, not from an init() of its own (files init in name order, so
// a db.go init() would run before plugin.go and capture the no-op provider). The
// framework applies the registered plugin when the session factory opens its base
// connection, which happens later, during environment construction.
func registerDBTracing() {
	// WithoutQueryVariables masks bound parameter values with '?' so no resource
	// identifier, secret, or literal reaches the db.statement attribute
	// (API-OBS-06). WithoutMetrics keeps this to tracing; database metrics stay on
	// the framework's existing Prometheus collector (API-OBS-05, API-OBS-08).
	pkgdb.RegisterGormPlugin(otelgorm.NewPlugin(
		otelgorm.WithoutQueryVariables(),
		otelgorm.WithoutMetrics(),
	))

	glog.Info("OpenTelemetry GORM query tracing enabled")
}
