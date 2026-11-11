package helm

import (
	"fmt"
	"os"
	"strings"
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
	return strings.HasSuffix(path, ".tgz") || strings.HasSuffix(path, ".tar.gz")
}
