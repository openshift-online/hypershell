//go:build helmlive

package gateway

// Stuck-release recovery against a real helm and a real Kubernetes API server.
// It strands a release the way a controller crash does (kill -9 of a running
// `helm upgrade --wait`) and runs the recovery on it. It needs:
//
//	HELM_LIVE_BIN         the helm binary (build it from tools/helm)
//	KUBECONFIG            an API server; an envtest one is enough, since no pod
//	                      has to run (the test writes the pod status itself)
//
//	go test -tags helmlive ./internal/gateway -run TestHelmPendingRecoveryLive -v

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

const liveChartYAML = `apiVersion: v2
name: openshell
version: 0.1.0
`

const liveDeploymentYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: openshell-gateway
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: openshell
      app.kubernetes.io/instance: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: openshell
        app.kubernetes.io/instance: {{ .Release.Name }}
    spec:
      containers:
        - name: openshell-gateway
          image: {{ .Values.image | quote }}
`

const (
	liveV1 = "example.com/openshell-gateway:v1"
	liveV2 = "example.com/openshell-gateway:v2"
	liveV3 = "example.com/openshell-gateway:v3"
)

type liveEnv struct {
	t         *testing.T
	helmBin   string
	chart     string
	clientset kubernetes.Interface
	client    *helm.ShellClient
}

func newLiveEnv(t *testing.T) *liveEnv {
	bin := os.Getenv("HELM_LIVE_BIN")
	if bin == "" || os.Getenv("KUBECONFIG") == "" {
		t.Skip("HELM_LIVE_BIN and KUBECONFIG are required")
	}
	chart := t.TempDir()
	must(t, os.WriteFile(filepath.Join(chart, "Chart.yaml"), []byte(liveChartYAML), 0o644))
	must(t, os.MkdirAll(filepath.Join(chart, "templates"), 0o755))
	must(t, os.WriteFile(filepath.Join(chart, "templates", "deployment.yaml"), []byte(liveDeploymentYAML), 0o644))
	cs, err := kubernetes.NewForConfig(restConfigFromKubeconfig(t, os.Getenv("KUBECONFIG")))
	must(t, err)
	return &liveEnv{t: t, helmBin: bin, chart: chart, clientset: cs,
		client: &helm.ShellClient{HelmBinary: bin, ChartPath: chart}}
}

// restConfigFromKubeconfig reads the first cluster and user of a kubeconfig
// with embedded certificate data, such as one written for an envtest server.
func restConfigFromKubeconfig(t *testing.T, path string) *rest.Config {
	data, err := os.ReadFile(path)
	must(t, err)
	var kc struct {
		Clusters []struct {
			Cluster struct {
				Server string `json:"server"`
				CA     []byte `json:"certificate-authority-data"`
			} `json:"cluster"`
		} `json:"clusters"`
		Users []struct {
			User struct {
				Cert []byte `json:"client-certificate-data"`
				Key  []byte `json:"client-key-data"`
			} `json:"user"`
		} `json:"users"`
	}
	must(t, yaml.Unmarshal(data, &kc))
	if len(kc.Clusters) == 0 || len(kc.Users) == 0 {
		t.Fatalf("kubeconfig %s has no cluster or user", path)
	}
	return &rest.Config{Host: kc.Clusters[0].Cluster.Server, TLSClientConfig: rest.TLSClientConfig{
		CAData: kc.Clusters[0].Cluster.CA, CertData: kc.Users[0].User.Cert, KeyData: kc.Users[0].User.Key}}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (e *liveEnv) namespace(name string) string {
	ctx := context.Background()
	_, err := e.clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{})
	must(e.t, err)
	return name
}

// strand starts `helm <op> --wait` (which never finishes: no controller makes
// the Deployment ready), waits until helm has applied the new pod template and
// recorded the revision as pending, and kills helm with SIGKILL.
func (e *liveEnv) strand(ns, op, image string, wantStatus string) {
	t := e.t
	cmd := exec.Command(e.helmBin, op, helm.ReleaseName, e.chart, "--namespace", ns,
		"--set", "image="+image, "--wait", "--timeout", "5m")
	must(t, cmd.Start())
	deadline := time.Now().Add(60 * time.Second)
	for {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("helm %s never reached %s with image %s", op, wantStatus, image)
		}
		status, err := e.client.GetReleaseStatus(context.Background(), ns)
		dep, depErr := e.clientset.AppsV1().Deployments(ns).Get(context.Background(), GatewayDeploymentName, metav1.GetOptions{})
		if err == nil && status != nil && status.Status == wantStatus && depErr == nil &&
			dep.Spec.Template.Spec.Containers[0].Image == image {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	must(t, cmd.Process.Kill())
	_ = cmd.Wait()
}

func (e *liveEnv) helmOut(args ...string) (string, error) {
	out, err := exec.Command(e.helmBin, args...).CombinedOutput()
	return string(out), err
}

func (e *liveEnv) deploymentUID(ns string) string {
	dep, err := e.clientset.AppsV1().Deployments(ns).Get(context.Background(), GatewayDeploymentName, metav1.GetOptions{})
	must(e.t, err)
	return string(dep.UID)
}

func (e *liveEnv) deploymentImage(ns string) string {
	dep, err := e.clientset.AppsV1().Deployments(ns).Get(context.Background(), GatewayDeploymentName, metav1.GetOptions{})
	must(e.t, err)
	return dep.Spec.Template.Spec.Containers[0].Image
}

func (e *liveEnv) manifestImage(ns string, rev int) string {
	m, err := e.client.Manifest(context.Background(), ns, rev)
	must(e.t, err)
	image, _, ok := gatewayWorkloadFromManifest(m)
	if !ok {
		e.t.Fatalf("revision %d has no gateway workload", rev)
	}
	return image
}

// startedPod writes a gateway pod of image whose gateway container is running,
// as the Deployment's ReplicaSet and kubelet would.
func (e *liveEnv) startedPod(ns, image string) {
	ctx := context.Background()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "gw-new", Namespace: ns, Labels: map[string]string{
			"app.kubernetes.io/name": "openshell", "app.kubernetes.io/instance": helm.ReleaseName}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: gatewayContainerName, Image: image}}},
	}
	created, err := e.clientset.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	must(e.t, err)
	created.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: gatewayContainerName, Image: image,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}}}}
	_, err = e.clientset.CoreV1().Pods(ns).UpdateStatus(ctx, created, metav1.UpdateOptions{})
	must(e.t, err)
}

func (e *liveEnv) history(ns string) []helm.Revision {
	h, err := e.client.History(context.Background(), ns)
	must(e.t, err)
	return h
}

func statuses(h []helm.Revision) string {
	var s []string
	for _, r := range h {
		s = append(s, r.Status)
	}
	return strings.Join(s, ",")
}

func TestHelmPendingRecoveryLive(t *testing.T) {
	e := newLiveEnv(t)
	ctx := context.Background()
	later := time.Now().Add(time.Hour) // past the stale window
	desired := map[string]interface{}{"image": liveV3}

	// Helm itself refuses an upgrade on the stranded release; recovery is needed.
	setup := func(ns string) string {
		e.namespace(ns)
		must(t, e.client.Install(ctx, ns, map[string]interface{}{"image": liveV1}))
		uid := e.deploymentUID(ns)
		e.strand(ns, "upgrade", liveV2, "pending-upgrade")
		if out, err := e.helmOut("upgrade", helm.ReleaseName, e.chart, "-n", ns, "--set", "image="+liveV3); err == nil ||
			!strings.Contains(out, "another operation") {
			t.Fatalf("helm upgrade on a pending release: err %v, output %s", err, out)
		}
		return uid
	}

	t.Run("young pending upgrade is left alone", func(t *testing.T) {
		ns := "live-young"
		setup(ns)
		before := statuses(e.history(ns))
		err := recoverPendingRelease(ctx, e.client, e.clientset, ns, desired, time.Now())
		var inProgress *HelmOperationInProgressError
		if !errors.As(err, &inProgress) {
			t.Fatalf("want HelmOperationInProgressError, got %v", err)
		}
		if after := statuses(e.history(ns)); after != before {
			t.Fatalf("history changed from %s to %s", before, after)
		}
	})

	t.Run("stale pending upgrade with no new pod rolls back then upgrades", func(t *testing.T) {
		ns := "live-rollback"
		uid := setup(ns)
		must(t, recoverPendingRelease(ctx, e.client, e.clientset, ns, desired, later))
		h := e.history(ns)
		// 1 install, 2 stranded upgrade, 3 rollback to 1, 4 upgrade.
		if got := statuses(h); got != "superseded,pending-upgrade,superseded,deployed" {
			t.Fatalf("history = %s", got)
		}
		if img := e.manifestImage(ns, 3); img != liveV1 {
			t.Fatalf("revision 3 image = %s, want the rollback to %s", img, liveV1)
		}
		if img := e.deploymentImage(ns); img != liveV3 {
			t.Fatalf("deployment image = %s, want %s", img, liveV3)
		}
		if e.deploymentUID(ns) != uid {
			t.Fatal("gateway Deployment was recreated")
		}
	})

	t.Run("stale pending upgrade whose new pod started rolls forward", func(t *testing.T) {
		ns := "live-forward"
		uid := setup(ns)
		e.startedPod(ns, liveV2)
		must(t, recoverPendingRelease(ctx, e.client, e.clientset, ns, desired, later))
		h := e.history(ns)
		// 1 install, 2 stranded upgrade, 3 rollback to 2 (forward), 4 upgrade.
		if got := statuses(h); got != "superseded,pending-upgrade,superseded,deployed" {
			t.Fatalf("history = %s", got)
		}
		for _, r := range h[2:] {
			if img := e.manifestImage(ns, r.Revision); img == liveV1 {
				t.Fatalf("revision %d applied the earlier gateway image %s", r.Revision, img)
			}
		}
		if img := e.manifestImage(ns, 3); img != liveV2 {
			t.Fatalf("revision 3 image = %s, want the pending image %s", img, liveV2)
		}
		if img := e.deploymentImage(ns); img != liveV3 {
			t.Fatalf("deployment image = %s, want %s", img, liveV3)
		}
		if e.deploymentUID(ns) != uid {
			t.Fatal("gateway Deployment was recreated")
		}
	})

	t.Run("stale first install is reinstalled", func(t *testing.T) {
		ns := e.namespace("live-install")
		e.strand(ns, "install", liveV1, "pending-install")
		must(t, recoverPendingRelease(ctx, e.client, e.clientset, ns, desired, later))
		if got := statuses(e.history(ns)); got != "deployed" {
			t.Fatalf("history = %s", got)
		}
		if img := e.deploymentImage(ns); img != liveV3 {
			t.Fatalf("deployment image = %s, want %s", img, liveV3)
		}
	})

	t.Run("recovery Events name the action", func(t *testing.T) {
		for ns, action := range map[string]string{"live-rollback": "rollback", "live-forward": "roll-forward", "live-install": "reinstall"} {
			events, err := e.clientset.CoreV1().Events(ns).List(ctx, metav1.ListOptions{})
			must(t, err)
			if len(events.Items) != 1 || events.Items[0].Action != action {
				t.Fatalf("%s: Events = %+v, want one %s", ns, events.Items, action)
			}
			t.Logf("%s Event: %s", ns, events.Items[0].Message)
		}
	})
}
