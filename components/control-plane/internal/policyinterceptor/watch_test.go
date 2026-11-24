package policyinterceptor

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
)

const testNamespace = "gw-ns"

func bundleConfigMap(raw []byte) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: policybundle.ConfigMapName, Namespace: testNamespace},
		Data:       map[string]string{policybundle.DataKey: string(raw)},
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Verifies: PLAT-PGP-12.01, OSG-CORE-19.02
func TestNewRevisionAppliesWithoutRestart(t *testing.T) {
	first := buildBundle(t, testGatewayID, false, testPolicy{name: "cpu-cap", mode: "audit", cedar: cpuOver("4000")})
	second := buildBundle(t, testGatewayID, false, testPolicy{name: "cpu-cap", mode: "enforce", cedar: cpuOver("1000")})
	client := fake.NewClientset(bundleConfigMap(first))

	srv := newTestServer(t)
	applied := make(chan string, 4)
	w := &BundleWatcher{Client: client, Namespace: testNamespace, Store: srv.Store, Metrics: srv.Metrics,
		OnApplied: func(rev string) { applied <- rev }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	waitFor(t, "first revision", func() bool { return srv.Store.AppliedRevision() != "" })
	firstRev := srv.Store.AppliedRevision()
	op := createOp("s1", map[string]any{"cpu": "2"}, nil)
	if res, err := srv.Evaluate(ctx, validateEval(t, "CreateSandbox", op)); err != nil || !res.GetAllowed() {
		t.Fatalf("first revision should allow: %+v, %v", res, err)
	}

	start := time.Now()
	if _, err := client.CoreV1().ConfigMaps(testNamespace).Update(ctx, bundleConfigMap(second), metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update configmap: %v", err)
	}
	waitFor(t, "second revision", func() bool { return srv.Store.AppliedRevision() != firstRev })
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("revision applied after %s", elapsed)
	}
	if res, err := srv.Evaluate(ctx, validateEval(t, "CreateSandbox", op)); err != nil || res.GetAllowed() {
		t.Fatalf("second revision should deny: %+v, %v", res, err)
	}
	if got := len(applied); got != 2 {
		t.Fatalf("OnApplied called %d times, want 2", got)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("watcher: %v", err)
	}
}

// Verifies: PLAT-PGP-03.02
func TestBundleForOtherGatewayOrSchemaIsRejected(t *testing.T) {
	good := buildBundle(t, testGatewayID, false, testPolicy{name: "cpu-cap", mode: "enforce", cedar: cpuOver("1000")})
	srv := newTestServer(t)
	w := &BundleWatcher{Store: srv.Store, Metrics: srv.Metrics}
	ctx := context.Background()

	// Before any good bundle: a foreign bundle leaves the store empty, so
	// fail_closed bindings still fail closed.
	w.Apply(ctx, bundleConfigMap(buildBundle(t, "gw-other", false, testPolicy{name: "p", mode: "enforce", cedar: cpuOver("4000")})))
	if srv.Store.Current() != nil {
		t.Fatalf("foreign bundle was applied")
	}
	if err := srv.Store.LastError(); err == nil || !strings.Contains(err.Error(), "gw-other") {
		t.Fatalf("foreign bundle not reported: %v", err)
	}

	w.Apply(ctx, bundleConfigMap(good))
	rev := srv.Store.AppliedRevision()
	if rev == "" {
		t.Fatalf("good bundle not applied: %v", srv.Store.LastError())
	}

	unknownSchema := strings.Replace(string(good), `"schemaVersion": "v1"`, `"schemaVersion": "v99"`, 1)
	w.Apply(ctx, bundleConfigMap([]byte(unknownSchema)))
	if err := srv.Store.LastError(); err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("unknown schema not reported: %v", err)
	}
	if srv.Store.AppliedRevision() != rev {
		t.Fatalf("rejected bundle replaced the applied revision")
	}
	w.Apply(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: policybundle.ConfigMapName}})
	if srv.Store.AppliedRevision() != rev {
		t.Fatalf("empty configmap replaced the applied revision")
	}
}
