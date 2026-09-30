package helm

import (
	"bytes"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// DefaultGatewayResources returns the gateway container requests and limits
// used when GATEWAY_RESOURCES is unset. The upstream chart defaults to
// `resources: {}`, which would run the gateway BestEffort with no memory
// ceiling. The memory limit was raised from 512Mi after the gateway was
// OOMKilled during review cycles on the IBM cluster.
func DefaultGatewayResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}
}

// ParseGatewayResources parses a GATEWAY_RESOURCES value: a JSON Kubernetes
// ResourceRequirements object with `requests` and/or `limits`, e.g.
//
//	{"requests":{"cpu":"100m","memory":"512Mi"},"limits":{"cpu":"500m","memory":"1Gi"}}
//
// The value replaces the defaults entirely; it is not merged with them. It must
// set limits.memory so a gateway is never deployed without a memory ceiling,
// and no request may exceed its limit.
func ParseGatewayResources(raw string) (corev1.ResourceRequirements, error) {
	var rr corev1.ResourceRequirements
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rr); err != nil {
		return corev1.ResourceRequirements{}, fmt.Errorf("parse resources JSON: %w", err)
	}
	if dec.More() {
		return corev1.ResourceRequirements{}, fmt.Errorf("parse resources JSON: unexpected data after object")
	}
	if len(rr.Claims) > 0 {
		return corev1.ResourceRequirements{}, fmt.Errorf("resource claims are not supported")
	}
	if _, ok := rr.Limits[corev1.ResourceMemory]; !ok {
		return corev1.ResourceRequirements{}, fmt.Errorf("limits.memory is required")
	}
	for name, req := range rr.Requests {
		if req.Sign() < 0 {
			return corev1.ResourceRequirements{}, fmt.Errorf("requests.%s must not be negative", name)
		}
		if limit, ok := rr.Limits[name]; ok && req.Cmp(limit) > 0 {
			return corev1.ResourceRequirements{}, fmt.Errorf("requests.%s (%s) exceeds limits.%s (%s)", name, req.String(), name, limit.String())
		}
	}
	for name, limit := range rr.Limits {
		if limit.Sign() <= 0 {
			return corev1.ResourceRequirements{}, fmt.Errorf("limits.%s must be positive", name)
		}
	}
	return rr, nil
}

// resourcesValue converts ResourceRequirements into the chart's `resources`
// value, omitting empty sections.
func resourcesValue(rr corev1.ResourceRequirements) map[string]interface{} {
	out := make(map[string]interface{})
	if m := resourceListValue(rr.Requests); len(m) > 0 {
		out["requests"] = m
	}
	if m := resourceListValue(rr.Limits); len(m) > 0 {
		out["limits"] = m
	}
	return out
}

func resourceListValue(rl corev1.ResourceList) map[string]interface{} {
	m := make(map[string]interface{}, len(rl))
	for name, q := range rl {
		m[string(name)] = q.String()
	}
	return m
}
