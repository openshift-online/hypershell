package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

// helmReleaseClient is the part of helm.ShellClient that gateway deployment
// uses. *helm.ShellClient implements it.
type helmReleaseClient interface {
	GetReleaseStatus(ctx context.Context, namespace string) (*helm.ReleaseStatus, error)
	Install(ctx context.Context, namespace string, values map[string]interface{}) error
	Upgrade(ctx context.Context, namespace string, values map[string]interface{}) error
	Uninstall(ctx context.Context, namespace string) error
	History(ctx context.Context, namespace string) ([]helm.Revision, error)
	Rollback(ctx context.Context, namespace string, rev int) error
	Manifest(ctx context.Context, namespace string, rev int) (string, error)
	PendingStaleAfter() time.Duration
}

// gatewayContainerName is the gateway container in the chart's pod template
// (charts/openshell/templates/_gateway-workload.tpl).
const gatewayContainerName = "openshell-gateway"

// Recovery actions for a stale pending-* release. They appear in
// the log line and the Kubernetes Event.
const (
	pendingActionRollback    = "rollback"
	pendingActionRollForward = "roll-forward"
	pendingActionReinstall   = "reinstall"
)

// HelmOperationInProgressError reports a pending-* release that is still young
// enough for a helm process to own it. The release was not
// changed; the reconcile is retried with reason helm_operation_in_progress.
type HelmOperationInProgressError struct {
	Namespace  string
	Revision   int
	Status     string
	Age        time.Duration
	StaleAfter time.Duration
}

func (e *HelmOperationInProgressError) Error() string {
	return fmt.Sprintf("helm release %s in namespace %s is %s at revision %d, %s old; another helm operation may still own it until it is %s old",
		helm.ReleaseName, e.Namespace, e.Status, e.Revision, e.Age.Round(time.Second), e.StaleAfter)
}

// recoverPendingRelease handles a release whose latest revision is pending-*
// and leaves it deployed at the desired values:
//
//   - young: return HelmOperationInProgressError and change nothing;
//   - stale pending-install with no earlier good revision: uninstall, then install;
//   - stale, and a gateway pod of the pending revision's image has started (the
//     gateway database may be migrated), or no earlier good revision exists:
//     roll forward with `helm rollback <pending revision>`, then upgrade;
//   - otherwise: `helm rollback <last good revision>`, then upgrade.
func recoverPendingRelease(
	ctx context.Context,
	helmClient helmReleaseClient,
	clientset kubernetes.Interface,
	namespace string,
	values map[string]interface{},
	now time.Time,
) error {
	history, err := helmClient.History(ctx, namespace)
	if err != nil {
		return fmt.Errorf("read helm history of stuck release: %w", err)
	}
	if len(history) == 0 {
		return fmt.Errorf("helm release %s in namespace %s is pending but has no history", helm.ReleaseName, namespace)
	}
	pending := history[len(history)-1]
	if !strings.HasPrefix(pending.Status, "pending-") {
		// The operation finished between `helm status` and `helm history`.
		// Retry so the next reconcile handles the new status.
		return &HelmOperationInProgressError{Namespace: namespace, Revision: pending.Revision, Status: pending.Status, Age: now.Sub(pending.Updated), StaleAfter: helmClient.PendingStaleAfter()}
	}

	age := now.Sub(pending.Updated)
	staleAfter := helmClient.PendingStaleAfter()
	if age < staleAfter {
		slog.InfoContext(ctx, "Helm release is pending and may still be owned by another helm; waiting",
			"namespace", namespace, "status", pending.Status, "revision", pending.Revision,
			"age", age.Round(time.Second).String(), "stale_after", staleAfter.String())
		return &HelmOperationInProgressError{Namespace: namespace, Revision: pending.Revision, Status: pending.Status, Age: age, StaleAfter: staleAfter}
	}

	var lastGood *helm.Revision
	for i := len(history) - 2; i >= 0; i-- {
		if history[i].Status == "deployed" || history[i].Status == "superseded" {
			lastGood = &history[i]
			break
		}
	}

	action := pendingActionRollback
	target := 0
	switch {
	case lastGood == nil && pending.Status == "pending-install":
		action = pendingActionReinstall
	case lastGood == nil:
		// Nothing good to go back to; take the pending revision instead of
		// discarding the release (only a first install may be uninstalled).
		action, target = pendingActionRollForward, pending.Revision
	default:
		started, err := pendingGatewayPodStarted(ctx, helmClient, clientset, namespace, pending.Revision)
		if err != nil {
			return err
		}
		if started {
			action, target = pendingActionRollForward, pending.Revision
		} else {
			target = lastGood.Revision
		}
	}

	msg := fmt.Sprintf("Helm release %s is %s at revision %d, %s old (stale after %s); recovering by %s",
		helm.ReleaseName, pending.Status, pending.Revision, age.Round(time.Second), staleAfter, action)
	if target > 0 {
		msg += fmt.Sprintf(" to revision %d, then upgrade", target)
	}
	slog.WarnContext(ctx, "Recovering stuck helm release",
		"namespace", namespace, "status", pending.Status, "revision", pending.Revision,
		"age", age.Round(time.Second).String(), "action", action, "target_revision", target)
	recordHelmRecoveryEvent(ctx, clientset, namespace, action, msg, now)

	if action == pendingActionReinstall {
		if err := helmClient.Uninstall(ctx, namespace); err != nil {
			return fmt.Errorf("uninstall stuck helm release: %w", err)
		}
		if err := helmClient.Install(ctx, namespace, values); err != nil {
			return fmt.Errorf("reinstall helm release: %w", err)
		}
		return nil
	}
	if err := helmClient.Rollback(ctx, namespace, target); err != nil {
		return fmt.Errorf("recover stuck helm release by %s: %w", action, err)
	}
	if err := helmClient.Upgrade(ctx, namespace, values); err != nil {
		return fmt.Errorf("helm upgrade after %s: %w", action, err)
	}
	return nil
}

