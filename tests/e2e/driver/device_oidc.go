package driver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (d *kindDriver) ValidateGatewayDeviceAuthorization(ctx context.Context, clientID string) error {
	return validateGatewayDeviceAuthorization(ctx, d.httpClient, d.oidcIssuer, clientID)
}

func (d *openshiftDriver) ValidateGatewayDeviceAuthorization(ctx context.Context, clientID string) error {
	return validateGatewayDeviceAuthorization(ctx, d.httpClient, d.oidcIssuer, clientID)
}

func validateGatewayDeviceAuthorization(ctx context.Context, client *http.Client, issuer, clientID string) error {
	discoveryURL := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	discoveryRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return fmt.Errorf("build OIDC discovery request: %w", err)
	}
	discoveryResponse, err := client.Do(discoveryRequest)
	if err != nil {
		return fmt.Errorf("OIDC discovery: %w", err)
	}
	discoveryBody, readErr := io.ReadAll(io.LimitReader(discoveryResponse.Body, 1<<20))
	_ = discoveryResponse.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read OIDC discovery: %w", readErr)
	}
	if discoveryResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("OIDC discovery returned status %d", discoveryResponse.StatusCode)
	}
	var discovery struct {
		DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
		TokenEndpoint               string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(discoveryBody, &discovery); err != nil {
		return fmt.Errorf("decode OIDC discovery: %w", err)
	}
	if discovery.DeviceAuthorizationEndpoint == "" || discovery.TokenEndpoint == "" {
		return fmt.Errorf("OIDC discovery does not advertise device and token endpoints")
	}

	verifierBytes := make([]byte, 48)
	if _, err := rand.Read(verifierBytes); err != nil {
		return fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	deviceResponse, statusCode, err := postOIDCForm(ctx, client, discovery.DeviceAuthorizationEndpoint, url.Values{
		"client_id":             {clientID},
		"scope":                 {"openid"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	})
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("device authorization returned status %d", statusCode)
	}
	var device struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Interval        int    `json:"interval"`
	}
	if err := json.Unmarshal(deviceResponse, &device); err != nil {
		return fmt.Errorf("decode device authorization: %w", err)
	}
	if device.DeviceCode == "" || device.UserCode == "" || device.VerificationURI == "" {
		return fmt.Errorf("device authorization response is incomplete")
	}
	if device.Interval < 0 || device.Interval > 30 {
		return fmt.Errorf("device authorization polling interval %d is invalid", device.Interval)
	}
	if device.Interval > 0 {
		timer := time.NewTimer(time.Duration(device.Interval) * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	tokenResponse, statusCode, err := postOIDCForm(ctx, client, discovery.TokenEndpoint, url.Values{
		"grant_type":    {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":     {clientID},
		"device_code":   {device.DeviceCode},
		"code_verifier": {verifier},
	})
	if err != nil {
		return err
	}
	var tokenError struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(tokenResponse, &tokenError); err != nil {
		return fmt.Errorf("decode device token response: %w", err)
	}
	if statusCode != http.StatusBadRequest || tokenError.Error != "authorization_pending" {
		return fmt.Errorf("device token poll returned status %s error %q, want authorization_pending", strconv.Itoa(statusCode), tokenError.Error)
	}
	return nil
}

func postOIDCForm(ctx context.Context, client *http.Client, endpoint string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("build OIDC form request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("OIDC form request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("read OIDC form response: %w", err)
	}
	return body, response.StatusCode, nil
}
