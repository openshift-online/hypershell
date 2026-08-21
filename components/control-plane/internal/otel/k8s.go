package otel

import (
	"net/http"
	"regexp"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/client-go/rest"
)

// InstrumentK8sConfig wraps the Kubernetes client transport with OTel HTTP
// tracing so every outbound API call produces a client span. When telemetry
// is not successfully initialized, it returns the config unmodified.
func InstrumentK8sConfig(cfg *rest.Config) *rest.Config {
	if !enabled || cfg == nil {
		return cfg
	}

	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + canonicalizePath(r.URL.Path)
			}),
		)
	})
	return cfg
}

// k8sPathSegments matches path segments that are concrete resource names or
// namespace names in Kubernetes API paths and replaces them with bounded
// placeholders, keeping span cardinality low and avoiding exporting
// namespace names or secret references (CP-OBS-06).
var k8sPathSegments = regexp.MustCompile(
	`(/namespaces/)[^/]+` +
		`|(/secrets/)[^/]+` +
		`|(/configmaps/)[^/]+` +
		`|(/deployments/)[^/]+` +
		`|(/services/)[^/]+` +
		`|(/serviceaccounts/)[^/]+` +
		`|(/pods/)[^/]+` +
		`|(/statefulsets/)[^/]+` +
		`|(/jobs/)[^/]+` +
		`|(/networkpolicies/)[^/]+` +
		`|(/roles/)[^/]+` +
		`|(/rolebindings/)[^/]+` +
		`|(/clusterroles/)[^/]+` +
		`|(/clusterrolebindings/)[^/]+`,
)

func canonicalizePath(path string) string {
	return k8sPathSegments.ReplaceAllStringFunc(path, func(match string) string {
		for i := len(match) - 1; i >= 0; i-- {
			if match[i] == '/' {
				return match[:i+1] + "{name}"
			}
		}
		return match
	})
}
