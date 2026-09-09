package gateway

import (
	"fmt"
	"os"
	"strings"

	"github.com/openshift-online/hypershell/components/control-plane/internal/config"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func applyMemoryRequest(obj *unstructured.Unstructured) error {
	if strings.TrimSpace(os.Getenv("GATEWAY_MEMORY_REQUEST")) == "" {
		return nil
	}
	memory, err := config.MemoryRequest("GATEWAY_MEMORY_REQUEST")
	if err != nil {
		return err
	}
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return fmt.Errorf("gateway deployment containers are unavailable")
	}
	foundGateway := false
	for _, raw := range containers {
		container, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("gateway deployment container has invalid format")
		}
		if container["name"] == "openshell-gateway" {
			foundGateway = true
			if err := unstructured.SetNestedField(container, memory, "resources", "requests", "memory"); err != nil {
				return err
			}
		}
	}
	if !foundGateway {
		return fmt.Errorf("gateway deployment has no openshell-gateway container")
	}
	return unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers")
}
