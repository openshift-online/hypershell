package gateway

import (
	"context"
	"errors"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"testing"
	"time"
)

func readinessOpts() deploymentReadinessWaitOptions {
	return deploymentReadinessWaitOptions{timeout: 50 * time.Millisecond, pollInterval: time.Millisecond}
}
func readinessDeployment(ns string, ready bool) *appsv1.Deployment {
	n := int32(1)
	r := int32(0)
	if ready {
		r = 1
	}
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: deploymentDatabaseName, Namespace: ns}, Spec: appsv1.DeploymentSpec{Replicas: &n}, Status: appsv1.DeploymentStatus{ReadyReplicas: r}}
}
func sourceSecret(ns string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: `openshell-db-credentials`, Namespace: ns}, Data: map[string][]byte{`dbname`: []byte(`db`), `user`: []byte(`user`), `password`: []byte(`password`)}}
}
func TestDeploymentReadinessWaitAlreadyReady(t *testing.T) {
	c := k8sfake.NewSimpleClientset(readinessDeployment(`db`, true))
	if err := waitForDeploymentReady(context.Background(), c, `db`, deploymentDatabaseName, readinessOpts()); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range c.Actions() {
		if a.GetVerb() == `get` && a.GetResource().Resource == `deployments` {
			n++
		}
	}
	if n != 1 {
		t.Fatalf(`checks=%d`, n)
	}
}
func TestDeploymentReadinessWaitRetriesUnreadyAndTransient(t *testing.T) {
	c := k8sfake.NewSimpleClientset()
	n := 0
	c.PrependReactor(`get`, `deployments`, func(k8stesting.Action) (bool, runtime.Object, error) {
		n++
		if n == 1 {
			return true, readinessDeployment(`db`, false), nil
		}
		if n == 2 {
			return true, nil, k8serrors.NewInternalError(fmt.Errorf(`temporary`))
		}
		return true, readinessDeployment(`db`, true), nil
	})
	if err := waitForDeploymentReady(context.Background(), c, `db`, deploymentDatabaseName, readinessOpts()); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf(`checks=%d`, n)
	}
}
func TestDeploymentReadinessWaitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := k8sfake.NewSimpleClientset()
	c.PrependReactor(`get`, `deployments`, func(k8stesting.Action) (bool, runtime.Object, error) {
		cancel()
		return true, readinessDeployment(`db`, false), nil
	})
	err := waitForDeploymentReady(ctx, c, `db`, deploymentDatabaseName, readinessOpts())
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestDeploymentCredentialsCopyWaitsForReadiness(t *testing.T) {
	c := k8sfake.NewSimpleClientset(sourceSecret(`db`))
	n := 0
	sourceReads := 0
	c.PrependReactor(`get`, `deployments`, func(k8stesting.Action) (bool, runtime.Object, error) {
		n++
		return true, readinessDeployment(`db`, n == 2), nil
	})
	c.PrependReactor(`get`, `secrets`, func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.(k8stesting.GetAction).GetName() == `openshell-db-credentials` {
			sourceReads++
			if n < 2 {
				t.Error(`source secret read before ready`)
			}
		}
		return false, nil, nil
	})
	if err := reconcileDeploymentDatabaseCredentials(context.Background(), c, `db`, `tenant`, readinessOpts()); err != nil {
		t.Fatal(err)
	}
	if sourceReads != 1 {
		t.Fatalf(`source reads=%d`, sourceReads)
	}
	if _, err := c.CoreV1().Secrets(`tenant`).Get(context.Background(), `openshell-gateway-db-credentials`, metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}
func TestDeploymentCredentialsUnreadyTimeoutDoesNotCopy(t *testing.T) {
	c := k8sfake.NewSimpleClientset(readinessDeployment(`db`, false), sourceSecret(`db`))
	o := readinessOpts()
	o.timeout = 5 * time.Millisecond
	err := reconcileDeploymentDatabaseCredentials(context.Background(), c, `db`, `tenant`, o)
	if err == nil {
		t.Fatal(`want timeout`)
	}
	if _, err := c.CoreV1().Secrets(`tenant`).Get(context.Background(), `openshell-gateway-db-credentials`, metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
		t.Fatalf(`secret=%v`, err)
	}
}
