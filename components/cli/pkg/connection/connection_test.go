package connection

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
func TestNewTransportHonorsProxyEnvironment(t *testing.T) {
	if os.Getenv(proxyHelperEnv) == "1" {
		for _, insecure := range []bool{false, true} {
			transport := newTransport(insecure)
			if transport.Proxy == nil {
				t.Fatalf("insecure=%v: transport.Proxy is nil", insecure)
			}
			req, err := http.NewRequest(http.MethodGet, "https://api.example.com/api/hypershell/v1/gateways", nil)
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

	cmd := exec.Command(os.Args[0], "-test.run=^TestNewTransportHonorsProxyEnvironment$", "-test.v")
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

func TestNewTransportInsecure(t *testing.T) {
	if cfg := newTransport(false).TLSClientConfig; cfg != nil && cfg.InsecureSkipVerify {
		t.Error("TLS verification disabled without insecure")
	}
	cfg := newTransport(true).TLSClientConfig
	if cfg == nil || !cfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify with insecure")
	}
}
