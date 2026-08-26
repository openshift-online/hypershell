package otel

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openshift-online/rh-trex-ai/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// operationIDAttr carries the framework's per-request operation id (also returned
// as the X-Operation-ID response header and in the error envelope's operation_id
// field) so a user-visible failure can be joined to the server trace.
const operationIDAttr = attribute.Key("hypershell.operation_id")

// otelHTTPHandler wraps the whole HTTP handler (registered pre-auth, outside the
// mux router) so it creates one server span per request and extracts inbound W3C
// trace context via the global propagator. The span is named by method here; the
// in-router middleware refines it to a templated route once routing has matched.
func otelHTTPHandler(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "http.server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method
		}),
	)
}

// registerHTTPRouteMiddleware installs the route-tagging middleware on the v1
// subrouter. It runs inside the router (after route matching), where
// mux.CurrentRoute is populated, so it can read the templated path.
func registerHTTPRouteMiddleware(apiV1Router *mux.Router, _ pkgserver.ServicesInterface, _ environments.JWTMiddleware, _ auth.AuthorizationMiddleware) {
	apiV1Router.Use(routeTagMiddleware)
}

// routeTagMiddleware refines the server span created by otelHTTPHandler: it sets
// the span name to "METHOD /templated/route" and records http.route, so Jaeger
// groups spans by endpoint and resource identifiers never inflate span-name
// cardinality. It also records the operation id for trace-to-support correlation.
func routeTagMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		if span.IsRecording() {
			if route := mux.CurrentRoute(r); route != nil {
				// GetPathTemplate keeps {id}-style placeholders, which are the
				// bounded route we want in the span name (unlike the raw path).
				if tmpl, err := route.GetPathTemplate(); err == nil && tmpl != "" {
					span.SetName(r.Method + " " + tmpl)
					span.SetAttributes(semconv.HTTPRoute(tmpl))
				}
			}
			if opID := logger.GetOperationID(r.Context()); opID != "" {
				span.SetAttributes(operationIDAttr.String(opID))
			}
		}
		next.ServeHTTP(w, r)
	})
}
