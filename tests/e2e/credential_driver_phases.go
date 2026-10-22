package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// credentialStorageKEKSecretName and credentialSecretsRBACName mirror the chart
// helpers of the same purpose (charts/openshell/templates/_helpers.tpl
// openshell.credentialStorageKeyEncryptionKeySecretName / the hard-coded
// "-credential-secrets" Role/RoleBinding name), both keyed off the chart
// fullname "openshell-gateway".
const (
	credentialStorageKEKSecretName = "openshell-gateway-credential-storage-key-encryption-key"
	credentialStorageKEKSecretKey  = "key-encryption-key"
	credentialStorageKEKEnvName    = "OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY"
	credentialSecretsRBACName      = "openshell-gateway-credential-secrets"
	credentialGatewayServiceAcct   = "openshell-gateway"
)

// p2_7CredentialDrivers validates the per-gateway credential storage drivers
// (E2E-COV-01): the Kubernetes Secrets and explicit KEK drivers unconditionally,
// the omitted-driver deployment default, the two rejection paths, and (opt-in)
// the Vault driver.
func (s *E2ESuite) p2_7CredentialDrivers(t *testing.T) {
	if s.skipKubeChecks {
		t.Skip("no per-cluster kube context; skipping credential driver checks")
	}

	t.Run("kubernetes-secrets", func(t *testing.T) { s.assertKubernetesSecretsDriver(t) })
	t.Run("kek-db", func(t *testing.T) { s.assertKEKDriver(t) })
	t.Run("omitted-default", func(t *testing.T) { s.assertOmittedDriverResolvesToDefault(t) })
	t.Run("invalid-driver", func(t *testing.T) { s.assertInvalidCredentialDriverRejected(t) })
	t.Run("vault", func(t *testing.T) { s.assertVaultDriverQualification(t) })
}

// assertKubernetesSecretsDriver covers E2E-COV-01.01 and the driver-change-after-set
// half of E2E-COV-01.06 (reusing this gateway rather than provisioning a sixth).
func (s *E2ESuite) assertKubernetesSecretsDriver(t *testing.T) {
	ctx := t.Context()
	gw := s.createGatewayWithCredentialDriver(t, "e2e-cred-ks-"+s.runID, `{"type":"kubernetes-secrets"}`)
	s.trackGateway(gw.ID)
	running := s.waitGatewayRunning(t, gw.ID)
	ns := running.Namespace

	toml := s.gatewayConfigTOML(t, ns)
	s.Assert().Contains(toml, `credential_drivers = ["kubernetes-secrets"]`, "gateway.toml declares the kubernetes-secrets driver")
	s.Assert().NotContains(toml, "[openshell.gateway.credential_storage]", "gateway.toml omits the KEK credential_storage section")

	_, err := s.clients.Kube.CoreV1().Secrets(ns).Get(ctx, credentialStorageKEKSecretName, metav1.GetOptions{})
	s.Assert().True(apierrors.IsNotFound(err), "chart-generated KEK Secret must not exist for the kubernetes-secrets driver (err=%v)", err)
	s.assertDeploymentEnvVarAbsent(t, ns, credentialStorageKEKEnvName)

	role, err := s.clients.Kube.RbacV1().Roles(ns).Get(ctx, credentialSecretsRBACName, metav1.GetOptions{})
	s.Require().NoError(err, "get credential-secrets Role")
	s.Assert().NotEmpty(role.Rules, "credential-secrets Role has rules")
	binding, err := s.clients.Kube.RbacV1().RoleBindings(ns).Get(ctx, credentialSecretsRBACName, metav1.GetOptions{})
	s.Require().NoError(err, "get credential-secrets RoleBinding")
	s.Require().Len(binding.Subjects, 1, "credential-secrets RoleBinding has one subject")
	s.Assert().Equal("ServiceAccount", binding.Subjects[0].Kind)
	s.Assert().Equal(credentialGatewayServiceAcct, binding.Subjects[0].Name, "RoleBinding binds the gateway ServiceAccount")

	// E2E-COV-01.06 (second half): changing the driver after it is set is rejected.
	status, body, err := s.admin.RawJSON(ctx, http.MethodPatch, "/gateways/"+gw.ID, map[string]string{"credential_driver": `{"type":"kek-db"}`})
	s.Require().NoError(err, "patch credential_driver to a different value")
	s.Assert().Equalf(http.StatusConflict, status, "changing an already-set credential_driver must be 409 (body: %s)", string(body))

	// Re-submitting the unchanged value is a no-op, not a conflict. The
	// immutability check is a raw string compare against the stored value
	// (handler.go), and credential_driver is a jsonb column whose read-back text
	// PostgreSQL may reformat, so re-submit the gateway's own current value
	// rather than the literal string this test originally sent.
	current, err := s.admin.Gateways().Get(ctx, gw.ID)
	s.Require().NoError(err, "get gateway to read its current credential_driver")
	status, body, err = s.admin.RawJSON(ctx, http.MethodPatch, "/gateways/"+gw.ID, map[string]string{"credential_driver": current.CredentialDriver})
	s.Require().NoError(err, "patch credential_driver to its current value")
	s.Assert().Equalf(http.StatusOK, status, "re-submitting the unchanged credential_driver must be 200 (body: %s)", string(body))

	s.deleteGatewayAndWaitGC(t, gw.ID, ns)
}

