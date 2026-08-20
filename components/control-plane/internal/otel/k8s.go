package otel

import (
	"net/http"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/client-go/rest"
)

// InstrumentK8sConfig wraps the Kubernetes client transport with OTel HTTP
// tracing so every outbound API call produces a client span. When telemetry is
// disabled, it returns the config unmodified.
func InstrumentK8sConfig(cfg *rest.Config) *rest.Config {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" || cfg == nil {
		return cfg
	}

	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		)
	})
	return cfg
}
