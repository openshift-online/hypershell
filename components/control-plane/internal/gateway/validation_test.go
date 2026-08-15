package gateway

import (
	"testing"
)

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