// assertKEKDriver covers E2E-COV-01.04: the explicit kek-db driver provisions
// the KEK store regardless of the deployment's omitted-driver default.
func (s *E2ESuite) assertKEKDriver(t *testing.T) {
	ctx := t.Context()
	gw := s.createGatewayWithCredentialDriver(t, "e2e-cred-kek-"+s.runID, `{"type":"kek-db"}`)
	s.trackGateway(gw.ID)
	running := s.waitGatewayRunning(t, gw.ID)
	ns := running.Namespace

	toml := s.gatewayConfigTOML(t, ns)
	s.Assert().Contains(toml, "[openshell.gateway.credential_storage]", "gateway.toml carries the KEK credential_storage section")

	secret, err := s.clients.Kube.CoreV1().Secrets(ns).Get(ctx, credentialStorageKEKSecretName, metav1.GetOptions{})
	s.Require().NoError(err, "get chart-generated KEK Secret")
	s.Assert().NotEmpty(secret.Data[credentialStorageKEKSecretKey], "KEK Secret carries its key-encryption-key")
	s.assertDeploymentEnvVarPresent(t, ns, credentialStorageKEKEnvName)

	s.deleteGatewayAndWaitGC(t, gw.ID, ns)
}

// assertOmittedDriverResolvesToDefault covers E2E-COV-01.05: an omitted
// credential_driver must match whatever the deployment's resolved default
// actually renders, not a hard-coded assumption of kek-db.
func (s *E2ESuite) assertOmittedDriverResolvesToDefault(t *testing.T) {
	ctx := t.Context()
	gw := s.createGateway(t, "e2e-cred-def-"+s.runID)
	s.trackGateway(gw.ID)
	running := s.waitGatewayRunning(t, gw.ID)
	ns := running.Namespace

	toml := s.gatewayConfigTOML(t, ns)
	hasKEKSection := strings.Contains(toml, "[openshell.gateway.credential_storage]")
	hasVaultDrivers := strings.Contains(toml, `credential_drivers = ["vault"]`)
	s.Require().True(hasKEKSection || hasVaultDrivers, "omitted-driver gateway.toml resolves to either the kek-db or vault driver:\n%s", toml)
	s.Assert().NotEqual(hasKEKSection, hasVaultDrivers, "exactly one of the kek-db or vault driver sections is present")

	if hasKEKSection {
		s.assertDeploymentEnvVarPresent(t, ns, credentialStorageKEKEnvName)
		_, err := s.clients.Kube.CoreV1().Secrets(ns).Get(ctx, credentialStorageKEKSecretName, metav1.GetOptions{})
		s.Assert().NoError(err, "resolved kek-db default provisions the KEK Secret")
	} else {
		s.assertDeploymentEnvVarAbsent(t, ns, credentialStorageKEKEnvName)
		s.Assert().NotContains(toml, "[openshell.gateway.credential_storage]", "resolved vault default omits the KEK section")
	}

	s.deleteGatewayAndWaitGC(t, gw.ID, ns)
}

