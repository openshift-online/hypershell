package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
)

func restSource(t *testing.T, handler http.HandlerFunc) *RESTSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	conn, err := connection.NewConnection().Config(&config.Config{URL: srv.URL, AccessToken: "token"}).RefreshPerRequest(true).Build()
	if err != nil {
		t.Fatal(err)
	}
	return NewRESTSource(conn)
}

func page(n, total int) []byte {
	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{"id": fmt.Sprintf("id-%d", i), "name": fmt.Sprintf("gw-%d", i)}
	}
	body, _ := json.Marshal(map[string]any{"kind": "GatewayList", "size": n, "total": total, "items": items})
	return body
}

func TestListWalksPages(t *testing.T) {
	var requests []string
	src := restSource(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("missing bearer token")
		}
		if r.URL.Query().Get("page") == "1" {
			w.Write(page(PageSize, 130))
			return
		}
		w.Write(page(30, 130))
	})
	res, err := src.List(context.Background(), KindGateways)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 130 || res.Truncated {
		t.Errorf("items = %d truncated = %v, want 130 and false", len(res.Items), res.Truncated)
	}
	want := []string{"/api/hypershell/v1/gateways?page=1&size=100", "/api/hypershell/v1/gateways?page=2&size=100"}
	if strings.Join(requests, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %v, want %v", requests, want)
	}
}

func TestListStopsAtPageBound(t *testing.T) {
	calls := 0
	src := restSource(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write(page(PageSize, 9999))
	})
	res, err := src.List(context.Background(), KindClusters)
	if err != nil {
		t.Fatal(err)
	}
	if calls != MaxPages || !res.Truncated {
		t.Errorf("calls = %d truncated = %v, want %d and true", calls, res.Truncated, MaxPages)
	}
}

func TestListExactlyAtBoundIsNotTruncated(t *testing.T) {
	src := restSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(page(PageSize, PageSize*MaxPages))
	})
	res, err := src.List(context.Background(), KindClusters)
	if err != nil {
		t.Fatal(err)
	}
	if res.Truncated {
		t.Error("a list that ends exactly at the bound was marked truncated")
	}
}

func TestCreateGatewaySendsOnlyFormFields(t *testing.T) {
	src := restSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/hypershell/v1/gateways" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"name":"demo","cluster_id":""}` {
			t.Errorf("body = %s", body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"g9","name":"demo","namespace":"gw-g9"}`))
	})
	res, err := src.CreateGateway(context.Background(), GatewayCreate{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "g9" || res.Field("namespace") != "gw-g9" {
		t.Errorf("created = %+v", res)
	}
}

func TestAPIErrorsCarryReason(t *testing.T) {
	src := restSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"kind":"Error","code":"HYPERSHELL-403","reason":"missing gateway:creator"}`))
	})
	_, err := src.CreateGateway(context.Background(), GatewayCreate{Name: "demo"})
	if StatusOf(err) != http.StatusForbidden || ReasonOf(err) != "missing gateway:creator" {
		t.Errorf("err = %v, want 403 with reason", err)
	}
}

func TestGetNotFoundAndDelete(t *testing.T) {
	src := restSource(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("no such gateway"))
		case http.MethodDelete:
			if r.URL.Path != "/api/hypershell/v1/gateways/g1" {
				t.Errorf("delete path = %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	})
	_, err := src.Get(context.Background(), KindGateways, "g1")
	if StatusOf(err) != http.StatusNotFound || ReasonOf(err) != "no such gateway" {
		t.Errorf("get err = %v, want 404", err)
	}
	if err := src.DeleteGateway(context.Background(), "g1"); err != nil {
		t.Errorf("delete: %v", err)
	}
}

func TestResourceFieldsKeepNumbersExact(t *testing.T) {
	r, err := DecodeResource([]byte(`{"id":"g1","active_sandbox_count":12345678901,"ready":true,"oidc":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Field("active_sandbox_count"); got != strconv.Itoa(12345678901) {
		t.Errorf("count = %q", got)
	}
	if r.Field("ready") != "true" || r.Field("oidc") != "" || r.Field("missing") != "" {
		t.Errorf("fields = %v", r.Fields)
	}
}

func TestToYAMLKeepsAPIOrder(t *testing.T) {
	got, err := toYAML([]byte(`{"name":"gw-a","id":"g1","server_dns_names":["a.example.com"],"labels":{"z":"1","a":"2"},"oidc":"{\"issuer\":\"x\"}"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"name: gw-a",
		"id: g1",
		"server_dns_names:",
		"  - a.example.com",
		"labels:",
		`  z: "1"`,
		`  a: "2"`,
		`oidc: '{"issuer":"x"}'`,
	}, "\n")
	if got != want {
		t.Errorf("yaml =\n%s\nwant\n%s", got, want)
	}
}

func TestReasonOfTransportErrorDropsURL(t *testing.T) {
	conn, err := connection.NewConnection().Config(&config.Config{URL: "http://127.0.0.1:1", AccessToken: "token"}).Build()
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewRESTSource(conn).List(context.Background(), KindGateways)
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if reason := ReasonOf(err); strings.Contains(reason, "/api/hypershell") || !strings.Contains(reason, "connect") {
		t.Errorf("reason = %q, want the cause without the URL", reason)
	}
}

func TestSentence(t *testing.T) {
	if got := sentence("user is not permitted", "Next."); got != "user is not permitted. Next." {
		t.Errorf("got %q", got)
	}
	if got := sentence("Denied.", "Next."); got != "Denied. Next." {
		t.Errorf("got %q", got)
	}
}
