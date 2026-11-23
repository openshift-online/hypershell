package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

// pendingFakeHelm stands in for the helm CLI. It answers `status`,
// `history` and `get manifest` from files in FAKE_HELM_DIR, records one line
// per call in $FAKE_HELM_DIR/calls, and succeeds for every mutating command
// unless $FAKE_HELM_DIR/fail-<command> exists.
const pendingFakeHelm = `#!/bin/sh
dir="$FAKE_HELM_DIR"
case "$1" in
  get) echo "get-$2 $(echo "$@" | sed -n 's/.*--revision \([0-9]*\).*/\1/p')" >> "$dir/calls" ;;
  rollback) echo "rollback $3" >> "$dir/calls" ;;
  *) echo "$1" >> "$dir/calls" ;;
esac
if [ -e "$dir/fail-$1" ]; then echo "Error: injected $1 failure" >&2; exit 1; fi
case "$1" in
  status)
    if [ ! -e "$dir/status.json" ]; then echo 'Error: release: not found' >&2; exit 1; fi
    cat "$dir/status.json" ;;
  history) cat "$dir/history.json" ;;
  get) cat "$dir/manifest" ;;
esac
exit 0
`

const (
	oldGatewayImage     = "quay.io/example/openshell-gateway:v1"
	pendingGatewayImage = "quay.io/example/openshell-gateway:v2"
)

var gatewaySelector = map[string]string{
	"app.kubernetes.io/name":     "openshell",
	"app.kubernetes.io/instance": "openshell-gateway",
}

// pendingManifest is a trimmed `helm get manifest` of a revision whose
// gateway Deployment runs image.
func pendingManifest(image string) string {
	return fmt.Sprintf(`---
# Source: openshell/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: openshell-gateway
spec:
  selector:
    app.kubernetes.io/name: openshell
---
# Source: openshell/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openshell-gateway
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: openshell
      app.kubernetes.io/instance: openshell-gateway
  template:
    spec:
      initContainers:
        - name: wait
          image: %[1]s
      containers:
        - name: openshell-gateway
          image: %[1]s
`, image)
}

type histEntry struct {
	rev    int
	status string
	age    time.Duration
}

type pendingCase struct {
	name     string
	status   string // `helm status` result; "" means no release
	history  []histEntry
	manifest string
	pods     []corev1.Pod
	failCmd  string

	wantCalls      []string
	wantInProgress bool
	wantErr        bool
	wantAction     string // the recorded Event's action; "" means no Event
}

func gatewayPod(name, image string, state corev1.ContainerState, restarts int32) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "gw-ns", Labels: gatewaySelector},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "openshell-gateway", Image: image},
		}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			{Name: "openshell-gateway", Image: image, State: state, RestartCount: restarts},
		}},
	}
}

var (
	running = corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}
	waiting = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}
	crashed = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}
)

// stale is older than the stale window of a default ShellClient (6m + 60s).
const stale = 10 * time.Minute

func runPendingCase(t *testing.T, tc pendingCase) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "helm")
	if err := os.WriteFile(bin, []byte(pendingFakeHelm), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_HELM_DIR", dir)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if tc.status != "" {
		latest := tc.history[len(tc.history)-1].rev
		write("status.json", fmt.Sprintf(`{"name":"openshell-gateway","namespace":"gw-ns","info":{"status":%q},"version":%d}`, tc.status, latest))
	}
	now := time.Now()
	var hist []map[string]interface{}
	for _, h := range tc.history {
		hist = append(hist, map[string]interface{}{
			"revision":    h.rev,
			"updated":     now.Add(-h.age).Format(time.RFC3339Nano),
			"status":      h.status,
			"chart":       "openshell-0.1.8",
			"app_version": "v0.1.8",
			"description": "test",
		})
	}
	histJSON, _ := json.Marshal(hist)
	write("history.json", string(histJSON))
	write("manifest", tc.manifest)
	if tc.failCmd != "" {
		write("fail-"+tc.failCmd, "")
	}

	clientset := fake.NewSimpleClientset()
	for i := range tc.pods {
		if _, err := clientset.CoreV1().Pods("gw-ns").Create(context.Background(), &tc.pods[i], metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	c := &helm.ShellClient{HelmBinary: bin, ChartPath: "/charts/openshell.tgz"}
	nsConfig := NamespaceConfig{Name: "gw-ns", Gateway: GatewayConfig{
		Image:            pendingGatewayImage,
		CredentialDriver: &CredentialDriverConfig{Type: "kek-db"},
		WorkspaceMode:    "shared",
	}}
	err := deployGatewayViaHelm(context.Background(), c, clientset, nsConfig, ReconcileOpts{}, false, false, "")

	var inProgress *HelmOperationInProgressError
	if got := errors.As(err, &inProgress); got != tc.wantInProgress {
		t.Errorf("HelmOperationInProgressError = %v, want %v (err: %v)", got, tc.wantInProgress, err)
	}
	if wantErr := tc.wantErr || tc.wantInProgress; (err != nil) != wantErr {
		t.Errorf("err = %v, want error %v", err, wantErr)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "calls"))
	calls := strings.Fields(strings.ReplaceAll(strings.TrimSpace(string(data)), " ", "_"))
	if strings.Join(calls, ",") != strings.Join(tc.wantCalls, ",") {
		t.Errorf("helm calls = %v, want %v", calls, tc.wantCalls)
	}

	events, err := clientset.CoreV1().Events("gw-ns").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if tc.wantAction == "" {
		if len(events.Items) != 0 {
			t.Errorf("recorded %d Events, want none: %+v", len(events.Items), events.Items)
		}
		return
	}
	if len(events.Items) != 1 {
		t.Fatalf("recorded %d Events, want 1", len(events.Items))
	}
	ev := events.Items[0]
	if ev.Action != tc.wantAction || ev.Reason != "HelmReleaseRecovery" || ev.InvolvedObject.Name != GatewayDeploymentName {
		t.Errorf("Event = action %q reason %q object %q, want action %q on the gateway Deployment", ev.Action, ev.Reason, ev.InvolvedObject.Name, tc.wantAction)
	}
	pendingRev := tc.history[len(tc.history)-1]
	for _, want := range []string{"gw-ns", fmt.Sprintf("revision %d", pendingRev.rev), pendingRev.status, "old", tc.wantAction} {
		if !strings.Contains(ev.Message, want) && !strings.Contains(ev.Namespace, want) {
			t.Errorf("Event message %q does not name %q", ev.Message, want)
		}
	}
}

