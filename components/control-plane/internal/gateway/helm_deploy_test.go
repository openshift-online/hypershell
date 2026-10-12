package gateway

import "testing"

// TestEffectiveCredentialDriver locks in the HYPERSHELL-369 rollout selector:
// an omitted credential_driver defaults to kek-db unless the deployment opts into
// vault via GATEWAY_DEFAULT_CREDENTIAL_DRIVER; an explicit driver is never changed.
func TestEffectiveCredentialDriver(t *testing.T) {
	t.Run("omitted defaults to kek-db (unset)", func(t *testing.T) {
		t.Setenv("GATEWAY_DEFAULT_CREDENTIAL_DRIVER", "")
		if got := effectiveCredentialDriver(nil); got != nil {
			t.Errorf("expected nil (kek-db default), got %+v", got)
		}
	})

	t.Run("omitted defaults to vault when deployment opts in", func(t *testing.T) {
		t.Setenv("GATEWAY_DEFAULT_CREDENTIAL_DRIVER", "vault")
		got := effectiveCredentialDriver(nil)
		if got == nil || got.Type != "vault" {
			t.Errorf("expected vault, got %+v", got)
		}
	})

	t.Run("unrecognized value falls back to kek-db", func(t *testing.T) {
		t.Setenv("GATEWAY_DEFAULT_CREDENTIAL_DRIVER", "s3")
		if got := effectiveCredentialDriver(nil); got != nil {
			t.Errorf("expected nil (kek-db) for unrecognized value, got %+v", got)
		}
	})

	t.Run("explicit driver is unchanged even when default is vault", func(t *testing.T) {
		t.Setenv("GATEWAY_DEFAULT_CREDENTIAL_DRIVER", "vault")
		in := &CredentialDriverConfig{Type: "kek-db"}
		if got := effectiveCredentialDriver(in); got != in {
			t.Errorf("explicit driver should be returned unchanged, got %+v", got)
		}
	})
}
