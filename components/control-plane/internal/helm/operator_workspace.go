package helm

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// OperatorWorkspaceConfig is deployment-owned: gateway users cannot grant access
// to external Kubernetes resources or choose trusted workload labels.
type OperatorWorkspaceConfig struct {
	NamespaceLabelKey        string   `json:"namespaceLabelKey"`
	AllowedPodLabels         []string `json:"allowedPodLabels"`
	AllowDriverConfig        bool     `json:"allowDriverConfig"`
	DisableResourceAdmission bool     `json:"disableResourceAdmission"`
	MTLSAuth                 bool     `json:"mtlsAuth"`
}

func ParseOperatorWorkspace(raw string) (*OperatorWorkspaceConfig, error) {
	var config OperatorWorkspaceConfig
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("expected one configuration object")
	}
	if len(validation.IsQualifiedName(config.NamespaceLabelKey)) != 0 {
		return nil, fmt.Errorf("namespaceLabelKey must be a Kubernetes label key")
	}
	if len(config.AllowedPodLabels) == 0 {
		return nil, fmt.Errorf("allowedPodLabels must not be empty")
	}
	for _, label := range config.AllowedPodLabels {
		name := strings.TrimSuffix(label, "/*")
		if strings.HasSuffix(label, "/*") {
			if len(validation.IsDNS1123Subdomain(name)) != 0 {
				return nil, fmt.Errorf("invalid Pod label prefix %q", label)
			}
		} else if len(validation.IsQualifiedName(label)) != 0 {
			return nil, fmt.Errorf("invalid Pod label %q", label)
		}
	}
	return &config, nil
}
