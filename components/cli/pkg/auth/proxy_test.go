package auth

import (
	"net/http"
	"os"
	"os/exec"
	"testing"
)

const proxyHelperEnv = "HSCTL_TEST_PROXY_HELPER"

// http.ProxyFromEnvironment reads the environment once per process, so the
// assertion runs in a re-executed test binary whose environment is fixed up
// front instead of relying on t.Setenv winning that race.
func TestNewHTTPClientHonorsProxyEnvironment(t *testing.T) {
	if os.Getenv(proxyHelperEnv) == "1" {
		for _, insecure := range []bool{false, true} {
			// A nil Transport means http.DefaultTransport.
			transport := http.DefaultTransport.(*http.Transport)
			if rt := newHTTPClient(insecure).Transport; rt != nil {
				var ok bool
				if transport, ok = rt.(*http.Transport); !ok {
					t.Fatalf("insecure=%v: unexpected transport type %T", insecure, rt)
				}
			}
			if transport.Proxy == nil {
				t.Fatalf("insecure=%v: transport.Proxy is nil", insecure)
			}
			req, err := http.NewRequest(http.MethodPost, "https://sso.example.com/protocol/openid-connect/token", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			got, err := transport.Proxy(req)
			if err != nil {
				t.Fatalf("insecure=%v: Proxy: %v", insecure, err)
			}
			if got == nil || got.String() != "http://proxy.example.com:3128" {
				t.Errorf("insecure=%v: proxy = %v, want http://proxy.example.com:3128", insecure, got)
			}
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestNewHTTPClientHonorsProxyEnvironment$", "-test.v")
	cmd.Env = append(os.Environ(),
		proxyHelperEnv+"=1",
		"HTTPS_PROXY=http://proxy.example.com:3128",
		"https_proxy=http://proxy.example.com:3128",
		"NO_PROXY=",
		"no_proxy=",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper process failed: %v\n%s", err, out)
	}
}

func TestNewHTTPClientInsecure(t *testing.T) {
	transport, ok := newHTTPClient(true).Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", newHTTPClient(true).Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify with insecure")
	}
}
