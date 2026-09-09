package config

import (
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

// WorkspaceStorage reads operator defaults for new sandbox workspace PVCs.
// Empty values retain the defaults of the selected OpenShell gateway image.
func WorkspaceStorage() (storageClass, defaultSize string, err error) {
	storageClass = strings.TrimSpace(os.Getenv("GATEWAY_WORKSPACE_STORAGE_CLASS"))
	if storageClass != "" && len(validation.IsDNS1123Subdomain(storageClass)) != 0 {
		return "", "", fmt.Errorf("GATEWAY_WORKSPACE_STORAGE_CLASS must be a Kubernetes storage class name")
	}
	defaultSize = strings.TrimSpace(os.Getenv("GATEWAY_WORKSPACE_DEFAULT_STORAGE_SIZE"))
	if defaultSize != "" {
		quantity, parseErr := resource.ParseQuantity(defaultSize)
		if parseErr != nil || quantity.Sign() <= 0 {
			return "", "", fmt.Errorf("GATEWAY_WORKSPACE_DEFAULT_STORAGE_SIZE must be a positive Kubernetes storage quantity")
		}
		defaultSize = quantity.String()
	}
	return storageClass, defaultSize, nil
}
