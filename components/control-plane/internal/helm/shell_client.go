package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// ShellClient executes Helm operations by shelling out to the helm CLI binary.
// This approach avoids Go dependency conflicts between the Helm SDK and our k8s packages.
type ShellClient struct {
	// HelmBinary is the path to the helm CLI executable (defaults to "helm")
	HelmBinary string
	// ChartPath is the path to the Helm chart archive or directory
	ChartPath string
}

// ReleaseName is the name of the Helm release for a gateway.
const ReleaseName = "openshell-gateway"

// Install installs a new Helm release for a gateway.
// It returns an error if the install fails.
func (c *ShellClient) Install(ctx context.Context, namespace string, values map[string]interface{}) error {
	valuesArgs, err := buildValuesArgs(values)
	if err != nil {
		return fmt.Errorf("build values args: %w", err)
	}

	args := []string{
		"install", ReleaseName, c.ChartPath,
		"--namespace", namespace,
		"--create-namespace=false",
		"--wait=false",
		"--timeout", "5m",
	}
	args = append(args, valuesArgs...)

	cmd := exec.CommandContext(ctx, c.helmBinary(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr.String())
	}

	log.Printf("INFO helm release %s installed in namespace %s", ReleaseName, namespace)
	return nil
}

// Uninstall uninstalls a Helm release for a gateway.
// It returns nil if the release does not exist (idempotent).
func (c *ShellClient) Uninstall(ctx context.Context, namespace string) error {
	args := []string{
		"uninstall", ReleaseName,
		"--namespace", namespace,
		"--wait=false",
		"--timeout", "5m",
	}

	cmd := exec.CommandContext(ctx, c.helmBinary(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Ignore "release not found" errors (idempotent)
		if strings.Contains(stderr.String(), "not found") {
			log.Printf("INFO helm release %s not found in namespace %s (already uninstalled)", ReleaseName, namespace)
			return nil
		}
		return fmt.Errorf("helm uninstall %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr.String())
	}

	log.Printf("INFO helm release %s uninstalled from namespace %s", ReleaseName, namespace)
	return nil
}

// ReleaseStatus represents the status of a Helm release.
type ReleaseStatus struct {
	Name      string
	Namespace string
	Status    string // deployed, failed, pending-install, etc.
	Revision  int
}

// GetReleaseStatus returns the status of a Helm release.
// It returns nil if the release does not exist.
func (c *ShellClient) GetReleaseStatus(ctx context.Context, namespace string) (*ReleaseStatus, error) {
	args := []string{
		"status", ReleaseName,
		"--namespace", namespace,
		"--output", "json",
	}

	cmd := exec.CommandContext(ctx, c.helmBinary(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Release not found
		if strings.Contains(stderr.String(), "not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("helm status %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr.String())
	}

	// Parse JSON output
	var result struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Info      struct {
			Status string `json:"status"`
		} `json:"info"`
		Version int `json:"version"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parse helm status output: %w\nOutput: %s", err, stdout.String())
	}

	return &ReleaseStatus{
		Name:      result.Name,
		Namespace: result.Namespace,
		Status:    result.Info.Status,
		Revision:  result.Version,
	}, nil
}

// Upgrade upgrades an existing Helm release.
// This is used to retry failed installs (status=failed or pending-install).
func (c *ShellClient) Upgrade(ctx context.Context, namespace string, values map[string]interface{}) error {
	valuesArgs, err := buildValuesArgs(values)
	if err != nil {
		return fmt.Errorf("build values args: %w", err)
	}

	args := []string{
		"upgrade", ReleaseName, c.ChartPath,
		"--namespace", namespace,
		"--reuse-values",
		"--wait=false",
		"--timeout", "5m",
	}
	args = append(args, valuesArgs...)

	cmd := exec.CommandContext(ctx, c.helmBinary(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm upgrade %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr.String())
	}

	log.Printf("INFO helm release %s upgraded in namespace %s", ReleaseName, namespace)
	return nil
}

// helmBinary returns the path to the helm binary.
func (c *ShellClient) helmBinary() string {
	if c.HelmBinary != "" {
		return c.HelmBinary
	}
	return "helm"
}

// buildValuesArgs converts a values map into --set arguments for helm CLI.
// It flattens nested maps using dot notation (e.g. "server.oidc.issuer=value").
func buildValuesArgs(values map[string]interface{}) ([]string, error) {
	var args []string
	flattened := flattenValues(values, "")

	for k, v := range flattened {
		// Convert value to string representation
		var valueStr string
		switch val := v.(type) {
		case string:
			valueStr = val
		case bool:
			valueStr = fmt.Sprintf("%t", val)
		case int, int32, int64:
			valueStr = fmt.Sprintf("%d", val)
		case float32, float64:
			valueStr = fmt.Sprintf("%f", val)
		case []string:
			valueStr = strings.Join(val, ",")
		case []interface{}:
			// Convert to comma-separated string
			var strs []string
			for _, item := range val {
				strs = append(strs, fmt.Sprintf("%v", item))
			}
			valueStr = strings.Join(strs, ",")
		case nil:
			// Skip nil values
			continue
		default:
			// For complex types, use JSON encoding
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("marshal value for key %s: %w", k, err)
			}
			valueStr = string(jsonBytes)
		}

		args = append(args, "--set", fmt.Sprintf("%s=%s", k, valueStr))
	}

	return args, nil
}

// flattenValues flattens a nested map into dot-notation keys.
// Example: {"server": {"oidc": {"issuer": "https://..."}}}
//
//	-> {"server.oidc.issuer": "https://..."}
func flattenValues(m map[string]interface{}, prefix string) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if nested, ok := v.(map[string]interface{}); ok {
			// Recursively flatten nested maps
			for nk, nv := range flattenValues(nested, key) {
				result[nk] = nv
			}
		} else {
			result[key] = v
		}
	}

	return result
}

// VerifyHelmAvailable checks if the helm binary is available.
// It returns an error if helm is not found or if the version is too old.
func VerifyHelmAvailable(helmBinary string) error {
	cmd := exec.Command(helmBinary, "version", "--short")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm binary not found in PATH or failed to execute: %w\nStderr: %s", err, stderr.String())
	}

	version := strings.TrimSpace(stdout.String())
	log.Printf("INFO helm version: %s", version)

	// Basic check: version should contain "v3"
	if !strings.Contains(version, "v3") {
		log.Printf("WARN helm version may be incompatible (expected v3.x): %s", version)
	}

	return nil
}

// WaitForHelmReady waits for a Helm release to reach a ready state.
// This is a helper for testing and may be used in the reconciler for synchronous operations.
func (c *ShellClient) WaitForHelmReady(ctx context.Context, namespace string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for helm release %s in namespace %s", ReleaseName, namespace)
		case <-ticker.C:
			status, err := c.GetReleaseStatus(ctx, namespace)
			if err != nil {
				log.Printf("WARN failed to check helm release status: %v", err)
				continue
			}
			if status == nil {
				return fmt.Errorf("helm release %s not found in namespace %s", ReleaseName, namespace)
			}
			if status.Status == "deployed" {
				return nil
			}
			if status.Status == "failed" {
				return fmt.Errorf("helm release %s in namespace %s is in failed state", ReleaseName, namespace)
			}
		}
	}
}
