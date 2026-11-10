package defaultworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (m *Manager) accessToken(ctx context.Context, issuer string, admin *bootstrapAdmin) (string, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {admin.ClientID},
		"client_secret": {admin.ClientSecret},
	}
	tokenEndpoint := strings.TrimRight(issuer, "/") + "/protocol/openid-connect/token"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("build gateway bootstrap-admin token request")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", errors.New("call gateway bootstrap-admin token endpoint")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("gateway bootstrap-admin token endpoint returned status %d", response.StatusCode)
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil || result.AccessToken == "" || result.RefreshToken != "" {
		return "", errors.New("parse gateway bootstrap-admin token response")
	}
	return result.AccessToken, nil
}

func errInvalidGatewayEndpoint() error {
	return errors.New("gateway endpoint must be an absolute http or https URL")
}
