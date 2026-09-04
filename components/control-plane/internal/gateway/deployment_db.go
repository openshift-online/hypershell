package gateway

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type deploymentDatabaseReconciler struct {
	dbNamespace string
}

func (r *deploymentDatabaseReconciler) Reconcile(ctx context.Context, _ dynamic.Interface, clientset kubernetes.Interface, tenantNamespace, _, _ string) error {
	return reconcileDeploymentDatabaseCredentials(ctx, clientset, r.dbNamespace, tenantNamespace)
}

func (r *deploymentDatabaseReconciler) Delete(_ context.Context, _ dynamic.Interface, _ kubernetes.Interface, _ string) {
}

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

func copyDeploymentDatabaseCredentials(
	ctx context.Context,
	clientset kubernetes.Interface,
	sourceNamespace string,
	tenantNamespace string,
) error {
	const (
		sourceSecretName = "openshell-db-credentials"
		gwSecretName     = "openshell-gateway-db-credentials"
	)

	sourceSecret, err := clientset.CoreV1().Secrets(sourceNamespace).Get(ctx, sourceSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read source database credentials from %s/%s: %w", sourceNamespace, sourceSecretName, err)
	}

	required := map[string]string{}
	for _, key := range []string{"dbname", "user", "password"} {
		value := string(sourceSecret.Data[key])
		if value == "" {
			return fmt.Errorf("source database credentials %s/%s is missing required key %q", sourceNamespace, sourceSecretName, key)
		}
		required[key] = value
	}

	host := fmt.Sprintf("openshell-gateway-db.%s.svc.cluster.local", sourceNamespace)
	port := "5432"
	dbURI := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		required["user"], url.QueryEscape(required["password"]), host, port, required["dbname"])
	desiredData := map[string][]byte{
		"host":     []byte(host),
		"port":     []byte(port),
		"dbname":   []byte(required["dbname"]),
		"user":     []byte(required["user"]),
		"password": []byte(required["password"]),
		"uri":      []byte(dbURI),
	}
	desiredLabels := map[string]string{
		"app.kubernetes.io/name":       "openshell",
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/managed-by": "hypershell-control-plane",
		"hypershell.redhat.io/managed": "true",
	}

	secrets := clientset.CoreV1().Secrets(tenantNamespace)
	existing, err := secrets.Get(ctx, gwSecretName, metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, err)
	}
	if k8serrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: gwSecretName, Namespace: tenantNamespace, Labels: desiredLabels},
			Type:       corev1.SecretTypeOpaque,
			Data:       desiredData,
		}
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, err)
		}
		log.Printf("INFO copied deployment database credentials to %s (host=%s db=%s)", tenantNamespace, host, required["dbname"])
		return nil
	}

	updated := existing.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	for key, value := range desiredLabels {
		updated.Labels[key] = value
	}
	updated.Type = corev1.SecretTypeOpaque
	updated.Data = desiredData
	if reflect.DeepEqual(existing.Labels, updated.Labels) && existing.Type == updated.Type && reflect.DeepEqual(existing.Data, updated.Data) {
		return nil
	}
	if _, err := secrets.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, err)
	}
	log.Printf("INFO updated deployment database credentials in %s (host=%s db=%s)", tenantNamespace, host, required["dbname"])
	return nil
}
