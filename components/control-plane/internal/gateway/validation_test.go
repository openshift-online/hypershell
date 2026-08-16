package gateway

import (
	"testing"
)

func TestValidateImageReference(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{name: "empty is invalid", ref: "", wantErr: true},
		{name: "bare name with tag", ref: "postgres:18", wantErr: false},
		{name: "docker hub library path", ref: "docker.io/library/postgres:18", wantErr: false},
		{name: "ghcr multi-segment path with tag", ref: "ghcr.io/nvidia/openshell/gateway:0.0.101", wantErr: false},
		{name: "quay long path with tag", ref: "quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-api-server-main:dev", wantErr: false},
		{name: "digest reference", ref: "registry.redhat.io/rhel9/postgresql-16@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: false},
		// In-cluster registry service address carries an explicit port; this is
		// the address the kubelet pulls mirrored images by on ROKS.
		{name: "internal registry host with port", ref: "image-registry.openshift-image-registry.svc:5000/openshift/openshell-gateway:0.0.101", wantErr: false},
		{name: "internal registry host with port, no tag", ref: "image-registry.openshift-image-registry.svc:5000/openshift/postgres", wantErr: false},
		{name: "localhost registry with port", ref: "localhost:5000/hypershell-controller:dev", wantErr: false},
		{name: "shell metacharacter rejected", ref: "postgres:18;rm -rf /", wantErr: true},
		{name: "command substitution rejected", ref: "postgres:$(whoami)", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageReference(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateImageReference(%q) error = %v, wantErr %v", tt.ref, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCredentialDriverConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *CredentialDriverConfig
		wantErr bool
	}{
		{
			name:    "nil config is valid",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "kubernetes-secrets is valid",
			config:  &CredentialDriverConfig{Type: "kubernetes-secrets"},
			wantErr: false,
		},
		{
			name: "kubernetes-secrets with namespace is valid",
			config: &CredentialDriverConfig{
				Type:              "kubernetes-secrets",
				KubernetesSecrets: &KubernetesSecretsConfig{Namespace: "my-ns"},
			},
			wantErr: false,
		},
		{
			name: "vault with address and role is valid",
			config: &CredentialDriverConfig{
				Type: "vault",
				Vault: &VaultCredentialConfig{
					Address: "https://vault.example.com",
					Role:    "gw-role",
				},
			},
			wantErr: false,
		},
		{
			name: "vault with all fields is valid",
			config: &CredentialDriverConfig{
				Type: "vault",
				Vault: &VaultCredentialConfig{
					Address:             "https://vault.example.com",
					Mount:               "secret",
					AuthMethod:          "kubernetes",
					Role:                "gw-role",
					KubernetesAuthMount: "kubernetes",
					TimeoutSecs:         30,
				},
			},
			wantErr: false,
		},
		{
			name:    "vault without config is invalid",
			config:  &CredentialDriverConfig{Type: "vault"},
			wantErr: true,
		},
		{
			name: "vault without address is invalid",
			config: &CredentialDriverConfig{
				Type:  "vault",
				Vault: &VaultCredentialConfig{Role: "gw-role"},
			},
			wantErr: true,
		},
		{
			name: "vault without role is invalid",
			config: &CredentialDriverConfig{
				Type:  "vault",
				Vault: &VaultCredentialConfig{Address: "https://vault.example.com"},
			},
			wantErr: true,
		},
		{
			name:    "unsupported type is invalid",
			config:  &CredentialDriverConfig{Type: "aws-secrets-manager"},
			wantErr: true,
		},
		{
			name:    "empty type is invalid",
			config:  &CredentialDriverConfig{Type: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCredentialDriverConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCredentialDriverConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
