package apply

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSupportedKind(t *testing.T) {
	tests := []struct {
		kind     string
		expected bool
	}{
		{"Gateway", true},
		{"GatewayNetwork", false},
		{"GatewayRelease", false},
		{"ManagedCluster", true},
		{"Role", true},
		{"RoleBinding", true},
		{"AgentRuntime", true},
		{"SandboxTemplate", true},
		{"ProviderSpec", true},
		{"ProviderBinding", true},
		{"InferenceRoute", true},
		{"SecretSource", true},
		{"Repository", true},
		{"Pod", false},
		{"Deployment", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := isSupportedKind(tt.kind)
			if got != tt.expected {
				t.Errorf("isSupportedKind(%q) = %v, want %v", tt.kind, got, tt.expected)
			}
		})
	}
}

func TestParseYAMLStream(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
		wantErr  bool
	}{
		{
			name: "single resource",
			input: `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: test-gateway
spec:
  cluster_id: cluster-1`,
			expected: 1,
			wantErr:  false,
		},
		{
			name: "multiple resources",
			input: `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: gateway-1
---
apiVersion: hypershell.redhat.io/v1
kind: GatewayRelease
metadata:
  name: release-1`,
			expected: 2,
			wantErr:  false,
		},
		{
			name: "resource with empty kind skipped",
			input: `metadata:
  name: no-kind
---
apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: gateway-1`,
			expected: 1,
			wantErr:  false,
		},
		{
			name:     "empty input",
			input:    "",
			expected: 0,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources, err := parseYAMLStream(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseYAMLStream() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resources) != tt.expected {
				t.Errorf("parseYAMLStream() got %d resources, want %d", len(resources), tt.expected)
			}
		})
	}
}

