package helm

import (
	"fmt"
	"os"
	"os/exec"
)

// VerifyChartPath verifies that the Helm chart exists at the specified path.
// It returns an error if the chart is not found or is not readable.
func VerifyChartPath(chartPath string) error {
	if chartPath == "" {
		return fmt.Errorf("chart path is required")
	}

	info, err := os.Stat(chartPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("helm chart not found at %s", chartPath)
		}
		return fmt.Errorf("stat helm chart at %s: %w", chartPath, err)
	}

	// Chart can be either a directory or a .tgz archive
	if !info.IsDir() && !isChartArchive(chartPath) {
		return fmt.Errorf("helm chart at %s is neither a directory nor a .tgz archive", chartPath)
	}

	return nil
}

// isChartArchive checks if the file is a Helm chart archive (.tgz).
func isChartArchive(path string) bool {
	// Simple check: ends with .tgz or .tar.gz
	return len(path) > 4 && (path[len(path)-4:] == ".tgz" || path[len(path)-7:] == ".tar.gz")
}

// PullChart pulls a Helm chart from an OCI registry to a local path.
// This is used for development when HELM_CHART_REGISTRY is set.
func PullChart(registryURL, version, destinationDir string) error {
	if registryURL == "" {
		return fmt.Errorf("registry URL is required")
	}
	if version == "" {
		return fmt.Errorf("chart version is required")
	}
	if destinationDir == "" {
		return fmt.Errorf("destination directory is required")
	}

	// Use helm CLI to pull the chart
	args := []string{
		"pull", registryURL,
		"--version", version,
		"--destination", destinationDir,
	}

	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm pull %s:%s: %w\nOutput: %s", registryURL, version, err, output)
	}

	return nil
}
