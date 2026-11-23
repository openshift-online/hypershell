package helm

import (
	"fmt"
	"sort"
	"strings"
)

// Policy interceptor sidecar rendering (OSG-CORE-19, OSG-HELM-06, GTW-CFG-05).
const (
	// PolicyInterceptorContainer is the native sidecar container name.
	PolicyInterceptorContainer = "hypershell-policy-interceptor"
	// PolicyInterceptorBinary is the interceptor binary inside the control-plane
	// image (PLAT-PGP-09).
	PolicyInterceptorBinary = "/usr/local/bin/hypershell-policy-interceptor"
	// PolicySocketDir is the pod-local emptyDir both containers mount.
	PolicySocketDir = "/var/run/hypershell-policy"
	// PolicySocketPath is the interceptor's unix socket.
	PolicySocketPath = PolicySocketDir + "/interceptor.sock"
	// PolicyTokenDir holds the projected token the sidecar reports events with.
	PolicyTokenDir = "/var/run/secrets/hypershell-policy"
	// PolicyJWTDir holds the gateway JWT public key the sidecar verifies with.
	PolicyJWTDir = "/etc/hypershell-policy/jwt"
	// PolicyEventsAudience binds the projected token to the control plane's
	// ReportPolicyEvents endpoint (SEC-RBAC-32).
	PolicyEventsAudience = "hypershell-policy-events"
	// PolicyHealthPort is the sidecar's health and metrics port.
	PolicyHealthPort = 9466

	policySocketVolume = "hypershell-policy-socket"
	policyTokenVolume  = "hypershell-policy-token"
	policyJWTVolume    = "hypershell-policy-jwt"
)

// InterceptorBinding is one static binding of the interceptor registration
// (PLAT-PGP-10, PLAT-PGP-11).
type InterceptorBinding struct {
	RPC           string
	Phases        []string
	FailurePolicy string
}

// PolicyInterceptorConfig is everything the chart values need to render the
// sidecar and its gateway.toml registration. It carries no policy content: a
// policy change never changes these values, so it never rolls the gateway
// (GTW-CFG-05.02).
type PolicyInterceptorConfig struct {
	Name          string
	Audience      string
	Image         string
	GatewayID     string
	JWTIssuer     string
	EventsAddress string
	PoliciesURL   string
	Defaults      map[string]string // env name -> quantity
	Bindings      []InterceptorBinding
	// KnownRPCs is the gateway's interceptable RPC allowlist; a binding outside
	// it fails validation (GTW-CFG-05.01).
	KnownRPCs []string
}

var validPhases = map[string]bool{"modify_operation": true, "validate": true, "post_commit": true}

// ValidateInterceptorRegistration checks the generated registration before any
// install or upgrade (GTW-CFG-05.01): every binding names a known interceptable
// RPC once, with valid phases and failure policy, and post_commit bindings fail
// open. A failure blocks rollout and leaves a running gateway untouched.
func ValidateInterceptorRegistration(c *PolicyInterceptorConfig) error {
	if c == nil {
		return nil
	}
	if c.Name == "" || c.Image == "" || c.Audience == "" {
		return fmt.Errorf("policy interceptor registration needs a name, an image, and an audience")
	}
	if len(c.Bindings) == 0 {
		return fmt.Errorf("policy interceptor registration has no bindings")
	}
	known := map[string]bool{}
	for _, r := range c.KnownRPCs {
		known[r] = true
	}
	seen := map[string]bool{}
	for _, b := range c.Bindings {
		if !known[b.RPC] {
			return fmt.Errorf("policy interceptor binding names an unknown or non-interceptable RPC %q", b.RPC)
		}
		if seen[b.RPC] {
			return fmt.Errorf("policy interceptor binding for %q is declared twice", b.RPC)
		}
		seen[b.RPC] = true
		if len(b.Phases) == 0 {
			return fmt.Errorf("policy interceptor binding %q declares no phases", b.RPC)
		}
		for _, p := range b.Phases {
			if !validPhases[p] {
				return fmt.Errorf("policy interceptor binding %q has invalid phase %q", b.RPC, p)
			}
			if p == "post_commit" && b.FailurePolicy != "fail_open" {
				return fmt.Errorf("policy interceptor binding %q uses post_commit, which must fail open", b.RPC)
			}
		}
		if b.FailurePolicy != "fail_closed" && b.FailurePolicy != "fail_open" {
			return fmt.Errorf("policy interceptor binding %q has invalid failure policy %q", b.RPC, b.FailurePolicy)
		}
	}
	return nil
}

