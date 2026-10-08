package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	AccessToken string `json:"access_token,omitempty"`
	URL         string `json:"url,omitempty"`
	Insecure    bool   `json:"insecure,omitempty"`
	Pager       string `json:"pager,omitempty"`

	// OIDC session fields, set by 'login --issuer-url' and cleared by logout and
	// by a static-token login.
	RefreshToken string `json:"refresh_token,omitempty"`
	IssuerURL    string `json:"issuer_url,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	// ExpiresAt is the Unix time the access token expires, from the token
	// response; it covers access tokens that are not JWTs.
	ExpiresAt int64 `json:"expires_at,omitempty"`
}

// Store persists the CLI configuration. Implement it to keep configuration
// somewhere else, or to store additional fields next to the generated ones.
type Store interface {
	Load() (*Config, error)
	Save(*Config) error
}

var (
	storeMu sync.RWMutex
	store   Store
)

// SetStore registers the Store used by Load and Save, and therefore by every
// generated command. Call it once at startup, for example from the
// hand-maintained main.go. Passing nil restores the default file-backed
// behavior. Location is not affected.
func SetStore(s Store) {
	storeMu.Lock()
	defer storeMu.Unlock()
	store = s
}

func registeredStore() Store {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return store
}

// Load returns the configuration from the registered Store, or from the
// default configuration file when none is registered.
func Load() (*Config, error) {
	if s := registeredStore(); s != nil {
		return s.Load()
	}
	return loadFile()
}

// Save persists the configuration through the registered Store, or to the
// default configuration file when none is registered.
func Save(cfg *Config) error {
	if s := registeredStore(); s != nil {
		return s.Save(cfg)
	}
	return saveFile(cfg)
}

func loadFile() (cfg *Config, err error) {
	file, err := Location()
	if err != nil {
		return
	}
	_, err = os.Stat(file)
	if os.IsNotExist(err) {
		cfg = &Config{}
		err = nil
		return
	}
	if err != nil {
		err = fmt.Errorf("can't check if config file '%s' exists: %v", file, err)
		return
	}
	data, err := os.ReadFile(file)
	if err != nil {
		err = fmt.Errorf("can't read config file '%s': %v", file, err)
		return
	}
	cfg = &Config{}
	if len(data) == 0 {
		return
	}
	err = json.Unmarshal(data, cfg)
	if err != nil {
		err = fmt.Errorf("can't parse config file '%s': %v", file, err)
		return
	}
	return
}

func saveFile(cfg *Config) error {
	file, err := Location()
	if err != nil {
		return err
	}
	dir := filepath.Dir(file)
	err = os.MkdirAll(dir, os.FileMode(0700))
	if err != nil {
		return fmt.Errorf("can't create directory %s: %v", dir, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("can't marshal config: %v", err)
	}
	err = os.WriteFile(file, data, 0600)
	if err != nil {
		return fmt.Errorf("can't write file '%s': %v", file, err)
	}
	// WriteFile keeps the mode of an existing file, and this file holds credentials.
	if err := os.Chmod(file, 0600); err != nil {
		return fmt.Errorf("can't restrict permissions of '%s': %v", file, err)
	}
	return nil
}

func Location() (path string, err error) {
	if envConfig := os.Getenv("HYPERSHELL_CONFIG"); envConfig != "" {
		return envConfig, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	path = filepath.Join(home, ".hypershell.json")

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return path, err
		}
		path = filepath.Join(configDir, "hypershell", "config.json")
	}

	return path, nil
}

func (c *Config) Armed() (armed bool, reason string) {
	if c.URL == "" {
		reason = "server URL isn't set"
		return
	}
	if c.AccessToken == "" {
		reason = "access token isn't set"
		return
	}
	armed = true
	return
}

func (c *Config) Disarm() {
	c.AccessToken = ""
	c.URL = ""
	c.Insecure = false
	c.ClearOIDC()
}

// ClearOIDC removes the OIDC session fields.
func (c *Config) ClearOIDC() {
	c.RefreshToken = ""
	c.IssuerURL = ""
	c.ClientID = ""
	c.ExpiresAt = 0
}

// CanRefresh reports whether the configuration holds what is needed to renew
// the access token.
func (c *Config) CanRefresh() bool {
	return c.RefreshToken != "" && c.IssuerURL != "" && c.ClientID != ""
}

// AccessTokenExpiring reports whether the access token is expired or expires
// within margin. A token with no known expiry is never reported as expiring.
func (c *Config) AccessTokenExpiring(margin time.Duration) bool {
	if c.AccessToken == "" {
		return true
	}
	if exp, ok := tokenExpiry(c.AccessToken); ok {
		return time.Until(exp) < margin
	}
	if c.ExpiresAt > 0 {
		return time.Until(time.Unix(c.ExpiresAt, 0)) < margin
	}
	return false
}
