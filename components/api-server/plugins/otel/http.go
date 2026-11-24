package otel

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// operationIDAttr carries the framework's per-request operation id (also returned
// as the X-Operation-ID response header and in the error envelope's operation_id
// field) so a user-visible failure can be joined to the server trace.
const operationIDAttr = attribute.Key("hypershell.operation_id")

// httpServerRequestDurationName is the OpenTelemetry semantic-convention metric
// name of the inbound HTTP latency histogram the otelhttp instrumentation emits
// (API-OBS-05). The OBS-SLI-07 bucket-boundary view in plugin.go matches this
// instrument by name, so the constant is shared to keep the producer (otelhttp)
// and the view selector in agreement.
const httpServerRequestDurationName = "http.server.request.duration"

// requestClassAttr classifies each HTTP request as a read or a write so the API
// latency SLO (OBS-SLI-08) can apply the 500 ms read target and the 2 s write
// target separately. The key sits in the http.request.* semconv namespace to read
// naturally alongside the method and status-code attributes otelhttp already
// records; it complements, and never replaces, those attributes. The value is
// bounded to exactly two values (read, write), so it adds no per-route or
// per-identifier cardinality.
const requestClassAttr = attribute.Key("http.request.class")

const (
	requestClassRead  = "read"
	requestClassWrite = "write"
)

// httpLatencySLOBoundaries returns the explicit, SLO-aligned bucket boundaries
// (in seconds) for the http.server.request.duration histogram (OBS-SLI-07). The
// boundaries include 0.5 and 2 seconds exactly, the read and write SLO
// thresholds, so the fraction of requests under each is computable from a bucket
// edge without interpolation. A fresh slice is returned on each call so no caller
// can mutate shared state.
func httpLatencySLOBoundaries() []float64 {
	return []float64{0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}
}

// classifyRequestMethod maps an HTTP method to its SLO request class (OBS-SLI-08).
// Safe methods (GET, HEAD, OPTIONS) are reads; every other method (POST, PUT,
// PATCH, DELETE, and anything else) is a write. It is a pure function so the
// classification is unit-testable without a running server.
func classifyRequestMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return requestClassRead
	default:
		return requestClassWrite
	}
}

// otelHTTPHandler wraps the whole HTTP handler (registered pre-auth, outside the
// mux router) so it creates one server span per request and extracts inbound W3C
// trace context via the global propagator. The span is named by method here; the
// in-router middleware refines it to a templated route once routing has matched.
func otelHTTPHandler(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "http.server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method
		}),
		// OBS-SLI-08: attach the bounded read/write request-class attribute to the
		// emitted request metrics (including http.server.request.duration) without
		// replacing the method or status-code attributes otelhttp already records.
		otelhttp.WithMetricAttributesFn(func(r *http.Request) []attribute.KeyValue {
			return []attribute.KeyValue{requestClassAttr.String(classifyRequestMethod(r.Method))}
		}),
	)
}

// routeTagMiddleware refines the server span created by otelHTTPHandler: it sets
// the span name to "METHOD /templated/route" and records http.route, so Jaeger
// groups spans by endpoint and resource identifiers never inflate span-name
// cardinality. It also records the operation id for trace-to-support correlation.
//
// OBS-SLI-10: the same templated route is added to the request metrics (including
// http.server.request.duration) via the otelhttp Labeler, so the API latency SLO
// can count user REST routes only and exclude the OpenAPI and managed-cluster
// registration routes. otelhttp merges the Labeler's attributes into the metric
// sample when the request completes. Only the bounded path template is recorded,
// never the raw path, so cardinality stays bounded by the registered routes; a
// request that matches no route adds no http.route attribute. This complements,
// and never replaces, the read/write request-class attribute (OBS-SLI-08) and the
// method and status-code attributes otelhttp already records.
func routeTagMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		if route := mux.CurrentRoute(r); route != nil {
			// GetPathTemplate keeps {id}-style placeholders, which are the
			// bounded route we want in the span name (unlike the raw path).
			if tmpl, err := route.GetPathTemplate(); err == nil && tmpl != "" {
				if span.IsRecording() {
					span.SetName(r.Method + " " + tmpl)
					span.SetAttributes(semconv.HTTPRoute(tmpl))
				}
				// Record the templated route on the request metrics regardless of
				// whether the span is sampled, so the latency SLO always sees the
				// route even when tracing is not recording this request.
				if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
					labeler.Add(semconv.HTTPRoute(tmpl))
				}
			}
		}
		if span.IsRecording() {
			if opID := logger.GetOperationID(r.Context()); opID != "" {
				span.SetAttributes(operationIDAttr.String(opID))
			}
		}
		next.ServeHTTP(w, r)
	})
}
