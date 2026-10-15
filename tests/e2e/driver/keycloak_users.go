package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type keycloakUserCredential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

type keycloakUserCreate struct {
	Username      string                   `json:"username"`
	Enabled       bool                     `json:"enabled"`
	EmailVerified bool                     `json:"emailVerified"`
	Credentials   []keycloakUserCredential `json:"credentials"`
}

func (d *kindDriver) CreateTestUser(ctx context.Context, username, password string) error {
	adminToken, err := d.kcAdminToken(ctx)
	if err != nil {
		return err
	}
	return createKeycloakTestUser(ctx, d.httpClient, d.kcAdminBase(), adminToken, username, password)
}

func (d *kindDriver) DeleteTestUser(ctx context.Context, username string) error {
	adminToken, err := d.kcAdminToken(ctx)
	if err != nil {
		return err
	}
	return deleteKeycloakTestUser(ctx, d.httpClient, d.kcAdminBase(), adminToken, username)
}

func (d *openshiftDriver) CreateTestUser(ctx context.Context, username, password string) error {
	adminToken, err := d.kcAdminToken(ctx)
	if err != nil {
		return err
	}
	return createKeycloakTestUser(ctx, d.httpClient, d.kcAdminBase(), adminToken, username, password)
}

func (d *openshiftDriver) DeleteTestUser(ctx context.Context, username string) error {
	adminToken, err := d.kcAdminToken(ctx)
	if err != nil {
		return err
	}
	return deleteKeycloakTestUser(ctx, d.httpClient, d.kcAdminBase(), adminToken, username)
}

func createKeycloakTestUser(ctx context.Context, client *http.Client, adminBase, adminToken, username, password string) error {
	exists, err := keycloakTestUserExists(ctx, client, adminBase, adminToken, username)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	statusCode, body, err := kcDoJSON(ctx, client, http.MethodPost, adminBase+"/users", adminToken, keycloakUserCreate{
		Username:      username,
		Enabled:       true,
		EmailVerified: true,
		Credentials: []keycloakUserCredential{{
			Type: "password", Value: password, Temporary: false,
		}},
	})
	if err != nil {
		return fmt.Errorf("create Keycloak test user %q: %w", username, err)
	}
	if statusCode != http.StatusCreated {
		return fmt.Errorf("create Keycloak test user %q: status %d: %s", username, statusCode, body)
	}
	return nil
}

func keycloakTestUserExists(ctx context.Context, client *http.Client, adminBase, adminToken, username string) (bool, error) {
	statusCode, body, err := kcDoJSON(ctx, client, http.MethodGet,
		adminBase+"/users?exact=true&username="+url.QueryEscape(username), adminToken, nil)
	if err != nil {
		return false, fmt.Errorf("lookup Keycloak test user %q: %w", username, err)
	}
	if statusCode != http.StatusOK {
		return false, fmt.Errorf("lookup Keycloak test user %q: status %d: %s", username, statusCode, body)
	}
	var users []kcEntity
	if err := json.Unmarshal(body, &users); err != nil {
		return false, fmt.Errorf("decode Keycloak test user lookup %q: %w", username, err)
	}
	return len(users) > 0, nil
}

func deleteKeycloakTestUser(ctx context.Context, client *http.Client, adminBase, adminToken, username string) error {
	userID, err := kcLookupID(ctx, client, adminBase+"/users?exact=true&username="+url.QueryEscape(username), adminToken)
	if err != nil {
		return fmt.Errorf("lookup Keycloak test user %q: %w", username, err)
	}
	statusCode, body, err := kcDoJSON(ctx, client, http.MethodDelete, adminBase+"/users/"+userID, adminToken, nil)
	if err != nil {
		return fmt.Errorf("delete Keycloak test user %q: %w", username, err)
	}
	if statusCode != http.StatusNoContent {
		return fmt.Errorf("delete Keycloak test user %q: status %d: %s", username, statusCode, body)
	}
	return nil
}
