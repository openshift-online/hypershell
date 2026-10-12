package urls

import (
	"strings"
	"testing"
)

func TestCoreAPIPrefix(t *testing.T) {
	if APIPrefix != "/api/hypershell/v1" {
		t.Errorf("APIPrefix = %q, want /api/hypershell/v1", APIPrefix)
	}
}

func TestExtAPIPrefix(t *testing.T) {
	if ExtAPIPrefix != "/api/hypershell/ext" {
		t.Errorf("ExtAPIPrefix = %q, want /api/hypershell/ext", ExtAPIPrefix)
	}
}

func TestCorePaths(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"GatewaysPath", GatewaysPath, "/api/hypershell/v1/gateways"},
		{"ManagedClustersPath", ManagedClustersPath, "/api/hypershell/v1/managed_clusters"},
		{"RolesPath", RolesPath, "/api/hypershell/v1/roles"},
		{"RoleBindingsPath", RoleBindingsPath, "/api/hypershell/v1/role_bindings"},
		{"UsersPath", UsersPath, "/api/hypershell/v1/users"},
	}
	for _, c := range cases {
		if c.path != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.path, c.want)
		}
	}
}

func TestExtPaths(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"AgentRuntimesPath", AgentRuntimesPath, "/api/hypershell/ext/agent_runtimes"},
		{"SandboxTemplatesPath", SandboxTemplatesPath, "/api/hypershell/ext/sandbox_templates"},
		{"ProviderSpecsPath", ProviderSpecsPath, "/api/hypershell/ext/provider_specs"},
		{"ProviderBindingsPath", ProviderBindingsPath, "/api/hypershell/ext/provider_bindings"},
		{"InferenceRoutesPath", InferenceRoutesPath, "/api/hypershell/ext/inference_routes"},
		{"SecretSourcesPath", SecretSourcesPath, "/api/hypershell/ext/secret_sources"},
		{"RepositoriesPath", RepositoriesPath, "/api/hypershell/ext/repositories"},
	}
	for _, c := range cases {
		if c.path != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.path, c.want)
		}
		if !strings.HasPrefix(c.path, ExtAPIPrefix) {
			t.Errorf("%s = %q does not start with ExtAPIPrefix %q", c.name, c.path, ExtAPIPrefix)
		}
		if strings.Contains(c.path, "/v1/") {
			t.Errorf("%s = %q must not contain /v1/ (should use /ext/)", c.name, c.path)
		}
	}
}

func TestCoreItemPaths(t *testing.T) {
	id := "abc123"
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"GatewayPath", GatewayPath(id), "/api/hypershell/v1/gateways/" + id},
		{"ManagedClusterPath", ManagedClusterPath(id), "/api/hypershell/v1/managed_clusters/" + id},
		{"RolePath", RolePath(id), "/api/hypershell/v1/roles/" + id},
		{"RoleBindingPath", RoleBindingPath(id), "/api/hypershell/v1/role_bindings/" + id},
		{"UserPath", UserPath(id), "/api/hypershell/v1/users/" + id},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s(%q) = %q, want %q", c.name, id, c.got, c.want)
		}
	}
}

func TestExtItemPaths(t *testing.T) {
	id := "xyz789"
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"AgentRuntimePath", AgentRuntimePath(id), "/api/hypershell/ext/agent_runtimes/" + id},
		{"SandboxTemplatePath", SandboxTemplatePath(id), "/api/hypershell/ext/sandbox_templates/" + id},
		{"ProviderSpecPath", ProviderSpecPath(id), "/api/hypershell/ext/provider_specs/" + id},
		{"ProviderBindingPath", ProviderBindingPath(id), "/api/hypershell/ext/provider_bindings/" + id},
		{"InferenceRoutePath", InferenceRoutePath(id), "/api/hypershell/ext/inference_routes/" + id},
		{"SecretSourcePath", SecretSourcePath(id), "/api/hypershell/ext/secret_sources/" + id},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s(%q) = %q, want %q", c.name, id, c.got, c.want)
		}
		if strings.Contains(c.got, "/v1/") {
			t.Errorf("%s(%q) = %q must not contain /v1/", c.name, id, c.got)
		}
	}
}
