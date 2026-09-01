package auth

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
}

func tokenEndpoint(issuerURL string) string {
	return strings.TrimRight(issuerURL, "/") + "/protocol/openid-connect/token"
}

func newHTTPClient(insecure bool) *http.Client {
	if !insecure {
		return &http.Client{Timeout: 30 * time.Second}
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, //nolint:gosec
			},
		},
	}
}

func Refresh(issuerURL, clientID, refreshToken string, insecure bool) (TokenResponse, error) {
	resp, err := newHTTPClient(insecure).PostForm(tokenEndpoint(issuerURL), url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return TokenResponse{}, fmt.Errorf("token refresh: %w", err)
	}
	defer resp.Body.Close()
	return parseTokenResponse(resp)
}

func Revoke(issuerURL, clientID, token string, insecure bool) error {
	revokeURL := strings.TrimRight(issuerURL, "/") + "/protocol/openid-connect/revoke"
	resp, err := newHTTPClient(insecure).PostForm(revokeURL, url.Values{
		"client_id": {clientID},
		"token":     {token},
	})
	if err != nil {
		return fmt.Errorf("token revocation request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("revocation failed (%d): %s", resp.StatusCode, body)
	}
	return nil
}

func parseTokenResponse(resp *http.Response) (TokenResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return TokenResponse{}, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, body)
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return TokenResponse{}, fmt.Errorf("parsing token response: %w", err)
	}
	return tr, nil
}
