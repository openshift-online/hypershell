package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// gatewayAPIClientset returns a clientset whose API server answers the
// gateway.networking.k8s.io/v1 discovery call with the given status and body.
// Every other request fails the test, so the reconcile must stop at the
// Gateway API check.
func gatewayAPIClientset(t *testing.T, status int, body string) *kubernetes.Clientset {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/gateway.networking.k8s.io/v1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("new clientset: %v", err)
	}
	return cs
}

const notFoundStatus = `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`

// Verifies: PLAT-ARCH-09.03
func TestReconcileGatewayFailsWithoutGatewayAPI(t *testing.T) {
	cs := gatewayAPIClientset(t, http.StatusNotFound, notFoundStatus)

	type report struct{ condition, status, message string }
	var reports []report
	opts := ReconcileOpts{ReportProgress: func(condition, status, message string) {
		reports = append(reports, report{condition, status, message})
	}}
	nsConfig := NamespaceConfig{Name: "openshell-abc", Gateway: GatewayConfig{Route: RouteConfig{}}}

	err := ReconcileGateway(context.Background(), nil, cs, nil, nsConfig, opts)
	if !errors.Is(err, errGatewayAPIMissing) {
		t.Fatalf("err = %v, want errGatewayAPIMissing", err)
	}
	if len(reports) == 0 {
		t.Fatal("no condition reported")
	}
	last := reports[len(reports)-1]
	want := report{ConditionEnvironmentReady, StatusFailed, "Gateway API is not installed on this cluster; the gateway cannot be exposed"}
	if last != want {
		t.Fatalf("last report = %+v, want %+v", last, want)
	}
	for _, r := range reports {
		if r.status == StatusComplete {
			t.Fatalf("condition %s reported Complete without the Gateway API", r.condition)
		}
	}
}

func TestRequireGatewayAPI(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantMissing bool
		wantErr     bool
	}{
		{
			name:   "GRPCRoute served",
			status: http.StatusOK,
			body:   `{"kind":"APIResourceList","groupVersion":"gateway.networking.k8s.io/v1","resources":[{"name":"grpcroutes","namespaced":true,"kind":"GRPCRoute","verbs":["get"]}]}`,
		},
		{
			name:        "group served without GRPCRoute",
			status:      http.StatusOK,
			body:        `{"kind":"APIResourceList","groupVersion":"gateway.networking.k8s.io/v1","resources":[{"name":"httproutes","namespaced":true,"kind":"HTTPRoute","verbs":["get"]}]}`,
			wantMissing: true, wantErr: true,
		},
		{
			name:        "group not served",
			status:      http.StatusNotFound,
			body:        notFoundStatus,
			wantMissing: true, wantErr: true,
		},
		{
			name:    "API server error is not reported as missing",
			status:  http.StatusInternalServerError,
			body:    `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"InternalError","code":500}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireGatewayAPI(gatewayAPIClientset(t, tt.status, tt.body).Discovery())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if errors.Is(err, errGatewayAPIMissing) != tt.wantMissing {
				t.Fatalf("err = %v, wantMissing %v", err, tt.wantMissing)
			}
		})
	}
}

func TestDeriveGatewayHostname(t *testing.T) {
	t.Run("explicit host wins", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "custom.example.net"}}}
		got, err := deriveGatewayHostname(ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "custom.example.net" {
			t.Errorf("got %q, want custom.example.net", got)
		}
	})

	t.Run("derived from base domain", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a"}
		got, err := deriveGatewayHostname(ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "gw-tenant-a.apps.example.com" {
			t.Errorf("got %q, want gw-tenant-a.apps.example.com", got)
		}
	})

	t.Run("errors when neither host nor base domain set", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "")
		ns := NamespaceConfig{Name: "tenant-a"}
		if _, err := deriveGatewayHostname(ns); err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("explicit host equal to own slot under base domain is allowed", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "gw-tenant-a.apps.example.com"}}}
		got, err := deriveGatewayHostname(ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "gw-tenant-a.apps.example.com" {
			t.Errorf("got %q, want gw-tenant-a.apps.example.com", got)
		}
	})

	t.Run("rejects another tenant's slot under the shared base domain (hijack)", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "gw-tenant-b.apps.example.com"}}}
		if _, err := deriveGatewayHostname(ns); err == nil {
			t.Error("expected hijack rejection, got nil")
		}
	})

	t.Run("rejects an arbitrary host under the shared base domain", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "victim.apps.example.com"}}}
		if _, err := deriveGatewayHostname(ns); err == nil {
			t.Error("expected rejection of foreign host under base domain, got nil")
		}
	})

	t.Run("external vanity host outside base domain passes through", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "gateway.customer.io"}}}
		got, err := deriveGatewayHostname(ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "gateway.customer.io" {
			t.Errorf("got %q, want gateway.customer.io", got)
		}
	})
}
