package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// gatewayProgrammed reports whether a Gateway's status carries Programmed=True.
func gatewayProgrammed(gw *gatewayv1.Gateway) bool {
	for _, c := range gw.Status.Conditions {
		if c.Type == string(gatewayv1.GatewayConditionProgrammed) && c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// grpcRouteAccepted reports whether any parent status on the GRPCRoute carries
// Accepted=True.
func grpcRouteAccepted(route *gatewayv1.GRPCRoute) bool {
	for _, parent := range route.Status.Parents {
		for _, c := range parent.Conditions {
			if c.Type == string(gatewayv1.RouteConditionAccepted) && c.Status == metav1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

// --- Keycloak admin REST helpers ---

type kcRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type kcEntity struct {
	ID string `json:"id"`
}

func (d *kindDriver) kcAdminBase() string {
	base, realm := d.keycloakBaseAndRealm()
	return base + "/admin/realms/" + realm
}

func (d *kindDriver) kcDo(ctx context.Context, method, rawURL, adminTok string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	return resp.StatusCode, respBody, nil
}

func (d *kindDriver) kcClientUUID(ctx context.Context, adminTok, clientID string) (string, error) {
	u := d.kcAdminBase() + "/clients?clientId=" + url.QueryEscape(clientID)
	status, body, err := d.kcDo(ctx, http.MethodGet, u, adminTok, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("lookup client %q: status %d: %s", clientID, status, body)
	}
	var clients []kcEntity
	if err := json.Unmarshal(body, &clients); err != nil {
		return "", fmt.Errorf("parse clients: %w", err)
	}
	if len(clients) == 0 {
		return "", fmt.Errorf("no Keycloak client with clientId %q", clientID)
	}
	return clients[0].ID, nil
}

func (d *kindDriver) kcUserUUID(ctx context.Context, adminTok, username string) (string, error) {
	u := d.kcAdminBase() + "/users?exact=true&username=" + url.QueryEscape(username)
	status, body, err := d.kcDo(ctx, http.MethodGet, u, adminTok, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("lookup user %q: status %d: %s", username, status, body)
	}
	var users []kcEntity
	if err := json.Unmarshal(body, &users); err != nil {
		return "", fmt.Errorf("parse users: %w", err)
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no Keycloak user with username %q", username)
	}
	return users[0].ID, nil
}

func okStatus(status int) bool {
	return status == http.StatusOK || status == http.StatusNoContent
}

// AssignGatewayClientRole grants user the client role on the per-gateway OIDC
// client. Idempotent: Keycloak returns 204 whether or not the mapping already
// existed.
func (d *kindDriver) AssignGatewayClientRole(ctx context.Context, user, clientID, role string) error {
	adminTok, err := d.kcAdminToken(ctx)
	if err != nil {
		return err
	}
	clientUUID, err := d.kcClientUUID(ctx, adminTok, clientID)
	if err != nil {
		return err
	}
	userUUID, err := d.kcUserUUID(ctx, adminTok, user)
	if err != nil {
		return err
	}

	roleURL := d.kcAdminBase() + "/clients/" + clientUUID + "/roles/" + url.PathEscape(role)
	status, body, err := d.kcDo(ctx, http.MethodGet, roleURL, adminTok, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("lookup client role %q on %q: status %d: %s", role, clientID, status, body)
	}
	var r kcRole
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("parse client role: %w", err)
	}

	mapURL := d.kcAdminBase() + "/users/" + userUUID + "/role-mappings/clients/" + clientUUID
	status, body, err = d.kcDo(ctx, http.MethodPost, mapURL, adminTok, []kcRole{r})
	if err != nil {
		return err
	}
	if !okStatus(status) {
		return fmt.Errorf("assign client role %q to %q: status %d: %s", role, user, status, body)
	}
	return nil
}

// AssignRealmRole grants user a platform-wide realm role (for example
// platform:admin). Idempotent.
func (d *kindDriver) AssignRealmRole(ctx context.Context, user, role string) error {
	adminTok, err := d.kcAdminToken(ctx)
	if err != nil {
		return err
	}
	userUUID, err := d.kcUserUUID(ctx, adminTok, user)
	if err != nil {
		return err
	}

	roleURL := d.kcAdminBase() + "/roles/" + url.PathEscape(role)
	status, body, err := d.kcDo(ctx, http.MethodGet, roleURL, adminTok, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("lookup realm role %q: status %d: %s", role, status, body)
	}
	var r kcRole
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("parse realm role: %w", err)
	}

	mapURL := d.kcAdminBase() + "/users/" + userUUID + "/role-mappings/realm"
	status, body, err = d.kcDo(ctx, http.MethodPost, mapURL, adminTok, []kcRole{r})
	if err != nil {
		return err
	}
	if !okStatus(status) {
		return fmt.Errorf("assign realm role %q to %q: status %d: %s", role, user, status, body)
	}
	return nil
}

// --- Namespace GC timing ---

// ConfigureNamespaceGCTiming shortens the controller's namespace-GC interval and
// grace period for the duration of a long-mode run, then waits for the rollout.
func (d *kindDriver) ConfigureNamespaceGCTiming(ctx context.Context, interval, grace time.Duration) error {
	if err := d.setControllerGCEnv(ctx, map[string]string{
		gcEnvInterval: interval.String(),
		gcEnvGrace:    grace.String(),
	}, nil); err != nil {
		return err
	}
	d.gcPatched = true
	return d.waitControllerRollout(ctx)
}

// RestoreNamespaceGCTiming removes the override so the deployment's configured
// defaults take effect again. A no-op if ConfigureNamespaceGCTiming never ran.
func (d *kindDriver) RestoreNamespaceGCTiming(ctx context.Context) error {
	if !d.gcPatched {
		return nil
	}
	if err := d.setControllerGCEnv(ctx, nil, []string{gcEnvInterval, gcEnvGrace}); err != nil {
		return err
	}
	d.gcPatched = false
	return d.waitControllerRollout(ctx)
}

// setControllerGCEnv sets the given env vars and removes the named ones on the
// controller container, then updates the deployment.
func (d *kindDriver) setControllerGCEnv(ctx context.Context, set map[string]string, unset []string) error {
	deploys := d.clients.Kube.AppsV1().Deployments(d.platformNS)
	dep, err := deploys.Get(ctx, kindControllerDeploy, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment %s/%s: %w", d.platformNS, kindControllerDeploy, err)
	}

	idx := -1
	for i, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == kindControllerContainer {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("deployment %s has no container %q", kindControllerDeploy, kindControllerContainer)
	}

	env := dep.Spec.Template.Spec.Containers[idx].Env
	unsetSet := make(map[string]bool, len(unset))
	for _, k := range unset {
		unsetSet[k] = true
	}
	// Drop unset vars and any set vars we are about to re-add.
	filtered := env[:0]
	for _, e := range env {
		if unsetSet[e.Name] {
			continue
		}
		if _, replacing := set[e.Name]; replacing {
			continue
		}
		filtered = append(filtered, e)
	}
	for k, v := range set {
		filtered = append(filtered, corev1.EnvVar{Name: k, Value: v})
	}
	dep.Spec.Template.Spec.Containers[idx].Env = filtered

	if _, err := deploys.Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update deployment %s: %w", kindControllerDeploy, err)
	}
	return nil
}

// waitControllerRollout blocks until the controller deployment's rollout completes.
func (d *kindDriver) waitControllerRollout(ctx context.Context) error {
	timeout := durationEnv("E2E_ROLLOUT_TIMEOUT", 300*time.Second)
	return harness.Poll(ctx, 3*time.Second, timeout, func(ctx context.Context) (bool, error) {
		dep, err := d.clients.Kube.AppsV1().Deployments(d.platformNS).Get(ctx, kindControllerDeploy, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("get deployment %s: %w", kindControllerDeploy, err)
		}
		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		st := dep.Status
		rolledOut := st.ObservedGeneration >= dep.Generation &&
			st.UpdatedReplicas == desired &&
			st.AvailableReplicas == desired &&
			st.UnavailableReplicas == 0
		return rolledOut, nil
	})
}
