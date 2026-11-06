package gateway

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const serverTLSSecretName = "openshell-server-tls"

func serverTLSSecret(namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: serverTLSSecretName, Namespace: namespace},
		Data:       data,
	}
}

// TestReadServerTLSCADistinguishesAbsentFromFailed verifies OSG-ROUT-14: a
// genuinely absent CA (NotFound Secret, or a Secret without a ca.crt key) returns
// ("", nil), while any other read error is surfaced so the caller retries instead
// of reading an unconfirmed read as "no CA".
// Verifies: OSG-ROUT-14.01, OSG-ROUT-14.02
func TestReadServerTLSCADistinguishesAbsentFromFailed(t *testing.T) {
	const namespace = "openshell-tenant"

	t.Run("missing secret is a genuine absent CA", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset()
		ca, err := readServerTLSCA(context.Background(), cs, namespace)
		if err != nil {
			t.Fatalf("readServerTLSCA error = %v, want nil", err)
		}
		if ca != "" {
			t.Fatalf("readServerTLSCA ca = %q, want empty", ca)
		}
	})

	t.Run("secret without ca.crt is a genuine absent CA", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(serverTLSSecret(namespace, map[string][]byte{"tls.crt": []byte("x")}))
		ca, err := readServerTLSCA(context.Background(), cs, namespace)
		if err != nil {
			t.Fatalf("readServerTLSCA error = %v, want nil", err)
		}
		if ca != "" {
			t.Fatalf("readServerTLSCA ca = %q, want empty", ca)
		}
	})

	t.Run("present ca.crt is returned", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(serverTLSSecret(namespace, map[string][]byte{"ca.crt": []byte("CA-PEM")}))
		ca, err := readServerTLSCA(context.Background(), cs, namespace)
		if err != nil {
			t.Fatalf("readServerTLSCA error = %v, want nil", err)
		}
		if ca != "CA-PEM" {
			t.Fatalf("readServerTLSCA ca = %q, want CA-PEM", ca)
		}
	})

	t.Run("transient read failure is surfaced", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset()
		cs.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("boom")
		})
		ca, err := readServerTLSCA(context.Background(), cs, namespace)
		if err == nil {
			t.Fatal("readServerTLSCA error = nil, want a surfaced read failure")
		}
		if ca != "" {
			t.Fatalf("readServerTLSCA ca = %q, want empty on error", ca)
		}
	})
}

// backendTLSPolicyGVR locates the BackendTLSPolicy resource the fake dynamic
// client tracks.
var backendTLSPolicyGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "backendtlspolicies",
}

func newBackendTLSDynamicClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			backendTLSPolicyGVR:                     "BackendTLSPolicyList",
			{Version: "v1", Resource: "configmaps"}: "ConfigMapList",
		})
}

// TestReconcileBackendTLSStopsBeforePolicyOnConfigMapWriteFailure verifies
// OSG-ROUT-15: a backend-CA ConfigMap write failure is surfaced and the
// dependent BackendTLSPolicy is never created, so the policy cannot reference a
// missing or stale CA.
// Verifies: OSG-ROUT-15.01
func TestReconcileBackendTLSStopsBeforePolicyOnConfigMapWriteFailure(t *testing.T) {
	const namespace = "openshell-tenant"
	cs := k8sfake.NewSimpleClientset(serverTLSSecret(namespace, map[string][]byte{"ca.crt": []byte("CA-PEM")}))
	cs.PrependReactor("create", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("configmap write failed")
	})
	dc := newBackendTLSDynamicClient()

	err := reconcileBackendTLS(context.Background(), dc, cs, namespace)
	if err == nil {
		t.Fatal("reconcileBackendTLS error = nil, want the ConfigMap write error surfaced")
	}

	for _, action := range dc.Actions() {
		if action.GetResource() == backendTLSPolicyGVR && (action.GetVerb() == "create" || action.GetVerb() == "update") {
			t.Fatalf("BackendTLSPolicy was %sd after a failed ConfigMap write", action.GetVerb())
		}
	}
	if _, getErr := dc.Resource(backendTLSPolicyGVR).Namespace(namespace).Get(context.Background(), "openshell-gateway", metav1.GetOptions{}); !k8serrors.IsNotFound(getErr) {
		t.Fatalf("BackendTLSPolicy present after a failed ConfigMap write, err = %v", getErr)
	}
}