// assertInvalidCredentialDriverRejected covers the first half of E2E-COV-01.06:
// an unsupported driver type is rejected at the API or surfaces as a reconcile
// failure, never silently accepted.
func (s *E2ESuite) assertInvalidCredentialDriverRejected(t *testing.T) {
	ctx := t.Context()
	body := gatewayCreateRequestForDriver("e2e-cred-bad-"+s.runID, s.driver.Name(), s.gatewayOIDCConfig())
	body.CredentialDriver = `{"type":"s3"}`
	status, created, resp := s.createGatewayRequest(t, body)
	if status != http.StatusCreated {
		s.Assert().GreaterOrEqualf(status, http.StatusBadRequest, "invalid credential_driver.type must not be accepted (status %d, body: %s)", status, string(resp))
		return
	}

	s.trackGateway(created.ID)
	timeout := durationSecondsEnv("E2E_PROVISION_TIMEOUT", 180*time.Second)
	pollErr := harness.Poll(ctx, 3*time.Second, timeout, func(ctx context.Context) (bool, error) {
		gw, err := s.admin.Gateways().Get(ctx, created.ID)
		if err != nil {
			return false, nil
		}
		if gw.Phase == "Running" {
			return false, fmt.Errorf("gateway with credential_driver.type=s3 unexpectedly reached Running")
		}
		for _, condition := range gw.ProvisioningConditions {
			if value, _ := condition["condition_status"].(string); value == "Failed" {
				return true, nil
			}
		}
		return false, nil
	})
	s.Require().NoError(pollErr, "invalid credential_driver.type surfaces as a reconcile failure")

	ns := s.waitGatewayNamespace(t, created.ID)
	s.deleteGatewayAndWaitGC(t, created.ID, ns)
}

// assertVaultDriverQualification covers E2E-COV-01.02/01.03: the Vault driver
// is exercised only under E2E_QUALIFY_VAULT=1 with a reachable Vault; otherwise
// it SHALL t.Skipf, never a false pass.
func (s *E2ESuite) assertVaultDriverQualification(t *testing.T) {
	if envOrDefault("E2E_QUALIFY_VAULT", "0") != "1" {
		t.Skip("Vault credential-driver qualification requires E2E_QUALIFY_VAULT=1 (and a reachable E2E_VAULT_ADDR/E2E_VAULT_ROLE); the Kind gate hosts no Vault server")
	}
	vaultAddr := envOrDefault("E2E_VAULT_ADDR", "")
	vaultRole := envOrDefault("E2E_VAULT_ROLE", "")
	s.Require().NotEmpty(vaultAddr, "E2E_QUALIFY_VAULT=1 requires E2E_VAULT_ADDR")
	s.Require().NotEmpty(vaultRole, "E2E_QUALIFY_VAULT=1 requires E2E_VAULT_ROLE")

	ctx := t.Context()
	// Exercise the SUPPLIED Vault backend, not the convention default: put the
	// requested address and role into credential_driver.vault so the rendered
	// gateway config points at E2E_VAULT_ADDR/E2E_VAULT_ROLE.
	credDriver := fmt.Sprintf(`{"type":"vault","vault":{"address":%q,"role":%q}}`, vaultAddr, vaultRole)
	gw := s.createGatewayWithCredentialDriver(t, "e2e-cred-vault-"+s.runID, credDriver)
	s.trackGateway(gw.ID)
	running := s.waitGatewayRunning(t, gw.ID)
	ns := running.Namespace

	toml := s.gatewayConfigTOML(t, ns)
	s.Assert().Contains(toml, `credential_drivers = ["vault"]`, "gateway.toml declares the vault driver")
	s.Assert().NotContains(toml, "[openshell.gateway.credential_storage]", "gateway.toml omits the KEK credential_storage section")
	s.Assert().Contains(toml, vaultAddr, "gateway.toml uses the supplied Vault address (E2E_VAULT_ADDR), not the convention default")
	s.Assert().Contains(toml, vaultRole, "gateway.toml uses the supplied Vault role (E2E_VAULT_ROLE), not the convention default")

	dep, err := s.clients.Kube.AppsV1().Deployments(ns).Get(ctx, "openshell-gateway", metav1.GetOptions{})
	s.Require().NoError(err, "get gateway deployment for vault SA token volume")
	s.Assert().True(hasProjectedTokenVolumeMount(dep.Spec.Template.Spec, "vault", "/var/run/secrets/vault"),
		"gateway deployment mounts a projected ServiceAccount token volume with audience vault at /var/run/secrets/vault")

	s.deleteGatewayAndWaitGC(t, gw.ID, ns)
}

