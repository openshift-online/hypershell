package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type cnpgDatabaseReconciler struct {
	cnpg CNPGConfig
}

func (r *cnpgDatabaseReconciler) Reconcile(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, tenantNamespace, gatewayID, rotateAnnotation string) error {
	if err := reconcileCNPGDatabaseResources(ctx, dynamicClient, clientset, tenantNamespace, gatewayID, r.cnpg); err != nil {
		return fmt.Errorf("reconcile CNPG database resources in %s: %w", tenantNamespace, err)
	}
	if rotateAnnotation != "" {
		if err := rotateCNPGDatabaseCredentials(ctx, clientset, tenantNamespace, gatewayID, r.cnpg, rotateAnnotation); err != nil {
			return fmt.Errorf("rotate database credentials in %s: %w", tenantNamespace, err)
		}
	}
	return nil
}

func (r *cnpgDatabaseReconciler) Delete(ctx context.Context, dynamicClient dynamic.Interface, clientset kubernetes.Interface, gatewayID string) error {
	if gatewayID == "" {
		return nil
	}
	if r.cnpg.ClusterNamespace == "" {
		log.Printf("WARN gateway %s: CNPG cluster namespace unknown; Database, DatabaseRole, and password Secret were not deleted and may require manual cleanup", gatewayID)
		return nil
	}
	deleteCNPGResources(ctx, dynamicClient, clientset, gatewayID, r.cnpg)
	return nil
}

func cnpgResourceName(gatewayID string) string {
	return "gw-" + strings.ToLower(gatewayID)
}

func cnpgPGName(gatewayID string) string {
	return "gw_" + strings.ToLower(gatewayID)
}

func reconcileCNPGDatabaseResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset kubernetes.Interface,
	tenantNamespace string,
	gatewayID string,
	cnpg CNPGConfig,
) error {
	crName := cnpgResourceName(gatewayID)
	pgName := cnpgPGName(gatewayID)
	passwordSecretName := crName + "-credentials"

	log.Printf("INFO CNPG provisioning: gateway=%s cr=%s db=%s cluster=%s/%s tenant=%s",
		gatewayID, crName, pgName, cnpg.ClusterNamespace, cnpg.ClusterName, tenantNamespace)

	_, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Get(ctx, passwordSecretName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get CNPG password secret: %w", err)
		}

		passwordBytes := make([]byte, 32)
		if _, err := rand.Read(passwordBytes); err != nil {
			return fmt.Errorf("generate database password: %w", err)
		}
		password := hex.EncodeToString(passwordBytes)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      passwordSecretName,
				Namespace: cnpg.ClusterNamespace,
				Labels: map[string]string{
					"cnpg.io/reload":                         "true",
					"hypershell.redhat.io/managed":           "true",
					"hypershell.redhat.io/gateway-namespace": tenantNamespace,
				},
			},
			Type: corev1.SecretTypeBasicAuth,
			StringData: map[string]string{
				"username": pgName,
				"password": password,
			},
		}
		if _, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create CNPG password secret: %w", err)
		}
		log.Printf("INFO created CNPG password secret %s in %s", passwordSecretName, cnpg.ClusterNamespace)
	} else {
		log.Printf("DEBUG CNPG password secret %s already exists in %s, skipping creation", passwordSecretName, cnpg.ClusterNamespace)
	}

	log.Printf("INFO reconciling CNPG DatabaseRole %s in %s (cluster=%s)", crName, cnpg.ClusterNamespace, cnpg.ClusterName)
	role := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "DatabaseRole",
			"metadata": map[string]interface{}{
				"name":      crName,
				"namespace": cnpg.ClusterNamespace,
				"labels": map[string]interface{}{
					"hypershell.redhat.io/managed":           "true",
					"hypershell.redhat.io/gateway-namespace": tenantNamespace,
				},
			},
			"spec": map[string]interface{}{
				"cluster": map[string]interface{}{
					"name": cnpg.ClusterName,
				},
				"name":  pgName,
				"login": true,
				"passwordSecret": map[string]interface{}{
					"name": passwordSecretName,
				},
				"databaseRoleReclaimPolicy": "delete",
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, role); err != nil {
		return fmt.Errorf("reconcile CNPG DatabaseRole: %w", err)
	}

	log.Printf("INFO reconciling CNPG Database %s in %s (owner=%s)", crName, cnpg.ClusterNamespace, pgName)
	db := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      crName,
				"namespace": cnpg.ClusterNamespace,
				"labels": map[string]interface{}{
					"hypershell.redhat.io/managed":           "true",
					"hypershell.redhat.io/gateway-namespace": tenantNamespace,
				},
			},
			"spec": map[string]interface{}{
				"cluster": map[string]interface{}{
					"name": cnpg.ClusterName,
				},
				"name":                  pgName,
				"owner":                 pgName,
				"databaseReclaimPolicy": "delete",
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, db); err != nil {
		return fmt.Errorf("reconcile CNPG Database: %w", err)
	}

	log.Printf("INFO waiting for CNPG Database %s/%s to become ready (timeout=2m)", cnpg.ClusterNamespace, crName)
	if err := waitForCNPGDatabase(ctx, dynamicClient, cnpg.ClusterNamespace, crName, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for CNPG database: %w", err)
	}

	gwSecretName := "openshell-gateway-db-credentials"
	_, err = clientset.CoreV1().Secrets(tenantNamespace).Get(ctx, gwSecretName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get gateway credentials secret: %w", err)
		}

		cnpgSecret, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Get(ctx, passwordSecretName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("read CNPG password secret: %w", err)
		}
		passwordBytes, ok := cnpgSecret.Data["password"]
		if !ok || len(passwordBytes) == 0 {
			return fmt.Errorf("CNPG password secret %s/%s has no password key", cnpg.ClusterNamespace, passwordSecretName)
		}
		password := string(passwordBytes)

		host := fmt.Sprintf("%s-rw.%s.svc.cluster.local", cnpg.ClusterName, cnpg.ClusterNamespace)
		dbURI := fmt.Sprintf("postgresql://%s:%s@%s:5432/%s?sslmode=require",
			pgName, url.QueryEscape(password), host, pgName)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gwSecretName,
				Namespace: tenantNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "database",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"host":     host,
				"port":     "5432",
				"dbname":   pgName,
				"user":     pgName,
				"password": password,
				"uri":      dbURI,
			},
		}
		if _, err := clientset.CoreV1().Secrets(tenantNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create gateway credentials secret: %w", err)
		}
		log.Printf("INFO created gateway credentials secret %s in %s (host=%s db=%s)", gwSecretName, tenantNamespace, host, pgName)
	} else {
		log.Printf("DEBUG gateway credentials secret %s already exists in %s, skipping creation", gwSecretName, tenantNamespace)
	}

	log.Printf("INFO CNPG database provisioning complete for gateway %s in %s", gatewayID, tenantNamespace)
	return nil
}

