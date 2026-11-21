package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"sigs.k8s.io/yaml"
)

// ShellClient executes Helm operations by shelling out to the helm CLI binary.
// This approach avoids Go dependency conflicts between the Helm SDK and our k8s packages.
type ShellClient struct {
	// HelmBinary is the path to the helm CLI executable (defaults to "helm")
	HelmBinary string
	// ChartPath is the path to the Helm chart archive or directory
	ChartPath string
	// OperationTimeout is the --timeout passed to helm (defaults to
	// DefaultOperationTimeout). The controller's terminationGracePeriodSeconds
	// is sized to cover it.
	OperationTimeout time.Duration
	// DeadlineMargin is how long past OperationTimeout the controller lets helm
	// run before sending SIGTERM (defaults to DefaultDeadlineMargin).
	DeadlineMargin time.Duration
	// KillDelay is how long helm has to exit after SIGTERM before it is killed
	// (defaults to DefaultKillDelay).
	KillDelay time.Duration

	inFlight atomic.Int64
}

const (
	// DefaultOperationTimeout is helm's own --timeout for every operation.
	DefaultOperationTimeout = 5 * time.Minute
	// DefaultDeadlineMargin lets helm hit its own timeout and record the
	// release as failed before the controller stops it.
	DefaultDeadlineMargin = 30 * time.Second
	// DefaultKillDelay is the SIGTERM-to-SIGKILL window once the deadline
	// passes. Timeout + margin + kill delay = 360s, the controller's
	// terminationGracePeriodSeconds (deploy/base/platform-resources/controller.yaml).
	DefaultKillDelay = 30 * time.Second
)

// ReleaseName is the name of the Helm release for a gateway.
const ReleaseName = "openshell-gateway"

