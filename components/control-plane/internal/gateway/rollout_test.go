package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// gatewayDeployment builds a gateway Deployment fixture with the given spec and
// observed status, so the revision-aware readiness helpers can be exercised
// against the same fields the Deployment controller reports during a rollout.
func gatewayDeployment(replicas, generation, observedGeneration, updated, total, available int32) *appsv1.Deployment {
	r := replicas
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       GatewayDeploymentName,
			Namespace:  "gw-ns",
			Generation: int64(generation),
		},
		Spec: appsv1.DeploymentSpec{Replicas: &r},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: int64(observedGeneration),
			UpdatedReplicas:    updated,
			Replicas:           total,
			AvailableReplicas:  available,
		},
	}
}

// TestDeploymentRolloutComplete pins the revision-aware readiness decision: it is
// judged on the *new* revision (observed generation caught up, updated replicas
// available at the desired count, no old replicas left), and it distinguishes an
// in-progress rollout from a steady-state degradation of the current revision.
// See gateway-release-rollout.spec.md.
func TestDeploymentRolloutComplete(t *testing.T) {
	tests := []struct {
		name               string
		deploy             *appsv1.Deployment
		wantComplete       bool
		wantRollingOut     bool
		wantReasonContains string
	}{
		{
			// Success: the new revision is fully rolled out and available.
			name:         "new revision fully available",
			deploy:       gatewayDeployment(1, 2, 2, 1, 1, 1),
			wantComplete: true,
		},
		{
			// Rollout in progress: the Deployment controller has not yet observed
			// the new spec. This is the window the health loop must defer to the
			// provisioning path rather than flapping the phase.
			name:           "spec update not yet observed",
			deploy:         gatewayDeployment(1, 3, 2, 1, 1, 1),
			wantRollingOut: true,
		},
		{
			// Rollout in progress: the new revision's pod is not yet updated. A
			// still-Ready old pod (available=1) must NOT satisfy the gate.
			name:           "updated replica not yet rolled out",
			deploy:         gatewayDeployment(1, 2, 2, 0, 1, 1),
			wantRollingOut: true,
		},
		{
			// Rollout in progress: the new revision is up (available == total) but an
			// old replica is still terminating (surge). Not complete until it is gone.
			name:               "old replica still terminating",
			deploy:             gatewayDeployment(1, 2, 2, 1, 2, 2),
			wantRollingOut:     true,
			wantReasonContains: "terminate",
		},
		{
			// Rollout in progress but stuck: under maxUnavailable:0 the old replica is
			// retained because the updated pod is not available yet (e.g.
			// ImagePullBackOff). available (1) < total (2) distinguishes this from a
			// healthy surge, so the reason must not claim an old replica is winding
			// down. Still rollingOut -- only the timeout (WaitForGatewayReady) can
			// tell a slow pull from a stuck one.
			name:               "updated pod not available retains old replica",
			deploy:             gatewayDeployment(1, 2, 2, 1, 2, 1),
			wantRollingOut:     true,
			wantReasonContains: "not yet available",
		},
		{
			// Failed readiness / steady-state degradation: the updated revision is
			// the only revision (no old replicas), so this is not a roll -- the
			// current release's pod is simply unavailable (e.g. crash-looping).
			name:   "updated revision unavailable is not a roll",
			deploy: gatewayDeployment(1, 2, 2, 1, 1, 0),
		},
		{
			name:   "zero desired replicas",
			deploy: gatewayDeployment(0, 1, 1, 0, 0, 0),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			complete, rollingOut, reason := deploymentRolloutComplete(tc.deploy)
			if complete != tc.wantComplete {
				t.Errorf("complete = %v, want %v (reason %q)", complete, tc.wantComplete, reason)
			}
			if rollingOut != tc.wantRollingOut {
				t.Errorf("rollingOut = %v, want %v (reason %q)", rollingOut, tc.wantRollingOut, reason)
			}
			if !complete && reason == "" {
				t.Error("expected a non-empty reason when not complete")
			}
			if complete && reason != "" {
				t.Errorf("expected empty reason when complete, got %q", reason)
			}
			if tc.wantReasonContains != "" && !strings.Contains(reason, tc.wantReasonContains) {
				t.Errorf("reason = %q, want it to contain %q", reason, tc.wantReasonContains)
			}
		})
	}
}

// withAppliedRelease stamps the applied-release annotation on a Deployment
// fixture, mirroring what deployGateway records at apply time, so the
// appliedRelease return of ObserveGatewayRollout can be exercised.
func withAppliedRelease(deploy *appsv1.Deployment, releaseID string) *appsv1.Deployment {
	if deploy.Annotations == nil {
		deploy.Annotations = map[string]string{}
	}
	deploy.Annotations[AppliedReleaseAnnotation] = releaseID
	return deploy
}