func waitForCNPGDatabase(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string, timeout time.Duration) error {
	databaseGVR := schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "databases",
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for CNPG Database %s/%s to become ready", namespace, name)
		case <-ticker.C:
			obj, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					log.Printf("DEBUG CNPG Database %s/%s not found yet, waiting...", namespace, name)
				} else {
					log.Printf("WARN error checking CNPG Database %s/%s: %v", namespace, name, err)
				}
				continue
			}
			applied, _, _ := unstructured.NestedBool(obj.Object, "status", "applied")
			if applied {
				log.Printf("INFO CNPG Database %s/%s is ready (status.applied=true)", namespace, name)
				return nil
			}
			log.Printf("DEBUG CNPG Database %s/%s exists but not ready (status.applied=%v)", namespace, name, applied)
		}
	}
}

func deleteCNPGResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset kubernetes.Interface,
	gatewayID string,
	cnpg CNPGConfig,
) {
	crName := cnpgResourceName(gatewayID)
	ns := cnpg.ClusterNamespace
	log.Printf("INFO deleting CNPG resources for gateway %s: cr=%s namespace=%s", gatewayID, crName, ns)

	databaseGVR := schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "databases",
	}
	if err := dynamicClient.Resource(databaseGVR).Namespace(ns).Delete(ctx, crName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete CNPG Database %s: %v", crName, err)
		}
	} else {
		log.Printf("INFO deleted CNPG Database %s from %s", crName, ns)
	}

	roleGVR := schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "databaseroles",
	}
	if err := dynamicClient.Resource(roleGVR).Namespace(ns).Delete(ctx, crName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete CNPG DatabaseRole %s: %v", crName, err)
		}
	} else {
		log.Printf("INFO deleted CNPG DatabaseRole %s from %s", crName, ns)
	}

	passwordSecretName := crName + "-credentials"
	if err := clientset.CoreV1().Secrets(ns).Delete(ctx, passwordSecretName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete CNPG password secret %s: %v", passwordSecretName, err)
		}
	} else {
		log.Printf("INFO deleted CNPG password secret %s from %s", passwordSecretName, ns)
	}
}

func rotateCNPGDatabaseCredentials(
	ctx context.Context,
	clientset kubernetes.Interface,
	tenantNamespace string,
	gatewayID string,
	cnpg CNPGConfig,
	rotateTimestamp string,
) error {
	gwSecretName := "openshell-gateway-db-credentials"
	existing, err := clientset.CoreV1().Secrets(tenantNamespace).Get(ctx, gwSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get gateway credentials secret for rotation: %w", err)
	}

	lastRotation := existing.Annotations["hypershell.redhat.io/last-db-rotation"]
	if lastRotation == rotateTimestamp {
		log.Printf("DEBUG database credentials in %s already rotated at %s, skipping", tenantNamespace, rotateTimestamp)
		return nil
	}

	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return fmt.Errorf("generate new database password: %w", err)
	}
	newPassword := hex.EncodeToString(passwordBytes)

	crName := cnpgResourceName(gatewayID)
	pgName := cnpgPGName(gatewayID)
	passwordSecretName := crName + "-credentials"

	cnpgSecret, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Get(ctx, passwordSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get CNPG password secret for rotation: %w", err)
	}
	cnpgSecret.Data["password"] = []byte(newPassword)
	if _, err := clientset.CoreV1().Secrets(cnpg.ClusterNamespace).Update(ctx, cnpgSecret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update CNPG password secret: %w", err)
	}
	log.Printf("INFO updated CNPG password secret %s in %s", passwordSecretName, cnpg.ClusterNamespace)

	host := fmt.Sprintf("%s-rw.%s.svc.cluster.local", cnpg.ClusterName, cnpg.ClusterNamespace)
	newURI := fmt.Sprintf("postgresql://%s:%s@%s:5432/%s?sslmode=require",
		pgName, url.QueryEscape(newPassword), host, pgName)

	existing.Data["password"] = []byte(newPassword)
	existing.Data["uri"] = []byte(newURI)
	if existing.Annotations == nil {
		existing.Annotations = make(map[string]string)
	}
	existing.Annotations["hypershell.redhat.io/last-db-rotation"] = rotateTimestamp

	if _, err := clientset.CoreV1().Secrets(tenantNamespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update gateway credentials secret after rotation: %w", err)
	}

	log.Printf("INFO rotated database credentials in %s (timestamp=%s)", tenantNamespace, rotateTimestamp)
	return nil
}
