package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ImagePullRole identifies an operator-owned Role in an image namespace.
type ImagePullRole struct {
	Namespace string `json:"namespace"`
	Role      string `json:"role"`
}

func SandboxImagePullRoles() ([]ImagePullRole, error) {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_SANDBOX_IMAGE_PULL_ROLES"))
	if raw == "" {
		return nil, nil
	}
	var roles []ImagePullRole
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&roles); err != nil {
		return nil, fmt.Errorf("GATEWAY_SANDBOX_IMAGE_PULL_ROLES must be a JSON array of namespace and role objects")
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF || roles == nil || len(roles) > 32 {
		return nil, fmt.Errorf("GATEWAY_SANDBOX_IMAGE_PULL_ROLES must contain one array with at most 32 roles")
	}
	seen := map[ImagePullRole]bool{}
	for _, role := range roles {
		if role.Namespace == "" || len(validation.IsDNS1123Label(role.Namespace)) != 0 || role.Role == "" || len(validation.IsDNS1123Subdomain(role.Role)) != 0 || seen[role] {
			return nil, fmt.Errorf("GATEWAY_SANDBOX_IMAGE_PULL_ROLES requires unique valid namespace and role names")
		}
		seen[role] = true
	}
	return roles, nil
}
