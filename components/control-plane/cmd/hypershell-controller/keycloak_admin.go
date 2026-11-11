package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
)

// keycloakAdminSecretName is the Secret, in the control-plane namespace, that
// holds the Keycloak admin client the control plane uses for every gateway.
const keycloakAdminSecretName = "hypershell-keycloak-admin"

// secretGetter is the one Secret read loadKeycloakConfig needs; a seam for tests.
type secretGetter interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Secret, error)
}

// keycloakConfigFromSecret builds the Keycloak admin config and requires every
// key. A partial Secret is an error, not a disabled integration.
func keycloakConfigFromSecret(secret *corev1.Secret) (*gateway.KeycloakConfig, error) {
	value := func(key string) string { return strings.TrimSpace(string(secret.Data[key])) }
	cfg := &gateway.KeycloakConfig{
		ServerURL:    value("server-url"),
		Realm:        value("realm"),
		ClientID:     value("client-id"),
		ClientSecret: value("client-secret"),
	}
	var missing []string
	for _, key := range []string{"server-url", "realm", "client-id", "client-secret"} {
		if value(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("secret %s is missing or has empty keys: %s", keycloakAdminSecretName, strings.Join(missing, ", "))
	}
	return cfg, nil
}

// loadKeycloakConfig reads the Keycloak admin Secret and retries with
// exponential backoff until it exists and is complete. Keycloak is mandatory:
// the control plane never runs with it disabled, and no gateway is deployed
// until this returns (OSG-KCLK-01.02). It returns an error only when ctx ends.
func loadKeycloakConfig(ctx context.Context, secrets secretGetter, namespace string, initialBackoff time.Duration) (*gateway.KeycloakConfig, error) {
	backoff := initialBackoff
	const maxBackoff = 30 * time.Second
	for {
		var cfg *gateway.KeycloakConfig
		secret, err := secrets.Get(ctx, keycloakAdminSecretName, metav1.GetOptions{})
		if err == nil {
			cfg, err = keycloakConfigFromSecret(secret)
		} else {
			err = fmt.Errorf("read secret %s/%s: %w", namespace, keycloakAdminSecretName, err)
		}
		if err == nil {
			return cfg, nil
		}

		slog.ErrorContext(ctx, "Keycloak integration is not configured; it is required, so no gateway is deployed until the Secret is valid; retrying",
			"secret", keycloakAdminSecretName,
			"namespace", namespace,
			"backoff", backoff,
			"error", err)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for Keycloak admin secret cancelled: %w", ctx.Err())
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}
