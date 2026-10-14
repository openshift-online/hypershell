package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPurgeKVRecursive(t *testing.T) {
	deleted := map[string]bool{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("list") == "true":
			path := strings.TrimPrefix(r.URL.Path, "/v1/secret/metadata/")
			w.WriteHeader(http.StatusOK)
			switch path {
			case "openshell/ns-1":
				_, _ = w.Write([]byte(`{"data":{"keys":["prov-a","nested/"]}}`))
			case "openshell/ns-1/nested":
				_, _ = w.Write([]byte(`{"data":{"keys":["prov-b"]}}`))
			default:
				_, _ = w.Write([]byte(`{"data":{"keys":[]}}`))
			}
		case r.Method == http.MethodDelete:
			deleted[strings.TrimPrefix(r.URL.Path, "/v1/secret/metadata/")] = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestVaultClient(t, srv)
	if err := purgeKVRecursive(context.Background(), c, "ct", "secret", "openshell/ns-1"); err != nil {
		t.Fatalf("purgeKVRecursive: %v", err)
	}
	for _, want := range []string{"openshell/ns-1/prov-a", "openshell/ns-1/nested/prov-b"} {
		if !deleted[want] {
			t.Errorf("expected delete of %q; deleted=%v", want, deleted)
		}
	}
	// The folder marker itself must not be deleted as a secret.
	if deleted["openshell/ns-1/nested"] {
		t.Error("folder marker should not be deleted as a leaf secret")
	}
}
