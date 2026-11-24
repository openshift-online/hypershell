package helm

import (
	"reflect"
	"strings"
	"testing"
)

func testInterceptorConfig() *PolicyInterceptorConfig {
	return &PolicyInterceptorConfig{
		Name:          "hypershell-policy",
		Audience:      "urn:openshell:extension:interceptor:hypershell-policy",
		Image:         "quay.io/example/hypershell-control-plane@sha256:abc",
		GatewayID:     "gw-1",
		JWTIssuer:     "openshell-gateway:aaaaaaaaaaaaaaaa",
		EventsAddress: "hypershell-controller.hypershell-system.svc.cluster.local:9444",
		Defaults:      map[string]string{"HYPERSHELL_POLICY_DEFAULT_CPU_REQUEST": "500m"},
		Bindings: []InterceptorBinding{
			{RPC: "openshell.v1.OpenShell/CreateSandbox", Phases: []string{"modify_operation", "validate"}, FailurePolicy: "fail_closed"},
			{RPC: "openshell.v1.OpenShell/DeleteSandbox", Phases: []string{"validate"}, FailurePolicy: "fail_open"},
		},
		KnownRPCs: []string{"openshell.v1.OpenShell/CreateSandbox", "openshell.v1.OpenShell/DeleteSandbox"},
	}
}

// Verifies: OSG-HELM-06.01, OSG-CORE-19.01, PLAT-PGP-09.01
func TestPolicyInterceptorValuesRenderSidecarAndRegistration(t *testing.T) {
	values := map[string]interface{}{}
	b := &ValuesBuilder{PolicyInterceptor: testInterceptorConfig()}
	b.buildPolicyInterceptorValues(values)

	server := values["server"].(map[string]interface{})
	inits := server["extraInitContainers"].([]interface{})
	sidecar := inits[0].(map[string]interface{})
	if sidecar["restartPolicy"] != "Always" || sidecar["image"] != "quay.io/example/hypershell-control-plane@sha256:abc" {
		t.Fatalf("sidecar must be a native sidecar at the controller image: %+v", sidecar)
	}
	sc := sidecar["securityContext"].(map[string]interface{})
	if sc["runAsNonRoot"] != true || sc["allowPrivilegeEscalation"] != false {
		t.Fatalf("sidecar must run with a restricted security context: %+v", sc)
	}
	vols := server["extraVolumes"].([]interface{})
	names := []string{}
	for _, v := range vols {
		names = append(names, v.(map[string]interface{})["name"].(string))
	}
	if !reflect.DeepEqual(names, []string{policySocketVolume, policyTokenVolume, policyJWTVolume}) {
		t.Fatalf("volumes = %v", names)
	}
	mounts := server["extraVolumeMounts"].([]interface{})
	if mounts[0].(map[string]interface{})["mountPath"] != PolicySocketDir {
		t.Fatalf("the gateway container must mount the socket dir: %+v", mounts)
	}
	reg := values["gatewayConfig"].(map[string]interface{})["openshell.gateway.interceptors"].([]interface{})[0].(map[string]interface{})
	if reg["binding_policy"] != "exact" || reg["grpc_endpoint"] != "unix://"+PolicySocketPath || len(reg["bindings"].([]interface{})) != 2 {
		t.Fatalf("registration = %+v", reg)
	}
}

// Verifies: GTW-CFG-05.02, PLAT-PGP-10.01
func TestPolicyInterceptorValuesIndependentOfPolicies(t *testing.T) {
	// The values carry no policy content: two builds for the same gateway are
	// identical whatever the policies are, so a policy change never changes
	// gateway.toml or rolls the workload.
	a, b := map[string]interface{}{}, map[string]interface{}{}
	(&ValuesBuilder{PolicyInterceptor: testInterceptorConfig()}).buildPolicyInterceptorValues(a)
	(&ValuesBuilder{PolicyInterceptor: testInterceptorConfig()}).buildPolicyInterceptorValues(b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("interceptor values must be deterministic")
	}
}

// Verifies: GTW-CFG-05.01
func TestValidateInterceptorRegistration(t *testing.T) {
	if err := ValidateInterceptorRegistration(testInterceptorConfig()); err != nil {
		t.Fatalf("valid registration rejected: %v", err)
	}
	cases := map[string]func(c *PolicyInterceptorConfig){
		"unknown RPC":    func(c *PolicyInterceptorConfig) { c.Bindings[1].RPC = "openshell.v1.OpenShell/GetSandbox" },
		"invalid phase":  func(c *PolicyInterceptorConfig) { c.Bindings[0].Phases = []string{"before"} },
		"invalid policy": func(c *PolicyInterceptorConfig) { c.Bindings[0].FailurePolicy = "ignore" },
		"post_commit closed": func(c *PolicyInterceptorConfig) {
			c.Bindings[0].Phases = []string{"post_commit"}
		},
		"duplicate": func(c *PolicyInterceptorConfig) { c.Bindings[1] = c.Bindings[0] },
	}
	for name, mutate := range cases {
		c := testInterceptorConfig()
		mutate(c)
		if err := ValidateInterceptorRegistration(c); err == nil {
			t.Errorf("%s: invalid registration accepted", name)
		}
	}
}

// Verifies: PLAT-PGP-09.02
func TestPolicyJWTIssuerMatchesChartGatewayID(t *testing.T) {
	got, err := PolicyJWTIssuer("managed", "openshell-0123456789abcdef")
	if err != nil || !strings.HasPrefix(got, "openshell-gateway:") || got == "openshell-gateway:"+ReleaseName {
		t.Fatalf("managed issuer = %q, %v", got, err)
	}
	if got, _ := PolicyJWTIssuer("shared", "anything"); got != "openshell-gateway:"+ReleaseName {
		t.Fatalf("shared issuer = %q", got)
	}
}

// Verifies: OSG-CORE-19.03, PLAT-PGP-08.04
func TestPolicyInterceptorValuesStampDefaultsHashOnPodTemplate(t *testing.T) {
	values := map[string]interface{}{"podAnnotations": map[string]interface{}{"existing": "kept"}}
	c := testInterceptorConfig()
	c.DefaultsHash = "0123456789abcdef"
	(&ValuesBuilder{PolicyInterceptor: c}).buildPolicyInterceptorValues(values)
	ann := values["podAnnotations"].(map[string]interface{})
	if ann[PolicyDefaultsHashAnnotation] != "0123456789abcdef" || ann["existing"] != "kept" {
		t.Fatalf("pod annotations = %v", ann)
	}

	// A different defaults hash changes the pod template, which rolls the pods.
	other := map[string]interface{}{}
	c2 := testInterceptorConfig()
	c2.DefaultsHash = "fedcba9876543210"
	(&ValuesBuilder{PolicyInterceptor: c2}).buildPolicyInterceptorValues(other)
	if other["podAnnotations"].(map[string]interface{})[PolicyDefaultsHashAnnotation] == ann[PolicyDefaultsHashAnnotation] {
		t.Fatal("a defaults change must change the pod template annotation")
	}
}