// TestReconcileBackendTLSToleratesMissingCRD verifies OSG-ROUT-16.01: when the
// BackendTLSPolicy CRD is not installed (the API server returns NotFound for the
// resource type) the step is tolerated and skipped without failing the pass, yet
// the backend-CA ConfigMap is still written.
// Verifies: OSG-ROUT-16.01
func TestReconcileBackendTLSToleratesMissingCRD(t *testing.T) {
	const namespace = "openshell-tenant"
	cs := k8sfake.NewSimpleClientset(serverTLSSecret(namespace, map[string][]byte{"ca.crt": []byte("CA-PEM")}))
	dc := newBackendTLSDynamicClient()
	notFound := func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewNotFound(backendTLSPolicyGVR.GroupResource(), "openshell-gateway")
	}
	dc.PrependReactor("get", "backendtlspolicies", notFound)
	dc.PrependReactor("create", "backendtlspolicies", notFound)

	if err := reconcileBackendTLS(context.Background(), dc, cs, namespace); err != nil {
		t.Fatalf("reconcileBackendTLS error = %v, want nil when the CRD is absent", err)
	}
	if _, err := cs.CoreV1().ConfigMaps(namespace).Get(context.Background(), backendCAConfigMapName, metav1.GetOptions{}); err != nil {
		t.Fatalf("backend CA ConfigMap not written: %v", err)
	}
}

// TestReconcileBackendTLSSurfacesNonCRDPolicyError verifies OSG-ROUT-16.02: a
// BackendTLSPolicy error that is not a missing CRD (for example an RBAC denial)
// is surfaced so the pass retries, rather than being discarded as if the CRD were
// absent.
// Verifies: OSG-ROUT-16.02
func TestReconcileBackendTLSSurfacesNonCRDPolicyError(t *testing.T) {
	const namespace = "openshell-tenant"
	cs := k8sfake.NewSimpleClientset(serverTLSSecret(namespace, map[string][]byte{"ca.crt": []byte("CA-PEM")}))
	dc := newBackendTLSDynamicClient()
	dc.PrependReactor("get", "backendtlspolicies", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(backendTLSPolicyGVR.GroupResource(), "openshell-gateway", errors.New("rbac"))
	})

	err := reconcileBackendTLS(context.Background(), dc, cs, namespace)
	if err == nil {
		t.Fatal("reconcileBackendTLS error = nil, want the non-CRD BackendTLSPolicy error surfaced")
	}
	if k8serrors.IsNotFound(err) {
		t.Fatalf("reconcileBackendTLS returned a NotFound error, want the RBAC error surfaced: %v", err)
	}
}

// TestReconcileBackendTLSNoCANoWrite verifies OSG-ROUT-14.02: a genuinely absent
// CA proceeds without creating the backend-CA ConfigMap or BackendTLSPolicy.
// Verifies: OSG-ROUT-14.02
func TestReconcileBackendTLSNoCANoWrite(t *testing.T) {
	const namespace = "openshell-tenant"
	cs := k8sfake.NewSimpleClientset() // no openshell-server-tls secret
	dc := newBackendTLSDynamicClient()

	if err := reconcileBackendTLS(context.Background(), dc, cs, namespace); err != nil {
		t.Fatalf("reconcileBackendTLS error = %v, want nil when the CA is absent", err)
	}
	if _, err := cs.CoreV1().ConfigMaps(namespace).Get(context.Background(), backendCAConfigMapName, metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
		t.Fatalf("backend CA ConfigMap written despite absent CA, err = %v", err)
	}
}