// Stuck release recovery: the controller handles each release status per
// OSG-HELM-01.03 and recovers a pending-* release.
func TestDeployGatewayViaHelmPendingRecovery(t *testing.T) {
	upgraded := []histEntry{{1, "superseded", time.Hour}, {2, "deployed", 30 * time.Minute}}
	cases := []pendingCase{
		{
			name:      "no release installs",
			wantCalls: []string{"status", "install"},
		},
		{
			name:      "deployed upgrades",
			status:    "deployed",
			history:   upgraded,
			wantCalls: []string{"status", "upgrade"},
		},
		{
			name:      "failed upgrades",
			status:    "failed",
			history:   append(upgraded[:2:2], histEntry{3, "failed", stale}),
			wantCalls: []string{"status", "upgrade"},
		},
		{
			// A young pending release may still be owned by another helm.
			name:           "young pending upgrade is left alone",
			status:         "pending-upgrade",
			history:        append(upgraded[:2:2], histEntry{3, "pending-upgrade", 30 * time.Second}),
			manifest:       pendingManifest(pendingGatewayImage),
			wantCalls:      []string{"status", "history"},
			wantInProgress: true,
		},
		{
			// A helm in the old controller pod can run for 6m; at 6m30s it may still own the release.
			name:           "pending upgrade inside the helm process budget is left alone",
			status:         "pending-upgrade",
			history:        append(upgraded[:2:2], histEntry{3, "pending-upgrade", 6*time.Minute + 30*time.Second}),
			wantCalls:      []string{"status", "history"},
			wantInProgress: true,
		},
		{
			name:           "young pending install is left alone",
			status:         "pending-install",
			history:        []histEntry{{1, "pending-install", time.Minute}},
			wantCalls:      []string{"status", "history"},
			wantInProgress: true,
		},
		{
			name:           "release that finished since helm status is retried, not touched",
			status:         "pending-upgrade",
			history:        append(upgraded[:2:2], histEntry{3, "deployed", stale}),
			wantCalls:      []string{"status", "history"},
			wantInProgress: true,
		},
		{
			// Roll back: no pod of the pending image exists.
			name:       "stale pending upgrade with no new pod rolls back then upgrades",
			status:     "pending-upgrade",
			history:    append(upgraded[:2:2], histEntry{3, "pending-upgrade", stale}),
			manifest:   pendingManifest(pendingGatewayImage),
			pods:       []corev1.Pod{gatewayPod("gw-old", oldGatewayImage, running, 0)},
			wantCalls:  []string{"status", "history", "get-manifest_3", "rollback_2", "upgrade"},
			wantAction: "rollback",
		},
		{
			// Roll back: the new pod exists but its gateway container never started.
			name:     "stale pending upgrade whose new pod never started rolls back",
			status:   "pending-upgrade",
			history:  append(upgraded[:2:2], histEntry{3, "pending-upgrade", stale}),
			manifest: pendingManifest(pendingGatewayImage),
			pods: []corev1.Pod{
				gatewayPod("gw-old", oldGatewayImage, running, 0),
				gatewayPod("gw-new", pendingGatewayImage, waiting, 0),
			},
			wantCalls:  []string{"status", "history", "get-manifest_3", "rollback_2", "upgrade"},
			wantAction: "rollback",
		},
		{
			name:       "stale pending upgrade after a failed revision rolls back to the last good one",
			status:     "pending-upgrade",
			history:    []histEntry{{1, "superseded", time.Hour}, {2, "failed", time.Hour}, {3, "pending-upgrade", stale}},
			manifest:   pendingManifest(pendingGatewayImage),
			wantCalls:  []string{"status", "history", "get-manifest_3", "rollback_1", "upgrade"},
			wantAction: "rollback",
		},
		{
			// Roll forward: the new gateway pod may have migrated the database.
			name:     "stale pending upgrade whose new pod is running rolls forward",
			status:   "pending-upgrade",
			history:  append(upgraded[:2:2], histEntry{3, "pending-upgrade", stale}),
			manifest: pendingManifest(pendingGatewayImage),
			pods: []corev1.Pod{
				gatewayPod("gw-old", oldGatewayImage, running, 0),
				gatewayPod("gw-new", pendingGatewayImage, running, 0),
			},
			wantCalls:  []string{"status", "history", "get-manifest_3", "rollback_3", "upgrade"},
			wantAction: "roll-forward",
		},
		{
			// Roll forward: a crash-looping new pod has still started (and may have migrated).
			name:       "stale pending upgrade whose new pod crash-loops rolls forward",
			status:     "pending-upgrade",
			history:    append(upgraded[:2:2], histEntry{3, "pending-upgrade", stale}),
			manifest:   pendingManifest(pendingGatewayImage),
			pods:       []corev1.Pod{gatewayPod("gw-new", pendingGatewayImage, crashed, 3)},
			wantCalls:  []string{"status", "history", "get-manifest_3", "rollback_3", "upgrade"},
			wantAction: "roll-forward",
		},
		{
			name:       "stale pending upgrade with an unreadable gateway workload rolls forward",
			status:     "pending-upgrade",
			history:    append(upgraded[:2:2], histEntry{3, "pending-upgrade", stale}),
			manifest:   "---\nkind: Service\n",
			wantCalls:  []string{"status", "history", "get-manifest_3", "rollback_3", "upgrade"},
			wantAction: "roll-forward",
		},
		{
			// Only a first install with nothing good to keep is reinstalled.
			name:       "stale first install is reinstalled",
			status:     "pending-install",
			history:    []histEntry{{1, "pending-install", stale}},
			wantCalls:  []string{"status", "history", "uninstall", "install"},
			wantAction: "reinstall",
		},
		{
			name:       "stale pending upgrade with no good revision rolls forward, never uninstalls",
			status:     "pending-upgrade",
			history:    []histEntry{{1, "failed", time.Hour}, {2, "pending-upgrade", stale}},
			wantCalls:  []string{"status", "history", "rollback_2", "upgrade"},
			wantAction: "roll-forward",
		},
		{
			name:       "stale pending rollback is recovered like a pending upgrade",
			status:     "pending-rollback",
			history:    append(upgraded[:2:2], histEntry{3, "pending-rollback", stale}),
			manifest:   pendingManifest(oldGatewayImage),
			wantCalls:  []string{"status", "history", "get-manifest_3", "rollback_2", "upgrade"},
			wantAction: "rollback",
		},
		{
			name:       "failed rollback is returned and does not upgrade",
			status:     "pending-upgrade",
			history:    append(upgraded[:2:2], histEntry{3, "pending-upgrade", stale}),
			manifest:   pendingManifest(pendingGatewayImage),
			failCmd:    "rollback",
			wantCalls:  []string{"status", "history", "get-manifest_3", "rollback_2"},
			wantErr:    true,
			wantAction: "rollback",
		},
		{
			name:      "unreadable history is returned and changes nothing",
			status:    "pending-upgrade",
			history:   append(upgraded[:2:2], histEntry{3, "pending-upgrade", stale}),
			failCmd:   "history",
			wantCalls: []string{"status", "history"},
			wantErr:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runPendingCase(t, tc) })
	}
}

func TestGatewayWorkloadFromManifest(t *testing.T) {
	image, selector, ok := gatewayWorkloadFromManifest(pendingManifest(pendingGatewayImage))
	if !ok || image != pendingGatewayImage || selector["app.kubernetes.io/instance"] != "openshell-gateway" {
		t.Fatalf("got image %q selector %v ok %v", image, selector, ok)
	}
	if _, _, ok := gatewayWorkloadFromManifest("---\nkind: Service\n"); ok {
		t.Fatal("found a gateway workload in a manifest without one")
	}
}
