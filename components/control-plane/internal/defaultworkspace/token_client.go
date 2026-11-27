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
	"time"
)

const tokenCacheSkew = 30 * time.Second

type cachedAccessToken struct {
	token     string
	expiresAt time.Time
}

func (m *Manager) accessToken(ctx context.Context, issuer string, admin *bootstrapAdmin) (string, error) {
	cacheKey := admin.ClientUUID
	if cached, ok := m.tokenCache.Load(cacheKey); ok {
		token := cached.(*cachedAccessToken)
		if time.Now().Before(token.expiresAt) {
			return token.token, nil
		}
		m.tokenCache.Delete(cacheKey)
	}

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
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil || result.AccessToken == "" || result.RefreshToken != "" {
		return "", errors.New("parse gateway bootstrap-admin token response")
	}
	if lifetime := time.Duration(result.ExpiresIn) * time.Second; lifetime > tokenCacheSkew {
		m.tokenCache.Store(cacheKey, &cachedAccessToken{
			token:     result.AccessToken,
			expiresAt: time.Now().Add(lifetime - tokenCacheSkew),
		})
	}
	return result.AccessToken, nil
}

func errInvalidGatewayEndpoint() error {
	return errors.New("gateway endpoint must be an absolute http or https URL")
}
