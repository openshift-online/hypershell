package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type deviceTokenError struct {
	Code        string
	Description string
}

func (e *deviceTokenError) Error() string {
	if e.Description != "" {
		return e.Code + ": " + e.Description
	}
	return e.Code
}

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DeviceFlow performs an OAuth2 device authorization grant.
// It prints a verification URL and user code, then polls until the user
// completes authentication or the code expires.
func DeviceFlow(issuerURL, clientID string, insecure bool) (TokenResponse, error) {
	deviceURL := strings.TrimRight(issuerURL, "/") + "/protocol/openid-connect/auth/device"
	client := newHTTPClient(insecure)

	resp, err := client.PostForm(deviceURL, url.Values{
		"client_id": {clientID},
		"scope":     {"openid email profile"},
	})
	if err != nil {
		return TokenResponse{}, fmt.Errorf("device authorization request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("reading device auth response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return TokenResponse{}, fmt.Errorf("device authorization failed (%d): %s", resp.StatusCode, body)
	}

	var dar deviceAuthResponse
	if err := json.Unmarshal(body, &dar); err != nil {
		return TokenResponse{}, fmt.Errorf("parsing device auth response: %w", err)
	}

	if dar.VerificationURIComplete != "" {
		fmt.Fprintf(os.Stderr, "\nTo complete login, open:\n  %s\n\n", dar.VerificationURIComplete)
	} else {
		fmt.Fprintf(os.Stderr, "\nGo to %s and enter code: %s\n\n", dar.VerificationURI, dar.UserCode)
	}
	fmt.Fprintf(os.Stderr, "Waiting for authentication")

	interval := time.Duration(dar.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dar.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(interval)
		fmt.Fprintf(os.Stderr, ".")

		tr, err := pollDeviceToken(client, issuerURL, clientID, dar.DeviceCode)
		if err == nil {
			fmt.Fprintf(os.Stderr, "\n")
			return tr, nil
		}

		var tokenErr *deviceTokenError
		if errors.As(err, &tokenErr) {
			if tokenErr.Code == "authorization_pending" {
				continue
			}
			if tokenErr.Code == "slow_down" {
				interval += 5 * time.Second
				continue
			}
		}

		fmt.Fprintf(os.Stderr, "\n")
		return TokenResponse{}, err
	}

	return TokenResponse{}, fmt.Errorf("device authorization timed out")
}

func pollDeviceToken(client *http.Client, issuerURL, clientID, deviceCode string) (TokenResponse, error) {
	resp, err := client.PostForm(tokenEndpoint(issuerURL), url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {clientID},
		"device_code": {deviceCode},
	})
	if err != nil {
		return TokenResponse{}, fmt.Errorf("device token poll: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error != "" {
			return TokenResponse{}, &deviceTokenError{Code: errResp.Error, Description: errResp.ErrorDescription}
		}
		return TokenResponse{}, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, body)
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return TokenResponse{}, fmt.Errorf("parsing token response: %w", err)
	}
	return tr, nil
}
