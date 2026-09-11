package reconciler

import (
	"context"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

func TestHTTPGatewayVersionObserver(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
		wantError  string
	}{
		{
			name:       "returns a trimmed version",
			statusCode: http.StatusOK,
			body:       `{"status":"healthy","version":" 0.0.109 "}`,
			want:       "0.0.109",
		},
		{
			name:       "keeps a reported version postfix",
			statusCode: http.StatusOK,
			body:       `{"status":"healthy","version":" v0.0.109-rh9a8f8 "}`,
			want:       "v0.0.109-rh9a8f8",
		},
		{
			name:       "returns a version when gateway readiness is unhealthy",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"status":"unhealthy","version":"0.0.109"}`,
			want:       "0.0.109",
		},
		{
			name:       "rejects another HTTP error",
			statusCode: http.StatusInternalServerError,
			body:       `{"version":"0.0.109"}`,
			wantError:  "HTTP 500",
		},
		{
			name:       "rejects invalid JSON",
			statusCode: http.StatusOK,
			body:       `{invalid`,
			wantError:  "decode gateway health response",
		},
		{
			name:       "rejects an empty version",
			statusCode: http.StatusOK,
			body:       `{"status":"healthy"}`,
			wantError:  "has no version",
		},
		{
			name:       "rejects an embedded control character",
			statusCode: http.StatusOK,
			body:       `{"version":"v0.0.109\nforged log entry"}`,
			wantError:  "contains a control character",
		},
		{
			name:       "rejects a long version",
			statusCode: http.StatusOK,
			body:       `{"version":"` + strings.Repeat("v", maxGatewayVersionLength+1) + `"}`,
			wantError:  "version is too long",
		},
		{
			name:       "rejects a large response",
			statusCode: http.StatusOK,
			body:       strings.Repeat(" ", maxGatewayHealthResponseBytes+1),
			wantError:  "response is too large",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			observer := &httpGatewayVersionObserver{
				client:  server.Client(),
				timeout: time.Second,
				endpoint: func(namespace string) string {
					if namespace != "openshell-test" {
						t.Fatalf("namespace = %q, want openshell-test", namespace)
					}
					return server.URL
				},
			}

			got, err := observer.Observe(context.Background(), "openshell-test")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("Observe() error = %v, want text %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Observe() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("Observe() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHTTPGatewayVersionObserverHasBoundedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	observer := &httpGatewayVersionObserver{
		client:   server.Client(),
		timeout:  25 * time.Millisecond,
		endpoint: func(string) string { return server.URL },
	}

	started := time.Now()
	_, err := observer.Observe(context.Background(), "openshell-test")
	if err == nil {
		t.Fatal("Observe() error = nil, want timeout error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Observe() took %s, want no more than one second", elapsed)
	}
}

func TestHTTPGatewayVersionObserverRejectsRedirects(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalled = true
		_, _ = w.Write([]byte(`{"version":"0.0.109"}`))
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	observer := newHTTPGatewayVersionObserver()
	observer.endpoint = func(string) string { return redirect.URL }

	_, err := observer.Observe(context.Background(), "openshell-test")
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("Observe() error = %v, want HTTP 302", err)
	}
	if targetCalled {
		t.Fatal("Observe() followed a redirect")
	}
}

func TestGatewayVersionUpdateRequest(t *testing.T) {
	storedVersion := "v0.0.108"
	gatewayRecord := &pb.Gateway{
		Metadata:       &pb.ObjectReference{Id: "gateway-1"},
		GatewayVersion: &storedVersion,
	}

	if request := gatewayVersionUpdateRequest(gatewayRecord, storedVersion); request != nil {
		t.Fatalf("unchanged request = %#v, want nil", request)
	}
	request := gatewayVersionUpdateRequest(gatewayRecord, "v0.0.109-rh9a8f8")
	if request == nil {
		t.Fatal("changed request = nil")
	}
	if request.GetId() != "gateway-1" || request.GetGatewayVersion() != "v0.0.109-rh9a8f8" {
		t.Fatalf("changed request = %#v", request)
	}
}

func TestObservedGatewayHealthUpdateWithoutVersion(t *testing.T) {
	currentPhase := "Running"
	currentStatus := "Healthy"

	t.Run("does not update unchanged state", func(t *testing.T) {
		if request := observedGatewayHealthUpdate("gateway-1", currentPhase, currentStatus, currentPhase, currentStatus, false); request != nil {
			t.Fatalf("request = %#v, want nil", request)
		}
	})

	t.Run("combines changed phase and status", func(t *testing.T) {
		request := observedGatewayHealthUpdate("gateway-1", currentPhase, currentStatus, "Degraded", "route not ready", false)
		if request == nil || request.Phase == nil || request.Status == nil {
			t.Fatalf("request = %#v, want phase and status", request)
		}
	})

	t.Run("updates health without a version observation", func(t *testing.T) {
		request := observedGatewayHealthUpdate("gateway-1", currentPhase, currentStatus, "Degraded", "deployment not ready", false)
		if request == nil || request.Phase == nil || request.Status == nil {
			t.Fatalf("request = %#v, want phase and status", request)
		}
	})

	t.Run("does not update health after an exposure observation failure", func(t *testing.T) {
		if request := observedGatewayHealthUpdate("gateway-1", currentPhase, currentStatus, "", "", false); request != nil {
			t.Fatalf("request = %#v, want nil", request)
		}
	})
}

type versionObserverFunc func(context.Context, string) (string, error)

func (f versionObserverFunc) Observe(ctx context.Context, namespace string) (string, error) {
	return f(ctx, namespace)
}

func TestGatewayVersionAccessChecks(t *testing.T) {
	accessChecks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			accessChecks++
		}
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"openshell-gateway-health"}}`))
	}))
	defer server.Close()
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	observationFails := false
	observations := 0
	h := &GatewayHealthReconciler{
		clientset:           clientset,
		skipNetworkPolicies: true,
		now:                 func() time.Time { return now },
		versionObserver: versionObserverFunc(func(context.Context, string) (string, error) {
			observations++
			if observationFails {
				return "", fmt.Errorf("health endpoint is unavailable")
			}
			return "0.0.109", nil
		}),
	}
	version := "0.0.109"
	record := &pb.Gateway{Metadata: &pb.ObjectReference{Id: "gateway-1"}, GatewayVersion: &version}
	check := func(wantAccess, wantObservations int) {
		t.Helper()
		h.reconcileGatewayVersion(context.Background(), nil, record, "gateway-1")
		if accessChecks != wantAccess || observations != wantObservations {
			t.Fatalf("access checks = %d, observations = %d; want %d, %d", accessChecks, observations, wantAccess, wantObservations)
		}
	}
	check(1, 1)
	now = now.Add(defaultHealthInterval)
	check(1, 2)
	now = now.Add(gatewayHealthAccessInterval)
	check(2, 3)
	observationFails = true
	check(2, 4)
	observationFails = false
	check(3, 5)
	// Entries for deleted gateways must not remain in memory without a limit.
	now = now.Add(gatewayHealthAccessInterval)
	h.pruneHealthAccessChecks()
	if len(h.healthAccessCheckedAt) != 0 {
		t.Fatal("expired access checks remain in memory")
	}
}
