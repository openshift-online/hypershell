package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// tokenResponse is the subset of an OIDC token endpoint response the suite reads.
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

const (
	grantPassword          = "password"
	grantClientCredentials = "client_credentials"
	grantTokenExchange     = "urn:ietf:params:oauth:grant-type:token-exchange"
	tokenTypeAccessToken   = "urn:ietf:params:oauth:token-type:access_token"
)

func (d *kindDriver) tokenEndpoint() string {
	return strings.TrimSuffix(d.oidcIssuer, "/") + "/protocol/openid-connect/token"
}

// keycloakBaseAndRealm splits the issuer "<base>/realms/<realm>" into its base URL
// and realm name, mirroring the Bash driver's _kc_base / _kc_realm.
func (d *kindDriver) keycloakBaseAndRealm() (base, realm string) {
	const marker = "/realms/"
	issuer := strings.TrimSuffix(d.oidcIssuer, "/")
	if idx := strings.LastIndex(issuer, marker); idx >= 0 {
		return issuer[:idx], issuer[idx+len(marker):]
	}
	return issuer, "hypershell"
}

// postToken issues a form-encoded token request and returns the access token.
func (d *kindDriver) postToken(ctx context.Context, endpoint string, fields map[string]string) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(escapeForm(fields)))
	if err != nil {
		return Token{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("token request to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Token{}, fmt.Errorf("parse token response (status %d): %w", resp.StatusCode, err)
	}
	if tr.AccessToken == "" {
		msg := tr.ErrorDescription
		if msg == "" {
			msg = tr.Error
		}
		return Token{}, fmt.Errorf("token request failed (status %d): %s", resp.StatusCode, msg)
	}
	return Token{AccessToken: tr.AccessToken}, nil
}

// AcquireOIDCToken obtains an access token for user. The grant is selected by
// E2E_OIDC_GRANT: password (Kind default, resource-owner against the frontend
// client) or client_credentials (the hypershell-e2e service account, for
// GitHub-brokered pull-request environments).
func (d *kindDriver) AcquireOIDCToken(ctx context.Context, user Credentials) (Token, error) {
	grant := envOr("E2E_OIDC_GRANT", grantPassword)
	switch grant {
	case grantPassword:
		return d.postToken(ctx, d.tokenEndpoint(), map[string]string{
			"grant_type": grantPassword,
			"client_id":  d.frontendID,
			"username":   user.Username,
			"password":   user.Password,
		})
	case grantClientCredentials:
		return d.clientCredentialsToken(ctx)
	default:
		return Token{}, fmt.Errorf("unsupported E2E_OIDC_GRANT %q", grant)
	}
}

// AcquireClientCredentialsToken mints a token via the client-credentials grant for
// a confidential client (for example the control-plane registrar).
func (d *kindDriver) AcquireClientCredentialsToken(ctx context.Context, clientID, clientSecret string) (Token, error) {
	return d.postToken(ctx, d.tokenEndpoint(), map[string]string{
		"grant_type":    grantClientCredentials,
		"client_id":     clientID,
		"client_secret": clientSecret,
	})
}

// clientCredentialsToken acquires an admin API token via the hypershell-e2e
// service-account client-credentials grant.
func (d *kindDriver) clientCredentialsToken(ctx context.Context) (Token, error) {
	saClientID := os.Getenv("E2E_OIDC_SA_CLIENT_ID")
	saSecret := os.Getenv("E2E_OIDC_SA_CLIENT_SECRET")
	if saClientID == "" || saSecret == "" {
		return Token{}, fmt.Errorf("client_credentials grant requires E2E_OIDC_SA_CLIENT_ID and E2E_OIDC_SA_CLIENT_SECRET")
	}
	return d.postToken(ctx, d.tokenEndpoint(), map[string]string{
		"grant_type":    grantClientCredentials,
		"client_id":     saClientID,
		"client_secret": saSecret,
	})
}

// AcquireGatewayTokenWithRole acquires a per-gateway token for clientID and blocks
// until role lands in the token (gateway roles reconcile asynchronously after
// create). Password grant polls the per-gateway client directly; client_credentials
// token-exchanges the service account onto the gateway client.
func (d *kindDriver) AcquireGatewayTokenWithRole(ctx context.Context, user Credentials, clientID, role string) (Token, error) {
	timeout := durationEnv("E2E_GATEWAY_TOKEN_TIMEOUT", 300*time.Second)
	grant := envOr("E2E_OIDC_GRANT", grantPassword)

	var last Token
	err := harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		var (
			tok Token
			err error
		)
		switch grant {
		case grantPassword:
			tok, err = d.postToken(ctx, d.tokenEndpoint(), map[string]string{
				"grant_type": grantPassword,
				"client_id":  clientID,
				"username":   user.Username,
				"password":   user.Password,
			})
		case grantClientCredentials:
			tok, err = d.gatewayTokenExchange(ctx, clientID, user.Username)
		default:
			return false, fmt.Errorf("unsupported E2E_OIDC_GRANT %q", grant)
		}
		if err != nil {
			return false, nil // transient; keep polling until the role/client is ready
		}
		last = tok
		return tokenHasRole(tok.AccessToken, role), nil
	})
	if err != nil {
		return Token{}, fmt.Errorf("gateway token for client %s never gained role %q: %w", clientID, role, err)
	}
	return last, nil
}

// gatewayTokenExchange exchanges the hypershell-e2e service-account token onto the
// gateway client audience, optionally impersonating requestedSubject (the seeded
// developer) when it is not the admin user.
func (d *kindDriver) gatewayTokenExchange(ctx context.Context, clientID, requestedSubject string) (Token, error) {
	subjectTok, err := d.clientCredentialsToken(ctx)
	if err != nil {
		return Token{}, err
	}
	saClientID := os.Getenv("E2E_OIDC_SA_CLIENT_ID")
	saSecret := os.Getenv("E2E_OIDC_SA_CLIENT_SECRET")
	fields := map[string]string{
		"grant_type":         grantTokenExchange,
		"client_id":          saClientID,
		"client_secret":      saSecret,
		"subject_token":      subjectTok.AccessToken,
		"subject_token_type": tokenTypeAccessToken,
		"audience":           clientID,
	}
	if requestedSubject != "" && requestedSubject != envOr("E2E_OIDC_USERNAME", "admin") {
		fields["requested_subject"] = requestedSubject
	}
	return d.postToken(ctx, d.tokenEndpoint(), fields)
}

// kcAdminToken obtains a Keycloak master-realm admin token via admin-cli, for the
// role-assignment helpers.
func (d *kindDriver) kcAdminToken(ctx context.Context) (string, error) {
	base, _ := d.keycloakBaseAndRealm()
	tok, err := d.postToken(ctx, base+"/realms/"+kcMasterRealm+"/protocol/openid-connect/token", map[string]string{
		"grant_type": grantPassword,
		"client_id":  kcAdminCLIClient,
		"username":   d.kcAdminUser,
		"password":   d.kcAdminPassword,
	})
	if err != nil {
		return "", fmt.Errorf("acquire Keycloak admin token: %w", err)
	}
	return tok.AccessToken, nil
}
