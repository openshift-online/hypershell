package config

import "testing"

func TestSandboxImagePullRoles(t *testing.T) {
	for _, value := range []string{"", "[]", `[{"namespace":"images","role":"runner-pull"}]`} {
		t.Setenv("GATEWAY_SANDBOX_IMAGE_PULL_ROLES", value)
		if _, err := SandboxImagePullRoles(); err != nil {
			t.Fatalf("valid configuration rejected: %v", err)
		}
	}
	for _, value := range []string{"null", "{}", "[] []", `[{"namespace":"images","role":"*"}]`, `[{"namespace":"","role":"pull"}]`, `[{"namespace":"images","role":"pull","subjects":["*"]}]`, `[{"namespace":"images","role":"pull"},{"namespace":"images","role":"pull"}]`} {
		t.Setenv("GATEWAY_SANDBOX_IMAGE_PULL_ROLES", value)
		if _, err := SandboxImagePullRoles(); err == nil {
			t.Fatalf("invalid configuration accepted: %s", value)
		}
		if _, err := Load(); err == nil {
			t.Fatal("invalid image pull configuration did not block startup")
		}
	}
}
