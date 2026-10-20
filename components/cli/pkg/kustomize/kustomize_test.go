package kustomize

import (
	"strings"
	"testing"

	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// writeInMemory builds an in-memory filesystem from a map of path -> contents.
func writeInMemory(t *testing.T, files map[string]string) filesys.FileSystem {
	t.Helper()
	fSys := filesys.MakeFsInMemory()
	for path, contents := range files {
		if err := fSys.WriteFile(path, []byte(contents)); err != nil {
			t.Fatalf("writing %s to in-memory fs: %v", path, err)
		}
	}
	return fSys
}

func TestBuild_SingleResource(t *testing.T) {
	fSys := writeInMemory(t, map[string]string{
		"/root/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - gateway.yaml
`,
		"/root/gateway.yaml": `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: test-gateway
spec:
  cluster_id: cluster-1
`,
	})

	resMap, err := Build(fSys, "/root")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := len(resMap.Resources()); got != 1 {
		t.Fatalf("Build() rendered %d resources, want 1", got)
	}
	if got := resMap.Resources()[0].GetKind(); got != "Gateway" {
		t.Errorf("Build() kind = %q, want Gateway", got)
	}
	if got := resMap.Resources()[0].GetName(); got != "test-gateway" {
		t.Errorf("Build() name = %q, want test-gateway", got)
	}
}

func TestBuild_BasesAndOverlays(t *testing.T) {
	// An overlay that references a base and patches the base resource. Exercises
	// multi-directory rendering entirely in memory.
	fSys := writeInMemory(t, map[string]string{
		"/base/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - gateway.yaml
  - template.yaml
`,
		"/base/gateway.yaml": `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: gateway-1
spec:
  cluster_id: base-cluster
`,
		"/base/template.yaml": `apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
metadata:
  name: template-1
spec:
  image: example.com/gateway:v1
`,
		"/overlay/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../base
commonLabels:
  env: prod
`,
	})

	resMap, err := Build(fSys, "/overlay")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := len(resMap.Resources()); got != 2 {
		t.Fatalf("Build() rendered %d resources, want 2", got)
	}

	kinds := map[string]int{}
	for _, r := range resMap.Resources() {
		kinds[r.GetKind()]++
		labels := r.GetLabels()
		if labels["env"] != "prod" {
			t.Errorf("resource %s/%s missing overlay label env=prod, got %v", r.GetKind(), r.GetName(), labels)
		}
	}
	if kinds["Gateway"] != 1 || kinds["SandboxTemplate"] != 1 {
		t.Errorf("Build() kinds = %v, want 1 Gateway and 1 SandboxTemplate", kinds)
	}
}

func TestBuildYAML_RendersStream(t *testing.T) {
	fSys := writeInMemory(t, map[string]string{
		"/root/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - resources.yaml
`,
		"/root/resources.yaml": `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: gateway-1
---
apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
metadata:
  name: template-1
`,
	})

	out, err := BuildYAML(fSys, "/root")
	if err != nil {
		t.Fatalf("BuildYAML() error = %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "kind: Gateway") || !strings.Contains(text, "kind: SandboxTemplate") {
		t.Errorf("BuildYAML() output missing expected kinds:\n%s", text)
	}
}

func TestBuild_MissingKustomization(t *testing.T) {
	fSys := filesys.MakeFsInMemory()
	_, err := Build(fSys, "/empty")
	if err == nil {
		t.Fatal("Build() expected error for missing kustomization, got nil")
	}
	if !strings.Contains(err.Error(), "kustomize build failed") {
		t.Errorf("Build() error = %v, want it wrapped with 'kustomize build failed'", err)
	}
}

func TestBuild_MissingReferencedResource(t *testing.T) {
	fSys := writeInMemory(t, map[string]string{
		"/root/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - missing.yaml
`,
	})
	_, err := Build(fSys, "/root")
	if err == nil {
		t.Fatal("Build() expected error for missing referenced resource, got nil")
	}
	if !strings.Contains(err.Error(), "kustomize build failed") {
		t.Errorf("Build() error = %v, want it wrapped with 'kustomize build failed'", err)
	}
}

func TestBuild_NilFilesystem(t *testing.T) {
	_, err := Build(nil, "/root")
	if err == nil {
		t.Fatal("Build() expected error for nil filesystem, got nil")
	}
}