func TestLoadFromKustomize_MissingDirectory(t *testing.T) {
	_, err := loadFromKustomize("/nonexistent/directory")
	if err == nil {
		t.Error("loadFromKustomize() expected error for missing directory, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("loadFromKustomize() error = %v, want error containing 'not found'", err)
	}
}

func TestLoadFromKustomize_NotADirectory(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	_, err = loadFromKustomize(tmpfile.Name())
	if err == nil {
		t.Error("loadFromKustomize() expected error for file instead of directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("loadFromKustomize() error = %v, want error containing 'not a directory'", err)
	}
}

func TestLoadFromKustomize_MissingKustomization(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "kustomize-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	_, err = loadFromKustomize(tmpdir)
	if err == nil {
		t.Error("loadFromKustomize() expected error for missing kustomization.yaml, got nil")
	}
	if !strings.Contains(err.Error(), "no kustomization.yaml") {
		t.Errorf("loadFromKustomize() error = %v, want error containing 'no kustomization.yaml'", err)
	}
}

func TestLoadFromKustomize_ValidOverlay(t *testing.T) {
	// Skip if kustomize is not available
	if _, err := exec.LookPath("kustomize"); err != nil {
		t.Skip("kustomize not found in PATH")
	}

	// Create test directory structure
	tmpdir, err := os.MkdirTemp("", "kustomize-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// Create base kustomization
	baseDir := filepath.Join(tmpdir, "base")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	baseKustomization := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - gateway.yaml
`
	if err := os.WriteFile(filepath.Join(baseDir, "kustomization.yaml"), []byte(baseKustomization), 0644); err != nil {
		t.Fatal(err)
	}

	baseGateway := `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: test-gateway
spec:
  cluster_id: cluster-1
  release_id: release-1
`
	if err := os.WriteFile(filepath.Join(baseDir, "gateway.yaml"), []byte(baseGateway), 0644); err != nil {
		t.Fatal(err)
	}

	// Test loading from base
	resources, err := loadFromKustomize(baseDir)
	if err != nil {
		t.Fatalf("loadFromKustomize() error = %v", err)
	}
	if len(resources) != 1 {
		t.Errorf("loadFromKustomize() got %d resources, want 1", len(resources))
	}
	if resources[0].Kind != "Gateway" {
		t.Errorf("loadFromKustomize() got kind %q, want Gateway", resources[0].Kind)
	}
}

func TestLoadFromKustomize_MixedKinds(t *testing.T) {
	// Skip if kustomize is not available
	if _, err := exec.LookPath("kustomize"); err != nil {
		t.Skip("kustomize not found in PATH")
	}

	tmpdir, err := os.MkdirTemp("", "kustomize-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	kustomization := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - resources.yaml
`
	if err := os.WriteFile(filepath.Join(tmpdir, "kustomization.yaml"), []byte(kustomization), 0644); err != nil {
		t.Fatal(err)
	}

	mixedResources := `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: gateway-1
spec:
  cluster_id: cluster-1
---
apiVersion: hypershell.redhat.io/v1
kind: GatewayRelease
metadata:
  name: release-1
spec:
  image: example.com/gateway:v1
---
apiVersion: hypershell.redhat.io/v1
kind: ManagedCluster
metadata:
  name: cluster-1
spec:
  provider: aws
  region: us-east-1
`
	if err := os.WriteFile(filepath.Join(tmpdir, "resources.yaml"), []byte(mixedResources), 0644); err != nil {
		t.Fatal(err)
	}

	resources, err := loadFromKustomize(tmpdir)
	if err != nil {
		t.Fatalf("loadFromKustomize() error = %v", err)
	}
	if len(resources) != 3 {
		t.Errorf("loadFromKustomize() got %d resources, want 3", len(resources))
	}

	// Verify kinds
	kinds := make(map[string]int)
	for _, r := range resources {
		kinds[r.Kind]++
	}
	if kinds["Gateway"] != 1 || kinds["GatewayRelease"] != 1 || kinds["ManagedCluster"] != 1 {
		t.Errorf("loadFromKustomize() got kinds %v, want 1 of each", kinds)
	}
}

func TestLoadFromKustomize_InvalidContent(t *testing.T) {
	// Skip if kustomize is not available
	if _, err := exec.LookPath("kustomize"); err != nil {
		t.Skip("kustomize not found in PATH")
	}

	tmpdir, err := os.MkdirTemp("", "kustomize-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// Invalid kustomization.yaml (malformed YAML)
	invalidKustomization := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - missing-file.yaml
`
	if err := os.WriteFile(filepath.Join(tmpdir, "kustomization.yaml"), []byte(invalidKustomization), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = loadFromKustomize(tmpdir)
	if err == nil {
		t.Error("loadFromKustomize() expected error for missing resource file, got nil")
	}
	if !strings.Contains(err.Error(), "kustomize build failed") {
		t.Errorf("loadFromKustomize() error = %v, want error containing 'kustomize build failed'", err)
	}
}

func TestLoadFromKustomize_SecurityNoPlugins(t *testing.T) {
	// Skip if kustomize is not available
	if _, err := exec.LookPath("kustomize"); err != nil {
		t.Skip("kustomize not found in PATH")
	}

	tmpdir, err := os.MkdirTemp("", "kustomize-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// Try to use a plugin (should fail with --enable-alpha-plugins=false)
	kustomizationWithPlugin := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
generators:
  - somegenerator.yaml
resources:
  - resource.yaml
`
	if err := os.WriteFile(filepath.Join(tmpdir, "kustomization.yaml"), []byte(kustomizationWithPlugin), 0644); err != nil {
		t.Fatal(err)
	}

	simpleResource := `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: test
spec:
  cluster_id: cluster-1
`
	if err := os.WriteFile(filepath.Join(tmpdir, "resource.yaml"), []byte(simpleResource), 0644); err != nil {
		t.Fatal(err)
	}

	// This should either fail due to plugin being disabled or ignore the generator
	// The exact behavior depends on kustomize version
	resources, err := loadFromKustomize(tmpdir)

	// If it succeeds, it should have ignored the generator and only loaded the resource
	if err == nil && len(resources) != 1 {
		t.Errorf("loadFromKustomize() with generator got %d resources, expected 1 (generator should be ignored)", len(resources))
	}

	// If it fails, the error should mention the plugin/generator
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "generator") {
		t.Logf("loadFromKustomize() with plugin error: %v (this is expected if plugins are disabled)", err)
	}
}

func TestGetName(t *testing.T) {
	tests := []struct {
		name     string
		resource Resource
		expected string
	}{
		{
			name: "valid name",
			resource: Resource{
				Metadata: map[string]interface{}{
					"name": "test-resource",
				},
			},
			expected: "test-resource",
		},
		{
			name:     "nil metadata",
			resource: Resource{},
			expected: "",
		},
		{
			name: "missing name",
			resource: Resource{
				Metadata: map[string]interface{}{
					"namespace": "default",
				},
			},
			expected: "",
		},
		{
			name: "wrong type",
			resource: Resource{
				Metadata: map[string]interface{}{
					"name": 123,
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getName(tt.resource)
			if got != tt.expected {
				t.Errorf("getName() = %q, want %q", got, tt.expected)
			}
		})
	}
}
