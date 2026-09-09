package gateway

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func serverTLSClusterIssuer() string {
	return strings.TrimSpace(os.Getenv("GATEWAY_SERVER_TLS_CLUSTER_ISSUER"))
}

// ReconcileSandboxTLS keeps server trust separate from the gateway client CA.
// OpenShell reads all sandbox TLS material through one Secret. Only public CA
// data comes from the server Secret; its private key must never reach a sandbox.
func ReconcileSandboxTLS(ctx context.Context, client kubernetes.Interface, namespace string) error {
	issuer := serverTLSClusterIssuer()
	if issuer == "" {
		return nil
	}
	secrets := client.CoreV1().Secrets(namespace)
	server, err := secrets.Get(ctx, "openshell-server-tls", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read gateway server TLS: %w", err)
	}
	if server.Annotations["cert-manager.io/issuer-name"] != issuer || server.Annotations["cert-manager.io/issuer-kind"] != "ClusterIssuer" {
		return fmt.Errorf("gateway server certificate has not been issued by the configured ClusterIssuer")
	}
	clientTLS, err := secrets.Get(ctx, "openshell-client-tls", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read gateway client TLS: %w", err)
	}
	if len(server.Data["ca.crt"]) == 0 || len(clientTLS.Data[corev1.TLSCertKey]) == 0 || len(clientTLS.Data[corev1.TLSPrivateKeyKey]) == 0 || clientTLS.UID == "" {
		return fmt.Errorf("gateway TLS material is not ready")
	}
	controller := true
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshell-sandbox-tls", Namespace: namespace,
			Labels:          map[string]string{ManagedLabel: ManagedLabelValue, ManagedByLabel: ManagedByValue, "app.kubernetes.io/name": "openshell", "app.kubernetes.io/component": "gateway"},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Secret", Name: clientTLS.Name, UID: clientTLS.UID, Controller: &controller}},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{"ca.crt": bytes.Clone(server.Data["ca.crt"]), corev1.TLSCertKey: bytes.Clone(clientTLS.Data[corev1.TLSCertKey]), corev1.TLSPrivateKeyKey: bytes.Clone(clientTLS.Data[corev1.TLSPrivateKeyKey])},
	}
	current, err := secrets.Get(ctx, desired.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err := secrets.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create sandbox TLS payload: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read sandbox TLS payload: %w", err)
	}
	if reflect.DeepEqual(current.Data, desired.Data) && reflect.DeepEqual(current.OwnerReferences, desired.OwnerReferences) && reflect.DeepEqual(current.Labels, desired.Labels) && current.Type == desired.Type {
		return nil
	}
	desired.ResourceVersion = current.ResourceVersion
	if _, err := secrets.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update sandbox TLS payload: %w", err)
	}
	return nil
}
