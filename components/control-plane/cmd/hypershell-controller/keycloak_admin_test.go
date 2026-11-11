package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// scriptedSecrets returns the scripted results in order, then the last one.
type scriptedSecrets struct {
	results []func() (*corev1.Secret, error)
	calls   int
}

func (s *scriptedSecrets) Get(context.Context, string, metav1.GetOptions) (*corev1.Secret, error) {
	i := min(s.calls, len(s.results)-1)
	s.calls++
	return s.results[i]()
}

func kcSecret(data map[string]string) func() (*corev1.Secret, error) {
	return func() (*corev1.Secret, error) {
		secret := &corev1.Secret{Data: map[string][]byte{}}
		for k, v := range data {
			secret.Data[k] = []byte(v)
		}
		return secret, nil
	}
}

func notFound() (*corev1.Secret, error) {
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, keycloakAdminSecretName)
}

var completeKC = map[string]string{
	"server-url":    "https://keycloak.example",
	"realm":         "hypershell",
	"client-id":     "hypershell-controller",
	"client-secret": "s3cret",
}

// Keycloak is mandatory: a missing, unreadable or incomplete admin Secret is
// retried until it is valid, never treated as "Keycloak disabled". Keycloak
// itself is not contacted: the fake Secret reader is the only dependency.
// Verifies: OSG-KCLK-01.01, OSG-KCLK-01.02
func TestLoadKeycloakConfig(t *testing.T) {
	partial := map[string]string{"server-url": "https://keycloak.example", "realm": "hypershell", "client-id": " "}
	cases := []struct {
		name      string
		results   []func() (*corev1.Secret, error)
		wantCalls int
	}{
		{name: "present on first read", results: []func() (*corev1.Secret, error){kcSecret(completeKC)}, wantCalls: 1},
		{name: "missing then created", results: []func() (*corev1.Secret, error){notFound, notFound, kcSecret(completeKC)}, wantCalls: 3},
		{name: "incomplete then fixed", results: []func() (*corev1.Secret, error){kcSecret(partial), kcSecret(completeKC)}, wantCalls: 2},
		{
			name: "transient read error then present",
			results: []func() (*corev1.Secret, error){
				func() (*corev1.Secret, error) { return nil, errors.New("connection refused") },
				kcSecret(completeKC),
			},
			wantCalls: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			secrets := &scriptedSecrets{results: tc.results}
			cfg, err := loadKeycloakConfig(context.Background(), secrets, "hypershell-system", time.Millisecond)
			if err != nil {
				t.Fatalf("loadKeycloakConfig: %v", err)
			}
			if secrets.calls != tc.wantCalls {
				t.Errorf("calls = %d, want %d", secrets.calls, tc.wantCalls)
			}
			if cfg.ServerURL != completeKC["server-url"] || cfg.Realm != completeKC["realm"] ||
				cfg.ClientID != completeKC["client-id"] || cfg.ClientSecret != completeKC["client-secret"] {
				t.Errorf("config = %+v, want values from the complete secret", cfg)
			}
		})
	}
}

// Verifies: OSG-KCLK-01.02
func TestLoadKeycloakConfig_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	cfg, err := loadKeycloakConfig(ctx, &scriptedSecrets{results: []func() (*corev1.Secret, error){notFound}}, "ns", time.Millisecond)
	if err == nil || cfg != nil {
		t.Fatalf("got cfg=%v err=%v, want nil config and a cancellation error", cfg, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
}

func TestKeycloakConfigFromSecret_NamesMissingKeys(t *testing.T) {
	_, err := keycloakConfigFromSecret(&corev1.Secret{Data: map[string][]byte{"realm": []byte("hypershell")}})
	if err == nil {
		t.Fatal("want an error for an incomplete secret")
	}
	if !strings.Contains(err.Error(), "server-url, client-id, client-secret") {
		t.Errorf("err = %q, want it to name the missing keys in order", err)
	}
}
