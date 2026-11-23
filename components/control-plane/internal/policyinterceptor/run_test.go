package policyinterceptor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	gi "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/gatewayinterceptorv1"
)

// Verifies: OSG-CORE-19.01
func TestRunServesDescribeBeforeBundleAndReportsRevision(t *testing.T) {
	kit := newJWTKit(t)
	dir, err := os.MkdirTemp("", "pgprun")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "interceptor.sock")
	// A stale socket from a previous container run is removed at startup.
	stale, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("stale listener: %v", err)
	}
	if ul, ok := stale.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(false)
	}
	_ = stale.Close()

	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("sa-token"), 0o600); err != nil {
		t.Fatalf("token: %v", err)
	}
	env := validEnv()
	env[EnvSocket] = sock
	env[EnvJWTPublicKeyFile] = kit.keyFile
	env[EnvEventsTokenFile] = tokenFile
	env[EnvHealthAddr] = "127.0.0.1:0"
	cfg, err := LoadConfig(envLookup(env))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	kube := fake.NewClientset()
	events := &fakeEventClient{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, Deps{Kube: kube, EventClient: events, Lookup: NoSandboxLookup{}}) }()

	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	client := gi.NewGatewayInterceptorClient(conn)
	authed := withToken(context.Background(), kit.token(t, nil))
	waitFor(t, "Describe over the socket", func() bool {
		_, err := client.Describe(authed, gatewayMeta(1))
		return err == nil
	})
	// No bundle yet: create fails closed.
	if _, err := client.Evaluate(authed, validateEval(t, "CreateSandbox", createOp("x", nil, nil))); err == nil {
		t.Fatalf("create without bundle should fail closed")
	}

	// The bundle arrives through the Kubernetes API and the reporter sends the
	// applied revision.
	raw := buildBundle(t, testGatewayID, false, testPolicy{name: "cpu-cap", mode: "enforce", cedar: cpuOver("1000")})
	if _, err := kube.CoreV1().ConfigMaps(testNamespace).Create(ctx, bundleConfigMap(raw), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create configmap: %v", err)
	}
	waitFor(t, "applied revision reported", func() bool {
		events.mu.Lock()
		defer events.mu.Unlock()
		for _, r := range events.requests {
			if r.GetAppliedRevision() != "" && r.GetGatewayId() == testGatewayID {
				return true
			}
		}
		return false
	})
	res, err := client.Evaluate(authed, validateEval(t, "CreateSandbox", createOp("x", map[string]any{"cpu": "2"}, nil)))
	if err != nil || res.GetAllowed() {
		t.Fatalf("enforced bundle should deny: %+v, %v", res, err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v", err)
	}
}

// Verifies: OSG-CORE-19.01
func TestHealthReadinessFollowsSocket(t *testing.T) {
	h := NewHealth(NewMetrics())
	get := func(path string) int {
		rec := httptest.NewRecorder()
		h.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}
	if get("/healthz") != http.StatusOK || get("/readyz") != http.StatusServiceUnavailable {
		t.Fatalf("before listening: healthz=%d readyz=%d", get("/healthz"), get("/readyz"))
	}
	h.SetReady(true)
	if get("/readyz") != http.StatusOK || get("/metrics") != http.StatusOK {
		t.Fatalf("after listening: readyz=%d metrics=%d", get("/readyz"), get("/metrics"))
	}
}
