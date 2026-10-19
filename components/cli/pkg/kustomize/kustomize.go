// Package kustomize is the single shared kustomize rendering implementation for
// the HyperShell stack. It renders a Kustomize root in process through an
// in-memory-capable filesystem, with no shell-out to an external `kustomize`
// binary.
//
// It has exactly two callers: the `hsctl apply -k` command (which renders a
// directory on disk) and the control plane's RepositoryReconciler (which renders
// a subtree fetched over the GitHub API into an in-memory filesystem). Keeping
// one implementation means the CLI and the server render identically and there
// is no second, divergent renderer to drift. See
// specs/platform/openshell-gateway.spec.md ("Shared Kustomize Library").
package kustomize

import (
	"fmt"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// Build renders the Kustomize root at path within fSys and returns the parsed
// resources as a resmap.ResMap. The caller supplies the filesystem: an on-disk
// filesystem (filesys.MakeFsOnDisk) for a local directory, or an in-memory
// filesystem (filesys.MakeFsInMemory) populated from a fetched subtree.
//
// Rendering uses kustomize's default options, which disable all plugins (no
// arbitrary generator/transformer execution) and restrict local file loading to
// the kustomize root directory. Note that remote bases referenced via git or
// http URLs in kustomization.yaml are not blocked by the root-only load
// restriction.
func Build(fSys filesys.FileSystem, path string) (resmap.ResMap, error) {
	if fSys == nil {
		return nil, fmt.Errorf("kustomize build failed: nil filesystem")
	}

	// MakeDefaultOptions already sets LoadRestrictionsRootOnly and a disabled
	// plugin config; it is the security posture the shared library guarantees for
	// both callers. Keep it explicit here so a future options change is a
	// deliberate edit in one place.
	k := krusty.MakeKustomizer(krusty.MakeDefaultOptions())

	resMap, err := k.Run(fSys, path)
	if err != nil {
		return nil, fmt.Errorf("kustomize build failed: %w", err)
	}

	return resMap, nil
}

// BuildYAML renders the Kustomize root at path within fSys and returns the
// rendered resources as a single multi-document YAML stream. It is a convenience
// over Build for callers that prefer to parse the YAML with their own decoder.
func BuildYAML(fSys filesys.FileSystem, path string) ([]byte, error) {
	resMap, err := Build(fSys, path)
	if err != nil {
		return nil, err
	}

	out, err := resMap.AsYaml()
	if err != nil {
		return nil, fmt.Errorf("kustomize build failed: serialize rendered resources: %w", err)
	}

	return out, nil
}