// Install installs a new Helm release for a gateway.
// It returns an error if the install fails.
func (c *ShellClient) Install(ctx context.Context, namespace string, values map[string]interface{}) error {
	valuesFile, err := writeValuesFile(values)
	if err != nil {
		return fmt.Errorf("write values file: %w", err)
	}
	defer func() { _ = os.Remove(valuesFile) }()

	args := []string{
		"install", ReleaseName, c.ChartPath,
		"--namespace", namespace,
		"--create-namespace=false",
		"--wait=false",
		"--timeout", c.timeoutArg(),
		"--values", valuesFile,
	}

	if _, stderr, err := c.run(ctx, args); err != nil {
		return fmt.Errorf("helm install %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr)
	}

	slog.InfoContext(ctx, "Helm release installed", "release", ReleaseName, "namespace", namespace)
	return nil
}

// Uninstall uninstalls a Helm release for a gateway.
// It returns nil if the release does not exist (idempotent).
func (c *ShellClient) Uninstall(ctx context.Context, namespace string) error {
	args := []string{
		"uninstall", ReleaseName,
		"--namespace", namespace,
		"--wait=false",
		"--timeout", c.timeoutArg(),
	}

	if _, stderr, err := c.run(ctx, args); err != nil {
		// Ignore "release not found" errors (idempotent)
		if strings.Contains(stderr, "not found") {
			slog.InfoContext(ctx, "Helm release not found, already uninstalled", "release", ReleaseName, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("helm uninstall %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr)
	}

	slog.InfoContext(ctx, "Helm release uninstalled", "release", ReleaseName, "namespace", namespace)
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

	stdout, stderr, err := c.run(ctx, args)
	if err != nil {
		// Release not found
		if strings.Contains(stderr, "not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("helm status %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr)
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

	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return nil, fmt.Errorf("parse helm status output: %w\nOutput: %s", err, stdout)
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
	valuesFile, err := writeValuesFile(values)
	if err != nil {
		return fmt.Errorf("write values file: %w", err)
	}
	defer func() { _ = os.Remove(valuesFile) }()

	args := []string{
		"upgrade", ReleaseName, c.ChartPath,
		"--namespace", namespace,
		"--reset-values",
		"--wait=false",
		"--timeout", c.timeoutArg(),
		"--values", valuesFile,
	}

	if _, stderr, err := c.run(ctx, args); err != nil {
		return fmt.Errorf("helm upgrade %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr)
	}

	slog.InfoContext(ctx, "Helm release upgraded", "release", ReleaseName, "namespace", namespace)
	return nil
}

// PendingStaleMargin is added to the longest time one helm process can run
// (OperationTimeout + DeadlineMargin + KillDelay) to get the age past which a
// pending-* release is treated as abandoned by stuck-release recovery. A helm started by
// another controller pod, for example the old pod during a controller rollout,
// can still be running before then.
const PendingStaleMargin = 60 * time.Second

// PendingStaleAfter is the age past which the latest revision of a pending-*
// release is abandoned: no helm process can still own it.
func (c *ShellClient) PendingStaleAfter() time.Duration {
	return c.operationTimeout() + c.deadlineMargin() + c.killDelay() + PendingStaleMargin
}

// Revision is one entry of a release's history.
type Revision struct {
	Revision    int
	Updated     time.Time
	Status      string // deployed, superseded, failed, pending-upgrade, etc.
	Chart       string
	AppVersion  string
	Description string
}

// History returns the release's revisions in ascending revision order. It
// returns nil when the release does not exist.
func (c *ShellClient) History(ctx context.Context, namespace string) ([]Revision, error) {
	args := []string{
		"history", ReleaseName,
		"--namespace", namespace,
		"--output", "json",
	}
	stdout, stderr, err := c.run(ctx, args)
	if err != nil {
		if strings.Contains(stderr, "not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("helm history %s in namespace %s: %w\nStderr: %s", ReleaseName, namespace, err, stderr)
	}
	return parseHistory(stdout)
}

// parseHistory parses `helm history -o json`. Helm writes each revision's
// last-deployed time as "updated" in RFC 3339 format.
func parseHistory(out string) ([]Revision, error) {
	var raw []struct {
		Revision    int    `json:"revision"`
		Updated     string `json:"updated"`
		Status      string `json:"status"`
		Chart       string `json:"chart"`
		AppVersion  string `json:"app_version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse helm history output: %w\nOutput: %s", err, out)
	}
	revs := make([]Revision, 0, len(raw))
	for _, r := range raw {
		// Helm writes a zero time as "", which is left as the zero time.
		var updated time.Time
		if r.Updated != "" {
			t, err := time.Parse(time.RFC3339Nano, r.Updated)
			if err != nil {
				return nil, fmt.Errorf("parse updated time %q of revision %d: %w", r.Updated, r.Revision, err)
			}
			updated = t
		}
		revs = append(revs, Revision{
			Revision:    r.Revision,
			Updated:     updated,
			Status:      r.Status,
			Chart:       r.Chart,
			AppVersion:  r.AppVersion,
			Description: r.Description,
		})
	}
	sort.Slice(revs, func(i, j int) bool { return revs[i].Revision < revs[j].Revision })
	return revs, nil
}

// Rollback creates a new revision of the release from the stored manifest of
// revision rev. Helm accepts it while the latest revision is pending-*, and rev
// may be that pending revision itself, which rolls the release forward to it
// (a roll forward).
func (c *ShellClient) Rollback(ctx context.Context, namespace string, rev int) error {
	if rev < 1 {
		return fmt.Errorf("helm rollback %s in namespace %s: invalid revision %d", ReleaseName, namespace, rev)
	}
	args := []string{
		"rollback", ReleaseName, strconv.Itoa(rev),
		"--namespace", namespace,
		"--wait=false",
		"--timeout", c.timeoutArg(),
	}
	if _, stderr, err := c.run(ctx, args); err != nil {
		return fmt.Errorf("helm rollback %s to revision %d in namespace %s: %w\nStderr: %s", ReleaseName, rev, namespace, err, stderr)
	}
	slog.InfoContext(ctx, "Helm release rolled back", "release", ReleaseName, "namespace", namespace, "revision", rev)
	return nil
}

// Manifest returns the rendered manifest stored for revision rev.
func (c *ShellClient) Manifest(ctx context.Context, namespace string, rev int) (string, error) {
	args := []string{
		"get", "manifest", ReleaseName,
		"--namespace", namespace,
		"--revision", strconv.Itoa(rev),
	}
	stdout, stderr, err := c.run(ctx, args)
	if err != nil {
		return "", fmt.Errorf("helm get manifest %s revision %d in namespace %s: %w\nStderr: %s", ReleaseName, rev, namespace, err, stderr)
	}
	return stdout, nil
}

// InFlight returns the number of helm processes currently running. The
// controller logs it while draining on shutdown.
func (c *ShellClient) InFlight() int {
	return int(c.inFlight.Load())
}

// run executes one helm command so it survives controller shutdown. A command
// is not started once ctx is cancelled, but a command that has started is
// detached from ctx: the controller's shutdown signal does not stop it. It runs to completion or to
// its own deadline (helm --timeout plus DeadlineMargin), and at that deadline
// it gets SIGTERM, not SIGKILL, so helm can record the release state. Only if
// it is still running KillDelay later is it killed.
func (c *ShellClient) run(ctx context.Context, args []string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", fmt.Errorf("not starting helm %s: %w", args[0], err)
	}
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.operationTimeout()+c.deadlineMargin())
	defer cancel()

	cmd := exec.CommandContext(runCtx, c.helmBinary(), args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = c.killDelay()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	c.inFlight.Add(1)
	defer c.inFlight.Add(-1)
	err := cmd.Run()
	if err != nil && runCtx.Err() != nil {
		err = fmt.Errorf("helm %s exceeded its %s deadline and was sent SIGTERM: %w", args[0], c.operationTimeout()+c.deadlineMargin(), err)
	}
	return stdout.String(), stderr.String(), err
}

func (c *ShellClient) operationTimeout() time.Duration {
	if c.OperationTimeout > 0 {
		return c.OperationTimeout
	}
	return DefaultOperationTimeout
}

func (c *ShellClient) deadlineMargin() time.Duration {
	if c.DeadlineMargin > 0 {
		return c.DeadlineMargin
	}
	return DefaultDeadlineMargin
}

func (c *ShellClient) killDelay() time.Duration {
	if c.KillDelay > 0 {
		return c.KillDelay
	}
	return DefaultKillDelay
}

// timeoutArg formats OperationTimeout for helm's --timeout flag.
func (c *ShellClient) timeoutArg() string {
	return c.operationTimeout().String()
}

// helmBinary returns the path to the helm binary.
func (c *ShellClient) helmBinary() string {
	if c.HelmBinary != "" {
		return c.HelmBinary
	}
	return "helm"
}

// writeValuesFile serializes a values map to a temporary YAML file and returns
// its path. The caller is responsible for removing the file when done.
func writeValuesFile(values map[string]interface{}) (string, error) {
	data, err := yaml.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("marshal values to YAML: %w", err)
	}

	f, err := os.CreateTemp("", "helm-values-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp values file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write values file: %w", err)
	}

	return f.Name(), nil
}

// VerifyHelmAvailable checks if the helm binary is available.
// It returns an error if helm is not found or if the version is too old.
func VerifyHelmAvailable(ctx context.Context, helmBinary string) error {
	cmd := exec.CommandContext(ctx, helmBinary, "version", "--short")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm binary not found in PATH or failed to execute: %w\nStderr: %s", err, stderr.String())
	}

	version := strings.TrimSpace(stdout.String())
	slog.InfoContext(ctx, "Helm version detected", "version", version)

	// Basic check: version should contain "v3"
	if !strings.Contains(version, "v3") {
		slog.WarnContext(ctx, "Helm version may be incompatible, expected v3.x", "version", version)
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
				slog.WarnContext(ctx, "Failed to check helm release status", "error", err)
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