// gatewayConfigTOML reads the rendered gateway.toml from the openshell-gateway-config
// ConfigMap in ns.
func (s *E2ESuite) gatewayConfigTOML(t *testing.T, ns string) string {
	cm, err := s.clients.Kube.CoreV1().ConfigMaps(ns).Get(t.Context(), "openshell-gateway-config", metav1.GetOptions{})
	s.Require().NoError(err, "get gateway config ConfigMap")
	toml, ok := cm.Data["gateway.toml"]
	s.Require().True(ok, "gateway config ConfigMap has a gateway.toml key")
	return toml
}

// assertDeploymentEnvVarPresent/Absent check the gateway Deployment's first
// (openshell-gateway) container for env.
func (s *E2ESuite) assertDeploymentEnvVarPresent(t *testing.T, ns, name string) {
	s.Assert().True(s.deploymentHasEnvVar(t, ns, name), "gateway deployment env carries %s", name)
}

func (s *E2ESuite) assertDeploymentEnvVarAbsent(t *testing.T, ns, name string) {
	s.Assert().False(s.deploymentHasEnvVar(t, ns, name), "gateway deployment env must not carry %s", name)
}

func (s *E2ESuite) deploymentHasEnvVar(t *testing.T, ns, name string) bool {
	dep, err := s.clients.Kube.AppsV1().Deployments(ns).Get(t.Context(), "openshell-gateway", metav1.GetOptions{})
	s.Require().NoError(err, "get gateway deployment")
	s.Require().NotEmpty(dep.Spec.Template.Spec.Containers, "gateway deployment has a container")
	for _, env := range dep.Spec.Template.Spec.Containers[0].Env {
		if env.Name == name {
			return true
		}
	}
	return false
}

// hasProjectedTokenVolumeMount reports whether pod declares a projected
// ServiceAccountToken volume with the given audience that the first container
// mounts at mountPath.
func hasProjectedTokenVolumeMount(pod corev1.PodSpec, audience, mountPath string) bool {
	audienceVolumes := map[string]bool{}
	for _, v := range pod.Volumes {
		if v.Projected == nil {
			continue
		}
		for _, source := range v.Projected.Sources {
			if source.ServiceAccountToken != nil && source.ServiceAccountToken.Audience == audience {
				audienceVolumes[v.Name] = true
			}
		}
	}
	if len(audienceVolumes) == 0 || len(pod.Containers) == 0 {
		return false
	}
	for _, mount := range pod.Containers[0].VolumeMounts {
		if audienceVolumes[mount.Name] && mount.MountPath == mountPath {
			return true
		}
	}
	return false
}
