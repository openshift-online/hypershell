package otel

import (
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/client-go/rest"
)

// InstrumentK8sConfig wraps the Kubernetes client transport with OTel HTTP
// tracing so every outbound API call produces a client span. When telemetry
// is not successfully initialized, it returns the config unmodified.
//
// The transport records only the canonicalized path template and safe
// attributes (method, status code). It does NOT use otelhttp.NewTransport,
// which would add url.full with concrete namespace and resource names,
// violating CP-OBS-05/06.
func InstrumentK8sConfig(cfg *rest.Config) *rest.Config {
	if !enabled || cfg == nil {
		return cfg
	}

	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return &k8sTracingTransport{base: rt}
	})
	return cfg
}

type k8sTracingTransport struct {
	base http.RoundTripper
}

func (t *k8sTracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tracer := otel.Tracer(TracerName)
	canonical := canonicalizePath(req.URL.Path)
	spanName := req.Method + " " + canonical

	ctx, span := tracer.Start(req.Context(), spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.request.method", req.Method),
			attribute.String("url.template", canonical),
		),
	)
	defer span.End()

	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		span.SetStatus(codes.Error, "request failed")
		span.RecordError(err)
		return resp, err
	}
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
	return resp, nil
}

// canonicalizePath parses Kubernetes API path structure generically and
// replaces concrete namespace and resource name segments with {name},
// independent of resource kind. This avoids maintaining a per-resource
// allowlist and ensures new resource types are canonicalized automatically.
//
// Kubernetes API paths follow two shapes:
//
//	Core:    /api/v1[/namespaces/{ns}]/{resource}[/{name}[/{subresource}]]
//	Grouped: /apis/{group}/{version}[/namespaces/{ns}]/{resource}[/{name}[/{subresource}]]
func canonicalizePath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return path
	}

	var i int
	switch parts[0] {
	case "api":
		// /api/v1/...
		i = 2
	case "apis":
		// /apis/{group}/{version}/...
		if len(parts) < 3 {
			return path
		}
		i = 3
	default:
		return path
	}

	// After the API prefix, optionally: namespaces/{ns}
	if i < len(parts) && parts[i] == "namespaces" {
		i++
		if i < len(parts) {
			parts[i] = "{name}"
			i++
		}
	}

	// Next is the resource type (plural, keep as-is)
	if i < len(parts) {
		i++
	}

	// Next is a concrete resource name (if present, replace)
	if i < len(parts) {
		parts[i] = "{name}"
	}

	return "/" + strings.Join(parts, "/")
}