// pendingGatewayPodStarted reports whether a gateway container running the
// pending revision's gateway image has started. Such a pod may have migrated
// the gateway database, and migrations only go forward. If
// the pending manifest has no recognisable gateway workload, it returns true:
// rolling forward never starts an earlier gateway image, rolling back might.
func pendingGatewayPodStarted(ctx context.Context, helmClient helmReleaseClient, clientset kubernetes.Interface, namespace string, rev int) (bool, error) {
	manifest, err := helmClient.Manifest(ctx, namespace, rev)
	if err != nil {
		return false, fmt.Errorf("read manifest of pending revision: %w", err)
	}
	image, selector, ok := gatewayWorkloadFromManifest(manifest)
	if !ok {
		slog.WarnContext(ctx, "Pending helm revision has no gateway workload; assuming its pod started",
			"namespace", namespace, "revision", rev)
		return true, nil
	}
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(selector).String(),
	})
	if err != nil {
		return false, fmt.Errorf("list gateway pods: %w", err)
	}
	for i := range pods.Items {
		if gatewayContainerStarted(&pods.Items[i], image) {
			return true, nil
		}
	}
	return false, nil
}

// gatewayContainerStarted reports whether pod's gateway container runs image
// and has started at least once.
func gatewayContainerStarted(pod *corev1.Pod, image string) bool {
	matches := false
	for _, c := range pod.Spec.Containers {
		if c.Name == gatewayContainerName && c.Image == image {
			matches = true
		}
	}
	if !matches {
		return false
	}
	for _, s := range pod.Status.ContainerStatuses {
		if s.Name != gatewayContainerName {
			continue
		}
		if s.State.Running != nil || s.State.Terminated != nil || s.LastTerminationState.Terminated != nil || s.RestartCount > 0 {
			return true
		}
	}
	return false
}

// gatewayWorkloadFromManifest finds the Deployment or StatefulSet whose pod
// template has the gateway container, and returns that container's image and
// the workload's selector labels.
func gatewayWorkloadFromManifest(manifest string) (string, map[string]string, bool) {
	for _, doc := range strings.Split(manifest, "\n---") {
		var obj struct {
			Kind string `json:"kind"`
			Spec struct {
				Selector struct {
					MatchLabels map[string]string `json:"matchLabels"`
				} `json:"selector"`
				Template struct {
					Spec struct {
						Containers []struct {
							Name  string `json:"name"`
							Image string `json:"image"`
						} `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			continue
		}
		if obj.Kind != "Deployment" && obj.Kind != "StatefulSet" {
			continue
		}
		if len(obj.Spec.Selector.MatchLabels) == 0 {
			continue
		}
		for _, c := range obj.Spec.Template.Spec.Containers {
			if c.Name == gatewayContainerName && c.Image != "" {
				return c.Image, obj.Spec.Selector.MatchLabels, true
			}
		}
	}
	return "", nil, false
}

// recordHelmRecoveryEvent writes a Kubernetes Event on the gateway Deployment
// for a stuck-release recovery. A failed write is logged and
// does not block the recovery.
func recordHelmRecoveryEvent(ctx context.Context, clientset kubernetes.Interface, namespace, action, message string, now time.Time) {
	ts := metav1.NewTime(now)
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "openshell-gateway-helm-recovery-",
			Namespace:    namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       GatewayDeploymentName,
			Namespace:  namespace,
		},
		Reason:         "HelmReleaseRecovery",
		Action:         action,
		Message:        message,
		Type:           corev1.EventTypeWarning,
		Source:         corev1.EventSource{Component: "hypershell-control-plane"},
		FirstTimestamp: ts,
		LastTimestamp:  ts,
		Count:          1,
	}
	if _, err := clientset.CoreV1().Events(namespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		slog.WarnContext(ctx, "Failed to record helm recovery Event", "namespace", namespace, "action", action, "error", err)
	}
}