// TestObserveGatewayRollout covers the revision-aware observation used by both
// the provisioning wait loop and the health loop, including the rollingOut signal
// the health loop relies on to defer an in-progress roll to the provisioning path,
// the appliedRelease the health loop advances observed_release_id to, and the
// not-found and API-error paths.
func TestObserveGatewayRollout(t *testing.T) {
	t.Run("complete reports the applied release", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(withAppliedRelease(gatewayDeployment(1, 2, 2, 1, 1, 1), "rel-1"))
		ready, rollingOut, appliedRelease, _, err := ObserveGatewayRollout(context.Background(), cs, "gw-ns", GatewayDeploymentName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ready || rollingOut {
			t.Errorf("ready=%v rollingOut=%v, want true/false", ready, rollingOut)
		}
		if appliedRelease != "rel-1" {
			t.Errorf("appliedRelease = %q, want %q", appliedRelease, "rel-1")
		}
	})

	t.Run("complete direct-image gateway reports no applied release", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(gatewayDeployment(1, 2, 2, 1, 1, 1))
		ready, _, appliedRelease, _, err := ObserveGatewayRollout(context.Background(), cs, "gw-ns", GatewayDeploymentName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ready {
			t.Errorf("ready=%v, want true", ready)
		}
		if appliedRelease != "" {
			t.Errorf("appliedRelease = %q, want empty for a Deployment without the annotation", appliedRelease)
		}
	})

	t.Run("rolling out", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(gatewayDeployment(1, 3, 2, 0, 1, 1))
		ready, rollingOut, _, _, err := ObserveGatewayRollout(context.Background(), cs, "gw-ns", GatewayDeploymentName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ready || !rollingOut {
			t.Errorf("ready=%v rollingOut=%v, want false/true", ready, rollingOut)
		}
	})

	t.Run("degraded current revision is not rolling out", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(gatewayDeployment(1, 2, 2, 1, 1, 0))
		ready, rollingOut, _, reason, err := ObserveGatewayRollout(context.Background(), cs, "gw-ns", GatewayDeploymentName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ready || rollingOut {
			t.Errorf("ready=%v rollingOut=%v, want false/false", ready, rollingOut)
		}
		if reason == "" {
			t.Error("expected a non-empty reason")
		}
	})

	t.Run("deployment not found is not rolling out", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset()
		ready, rollingOut, _, reason, err := ObserveGatewayRollout(context.Background(), cs, "gw-ns", GatewayDeploymentName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ready || rollingOut {
			t.Errorf("ready=%v rollingOut=%v, want false/false", ready, rollingOut)
		}
		if reason != "deployment not found" {
			t.Errorf("reason = %q, want %q", reason, "deployment not found")
		}
	})

	t.Run("api error is surfaced", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset()
		cs.PrependReactor("get", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("boom")
		})
		ready, rollingOut, _, _, err := ObserveGatewayRollout(context.Background(), cs, "gw-ns", GatewayDeploymentName)
		if err == nil {
			t.Fatal("expected an error to be surfaced, got nil")
		}
		if ready || rollingOut {
			t.Errorf("ready=%v rollingOut=%v, want false/false on error", ready, rollingOut)
		}
	})
}

// TestWaitForGatewayReady covers the provisioning wait loop's readiness gate: it
// reports ready once the new revision is fully rolled out, and -- critically -- it
// does NOT report ready when the readiness window expires with the workload still
// unready, so a timeout drives the gateway to Degraded rather than falsely
// reporting the new release. See gateway-release-rollout.spec.md.
func TestWaitForGatewayReady(t *testing.T) {
	// Shorten the poll cadence so the loop observes the fake within a short window.
	orig := gatewayReadyPollInterval
	gatewayReadyPollInterval = time.Millisecond
	defer func() { gatewayReadyPollInterval = orig }()

	t.Run("times out as not ready when the workload never becomes ready", func(t *testing.T) {
		// Updated revision is the only revision but its pod is unavailable (e.g.
		// crash-looping): not a roll, never ready. The window must expire not-ready.
		cs := k8sfake.NewSimpleClientset(gatewayDeployment(1, 2, 2, 1, 1, 0))
		ready, reason := WaitForGatewayReady(context.Background(), cs, "gw-ns", 50*time.Millisecond)
		if ready {
			t.Fatal("ready = true, want false on timeout so the gateway becomes Degraded")
		}
		if reason == "" {
			t.Error("expected a non-empty reason recording why readiness was not reached")
		}
	})

	t.Run("returns ready once the new revision is rolled out", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(gatewayDeployment(1, 2, 2, 1, 1, 1))
		ready, reason := WaitForGatewayReady(context.Background(), cs, "gw-ns", time.Second)
		if !ready {
			t.Fatalf("ready = false (reason %q), want true for a fully rolled-out workload", reason)
		}
	})

	t.Run("cancelled context returns not ready", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cs := k8sfake.NewSimpleClientset(gatewayDeployment(1, 2, 2, 1, 1, 1))
		ready, _ := WaitForGatewayReady(ctx, cs, "gw-ns", time.Second)
		if ready {
			t.Fatal("ready = true, want false when the context is cancelled")
		}
	})
}
