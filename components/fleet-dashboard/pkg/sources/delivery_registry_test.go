package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestQuayBundleRegistry(t *testing.T) {
	// Verifies: FLT-DLV-06.03, FLT-DLV-12.01
	var reads int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/repository/ns/app/tag/"):
			if got := r.URL.Query().Get("filter_tag_name"); got != "like:release-bundle-%" {
				t.Errorf("filter = %q", got)
			}
			_, _ = w.Write([]byte(`{"tags":[
				{"name":"release-bundle-20261101T100100000000Z-a","manifest_digest":"sha256:a","start_ts":1793527260},
				{"name":"release-bundle-20261101T100200000000Z-b","manifest_digest":"sha256:b","start_ts":1793527320},
				{"name":"other","manifest_digest":"sha256:c"}]}`))
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"token":"anon"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/"):
			if r.Header.Get("Authorization") != "Bearer anon" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="test",scope="repository:ns/app:pull"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			atomic.AddInt32(&reads, 1)
			if strings.Contains(r.URL.Path, "/manifests/") {
				_, _ = w.Write([]byte(`{"layers":[{"mediaType":"application/x","digest":"sha256:layer","annotations":{"org.opencontainers.image.title":"bundle.json"}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"manifests":{"git":{"revision":"abc123"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	q := NewQuayBundleRegistry(srv.URL + "/ns/app")
	if q == nil {
		t.Fatal("registry not built")
	}
	tags, err := q.ListBundles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Digest != "sha256:b" || tags[0].PublishedAt.IsZero() {
		t.Errorf("tags = %+v, want 2 bundle tags newest first with publication times", tags)
	}
	for i := 0; i < 2; i++ {
		rev, err := q.ManifestsRevision(context.Background(), "sha256:b")
		if err != nil || rev != "abc123" {
			t.Fatalf("revision = %q, %v", rev, err)
		}
	}
	if reads != 2 {
		t.Errorf("registry reads = %d, want 2 (manifest + blob, then cached)", reads)
	}
}

func TestNewQuayBundleRegistryRejectsBareHost(t *testing.T) {
	if NewQuayBundleRegistry("quay.io") != nil {
		t.Error("a host without a repository should disable the registry")
	}
}
