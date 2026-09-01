package config

import (
	"fmt"
	"os"

	"github.com/openshift-online/hypershell/components/cli/pkg/auth"
)

// EnsureFreshToken refreshes the access token if it is expired and a refresh
// token is available. The Config is updated in place and persisted. Save
// failures are logged to stderr but not returned as errors.
func EnsureFreshToken(cfg *Config) error {
	if cfg.RefreshToken == "" || cfg.IssuerURL == "" || cfg.ClientID == "" {
		return nil
	}
	expired, checkErr := TokenExpired(cfg.AccessToken)
	if checkErr == nil && !expired {
		return nil
	}
	tr, refreshErr := auth.Refresh(cfg.IssuerURL, cfg.ClientID, cfg.RefreshToken, cfg.Insecure)
	if refreshErr != nil {
		return fmt.Errorf("session expired and token refresh failed: %w - run 'hsctl login' to authenticate", refreshErr)
	}
	cfg.AccessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		cfg.RefreshToken = tr.RefreshToken
	}
	if saveErr := Save(cfg); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not persist refreshed token: %v\n", saveErr)
	}
	return nil
}
