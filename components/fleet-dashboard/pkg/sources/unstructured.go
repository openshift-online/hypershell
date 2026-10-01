package sources

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Thin wrappers over apimachinery's unstructured accessors so the source code
// reads cleanly. Each returns the zero value on a missing/mistyped path.

func nestedString(obj map[string]any, fields ...string) (string, bool) {
	v, ok, err := unstructured.NestedString(obj, fields...)
	if err != nil {
		return "", false
	}
	return v, ok
}

func nestedSlice(obj map[string]any, fields ...string) ([]any, bool, error) {
	return unstructured.NestedSlice(obj, fields...)
}

func stringMap(obj map[string]any, fields ...string) map[string]string {
	m, ok, err := unstructured.NestedStringMap(obj, fields...)
	if !ok || err != nil {
		return map[string]string{}
	}
	return m
}
