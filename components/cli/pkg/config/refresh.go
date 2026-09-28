package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/openshift-online/hypershell/components/cli/pkg/auth"
)

// ErrSessionExpired is wrapped by the error returned when the access token has
// expired and the refresh token can no longer renew it.
var ErrSessionExpired = errors.New("session expired")

// EnsureFreshToken refreshes the access token if it is expired and a refresh
// token is available. The Config is updated in place and persisted. Save
// failures are logged to stderr but not returned as errors.
func EnsureFreshToken(cfg *Config) error {
	saveErr, err := refreshToken(cfg)
	if saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not persist refreshed token: %v\n", saveErr)
	}
	return err
}

// EnsureFreshTokenQuietly behaves like EnsureFreshToken but never writes to the
// terminal, for callers that own the screen. A failure to persist the refreshed
// token is ignored: the in-memory Config still holds the new token.
func EnsureFreshTokenQuietly(cfg *Config) error {
	_, err := refreshToken(cfg)
	return err
}

func refreshToken(cfg *Config) (saveErr, err error) {
	if cfg.RefreshToken == "" || cfg.IssuerURL == "" || cfg.ClientID == "" {
		return nil, nil
	}
	expired, checkErr := TokenExpired(cfg.AccessToken)
	if checkErr == nil && !expired {
		return nil, nil
	}
	tr, refreshErr := auth.Refresh(cfg.IssuerURL, cfg.ClientID, cfg.RefreshToken, cfg.Insecure)
	if refreshErr != nil {
		return nil, fmt.Errorf("%w and token refresh failed: %v - run 'hsctl login' to authenticate", ErrSessionExpired, refreshErr)
	}
	cfg.AccessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		cfg.RefreshToken = tr.RefreshToken
	}
	return Save(cfg), nil
}