// buildPolicyInterceptorValues renders the native sidecar, its volumes, the
// gateway socket mount, and the interceptor registration in gatewayConfig.
func (b *ValuesBuilder) buildPolicyInterceptorValues(values map[string]interface{}) {
	c := b.PolicyInterceptor
	if c == nil {
		return
	}
	bindings := make([]interface{}, 0, len(c.Bindings))
	for _, bd := range c.Bindings {
		phases := make([]interface{}, 0, len(bd.Phases))
		for _, p := range bd.Phases {
			phases = append(phases, p)
		}
		bindings = append(bindings, map[string]interface{}{
			"rpc":            bd.RPC,
			"phases":         phases,
			"failure_policy": bd.FailurePolicy,
		})
	}
	setNestedValue(values, []interface{}{map[string]interface{}{
		"name":           c.Name,
		"grpc_endpoint":  "unix://" + PolicySocketPath,
		"audience":       c.Audience,
		"order":          10,
		"failure_policy": "fail_closed",
		"binding_policy": "exact",
		"timeout":        "500ms",
		"bindings":       bindings,
	}}, "gatewayConfig", "openshell.gateway.interceptors")

	env := []interface{}{
		map[string]interface{}{"name": "HYPERSHELL_POLICY_SOCKET", "value": PolicySocketPath},
		map[string]interface{}{"name": "HYPERSHELL_POLICY_GATEWAY_ID", "value": c.GatewayID},
		map[string]interface{}{"name": "HYPERSHELL_POLICY_NAMESPACE", "valueFrom": map[string]interface{}{
			"fieldRef": map[string]interface{}{"fieldPath": "metadata.namespace"},
		}},
		map[string]interface{}{"name": "HYPERSHELL_POLICY_JWT_ISSUER", "value": c.JWTIssuer},
		map[string]interface{}{"name": "HYPERSHELL_POLICY_JWT_AUDIENCE", "value": c.Audience},
		map[string]interface{}{"name": "HYPERSHELL_POLICY_JWT_PUBLIC_KEY_FILE", "value": PolicyJWTDir + "/public.pem"},
		map[string]interface{}{"name": "HYPERSHELL_POLICY_EVENTS_ADDR", "value": c.EventsAddress},
		map[string]interface{}{"name": "HYPERSHELL_POLICY_EVENTS_TOKEN_FILE", "value": PolicyTokenDir + "/token"},
		map[string]interface{}{"name": "HYPERSHELL_POLICY_HEALTH_ADDR", "value": fmt.Sprintf(":%d", PolicyHealthPort)},
	}
	if c.PoliciesURL != "" {
		env = append(env, map[string]interface{}{"name": "HYPERSHELL_POLICY_POLICIES_URL", "value": c.PoliciesURL})
	}
	names := make([]string, 0, len(c.Defaults))
	for k := range c.Defaults {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		env = append(env, map[string]interface{}{"name": k, "value": c.Defaults[k]})
	}

	sidecar := map[string]interface{}{
		"name":            PolicyInterceptorContainer,
		"image":           c.Image,
		"imagePullPolicy": "IfNotPresent",
		"command":         []interface{}{PolicyInterceptorBinary},
		// Native sidecar: started, and past its startup probe, before the
		// gateway container, so it serves Describe at gateway startup.
		"restartPolicy": "Always",
		"env":           env,
		"ports": []interface{}{map[string]interface{}{
			"name": "policy-health", "containerPort": PolicyHealthPort, "protocol": "TCP",
		}},
		"startupProbe": map[string]interface{}{
			"httpGet":          map[string]interface{}{"path": "/readyz", "port": PolicyHealthPort},
			"periodSeconds":    1,
			"failureThreshold": 60,
		},
		"livenessProbe": map[string]interface{}{
			"httpGet":       map[string]interface{}{"path": "/healthz", "port": PolicyHealthPort},
			"periodSeconds": 10,
		},
		"securityContext": map[string]interface{}{
			"allowPrivilegeEscalation": false,
			"readOnlyRootFilesystem":   true,
			"runAsNonRoot":             true,
			"capabilities":             map[string]interface{}{"drop": []interface{}{"ALL"}},
			"seccompProfile":           map[string]interface{}{"type": "RuntimeDefault"},
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "10m", "memory": "32Mi"},
			"limits":   map[string]interface{}{"cpu": "200m", "memory": "128Mi"},
		},
		"volumeMounts": []interface{}{
			map[string]interface{}{"name": policySocketVolume, "mountPath": PolicySocketDir},
			map[string]interface{}{"name": policyTokenVolume, "mountPath": PolicyTokenDir, "readOnly": true},
			map[string]interface{}{"name": policyJWTVolume, "mountPath": PolicyJWTDir, "readOnly": true},
		},
	}
	appendNestedSlice(values, []interface{}{sidecar}, "server", "extraInitContainers")
	appendNestedSlice(values, []interface{}{
		map[string]interface{}{"name": policySocketVolume, "emptyDir": map[string]interface{}{"medium": "Memory", "sizeLimit": "1Mi"}},
		map[string]interface{}{"name": policyTokenVolume, "projected": map[string]interface{}{
			"defaultMode": 0o400,
			"sources": []interface{}{map[string]interface{}{"serviceAccountToken": map[string]interface{}{
				"path": "token", "audience": PolicyEventsAudience, "expirationSeconds": 3600,
			}}},
		}},
		map[string]interface{}{"name": policyJWTVolume, "secret": map[string]interface{}{
			"secretName":  ReleaseName + "-jwt-keys",
			"defaultMode": 0o444,
			"items":       []interface{}{map[string]interface{}{"key": "public.pem", "path": "public.pem"}},
		}},
	}, "server", "extraVolumes")
	appendNestedSlice(values, []interface{}{
		map[string]interface{}{"name": policySocketVolume, "mountPath": PolicySocketDir},
	}, "server", "extraVolumeMounts")
}

// PolicyJWTIssuer returns the issuer the gateway stamps on extension tokens:
// openshell-gateway:<gateway_jwt.gateway_id>. The chart sets that id to
// server.sandboxJwt.gatewayId when present (managed workspace mode) and to the
// chart fullname otherwise.
func PolicyJWTIssuer(workspaceMode, namespace string) (string, error) {
	id := ReleaseName
	if workspaceMode == "managed" {
		managed, err := ManagedGatewayID(namespace)
		if err != nil {
			return "", err
		}
		id = managed
	}
	return "openshell-gateway:" + strings.TrimSpace(id), nil
}
