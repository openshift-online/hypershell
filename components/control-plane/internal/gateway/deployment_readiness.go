package gateway

import (
	"context"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"time"
)

const deploymentDatabaseName = `openshell-gateway-db`
const deploymentReadinessWaitTimeout = 2 * time.Minute
const deploymentReadinessPollInterval = 2 * time.Second

type deploymentReadinessWaitOptions struct{ timeout, pollInterval time.Duration }

func defaultDeploymentReadinessWaitOptions() deploymentReadinessWaitOptions {
	return deploymentReadinessWaitOptions{timeout: deploymentReadinessWaitTimeout, pollInterval: deploymentReadinessPollInterval}
}
func waitForDeploymentReady(ctx context.Context, clientset kubernetes.Interface, namespace, name string, opts deploymentReadinessWaitOptions) error {
	if opts.timeout <= 0 || opts.pollInterval <= 0 {
		return fmt.Errorf(`wait for deployment %s/%s: invalid wait options`, namespace, name)
	}
	observation := `not ready`
	check := func() bool {
		ready, reason, err := DeploymentReadiness(ctx, clientset, namespace, name)
		if err != nil {
			observation = `readiness check temporarily failed`
			return false
		}
		if ready {
			return true
		}
		if reason != `` {
			observation = reason
		}
		return false
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf(`wait for deployment %s/%s to become ready: %w`, namespace, name, err)
	}
	if check() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf(`wait for deployment %s/%s to become ready: %w`, namespace, name, err)
	}
	timer := time.NewTimer(opts.timeout)
	defer timer.Stop()
	ticker := time.NewTicker(opts.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf(`wait for deployment %s/%s to become ready: %w`, namespace, name, ctx.Err())
		case <-timer.C:
			return fmt.Errorf(`timed out waiting for deployment %s/%s to become ready: %s`, namespace, name, observation)
		case <-ticker.C:
			if check() {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return fmt.Errorf(`wait for deployment %s/%s to become ready: %w`, namespace, name, err)
			}
		}
	}
}
func reconcileDeploymentDatabaseCredentials(ctx context.Context, clientset kubernetes.Interface, sourceNamespace, tenantNamespace string, options ...deploymentReadinessWaitOptions) error {
	waitOpts := defaultDeploymentReadinessWaitOptions()
	if len(options) > 0 {
		waitOpts = options[0]
	}
	if err := waitForDeploymentReady(ctx, clientset, sourceNamespace, deploymentDatabaseName, waitOpts); err != nil {
		return fmt.Errorf(`deployment database %s/%s is not ready: %w`, sourceNamespace, deploymentDatabaseName, err)
	}
	if err := copyDeploymentDatabaseCredentials(ctx, clientset, sourceNamespace, tenantNamespace); err != nil {
		return fmt.Errorf(`copy deployment database credentials to %s: %w`, tenantNamespace, err)
	}
	return nil
}
