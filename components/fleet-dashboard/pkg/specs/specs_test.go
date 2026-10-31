package specs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// archiveOf builds a GitHub-style tarball: every entry sits under one top
// folder. Names ending in "@" become symlinks.
func archiveOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		header := &tar.Header{Name: "owner-repo-abc1234/" + name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if target, ok := strings.CutSuffix(name, "@"); ok {
			header = &tar.Header{Name: "owner-repo-abc1234/" + target, Linkname: content, Typeflag: tar.TypeSymlink}
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeSource holds fixed branches; countingSource serves them and counts
// downloads.
type fakeSource struct {
	mu        sync.Mutex
	list      RefList
	archives  map[string][]byte
	err       error
	downloads map[string]int
	gate      chan struct{} // when set, downloads wait for it
}

func (f *fakeSource) Branches(context.Context) (RefList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, nil
}

type countingSource struct{ *fakeSource }

func (c countingSource) Archive(_ context.Context, ref string) (io.ReadCloser, error) {
	c.mu.Lock()
	c.downloads[ref]++
	gate, err, data := c.gate, c.err, c.archives[ref]
	c.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeSource) count(ref string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.downloads[ref]
}

func repoFiles() map[string]string {
	return map[string]string{
		"specs/platform/feed.spec.md":    "### Requirement: ABC-DEF-01 -- Feed\n",
		"specs/node_modules/x.md":        "generated\n",
		"specs/notes.txt":                "not markdown\n",
		"pkg/feed_test.go":               "package pkg\n\n// Verifies: ABC-DEF-01.01\n// helper\nfunc TestFeed(t *testing.T) {}\n",
		"README.md":                      "See ABC-DEF-01.\nABC-DEF-010\nABC-DEF-01.01\n// Verifies: ABC-DEF-01\n",
		"go.sum":                         "// Verifies: ABC-DEF-01\n",
		"components/agents/locks/a.json": "// Verifies: ABC-DEF-01\n",
		"web/node_modules/dep/a.js":      "ABC-DEF-01\n",
		"blob.bin":                       "ABC-DEF-01\x00\x01",
		"huge.txt":                       "ABC-DEF-01 " + strings.Repeat("x", maxFileBytes),
		"link.txt@":                      "/etc/passwd",
	}
}

func newFake(t *testing.T) *fakeSource {
	t.Helper()
	return &fakeSource{
		list: RefList{Default: "main", Refs: []string{"main", "feature/x"}},
		archives: map[string][]byte{
			"main":      archiveOf(t, repoFiles()),
			"feature/x": archiveOf(t, map[string]string{"specs/other.md": "other\n"}),
		},
		downloads: map[string]int{},
	}
}

func get(t *testing.T, h http.Handler, target string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return rec.Code, string(body)
}

func decode[T any](t *testing.T, body string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return value
}

func specPaths(t *testing.T, h http.Handler, target string) string {
	t.Helper()
	status, body := get(t, h, target)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", target, status, body)
	}
	var names []string
	for _, file := range decode[[]SpecFile](t, body) {
		names = append(names, file.Path)
	}
	return strings.Join(names, ",")
}

func TestRefsListsSortedBranchesWithTheDefault(t *testing.T) {
	h := New(countingSource{newFake(t)}).Handler()
	status, body := get(t, h, "/refs")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	list := decode[RefList](t, body)
	if list.Default != "main" || strings.Join(list.Refs, ",") != "feature/x,main" {
		t.Errorf("refs = %+v", list)
	}
}

func TestSpecsReadTheRequestedBranch(t *testing.T) {
	h := New(countingSource{newFake(t)}).Handler()
	if got := specPaths(t, h, "/specs.json"); got != "platform/feed.spec.md" {
		t.Errorf("default branch specs = %q, want only platform/feed.spec.md", got)
	}
	if got := specPaths(t, h, "/specs.json?ref=feature/x"); got != "other.md" {
		t.Errorf("feature/x specs = %q", got)
	}
	status, body := get(t, h, "/specs.json?ref=nope")
	if status != http.StatusNotFound || !strings.Contains(body, "Unknown branch: nope") {
		t.Errorf("unknown ref = %d %q, want 404", status, body)
	}
}

func TestReferencesMatchWholeIDsTestsFirst(t *testing.T) {
	h := New(countingSource{newFake(t)}).Handler()
	status, body := get(t, h, "/references?id=ABC-DEF-01")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	refs := decode[[]FileReferences](t, body)
	var paths []string
	for _, ref := range refs {
		paths = append(paths, ref.Path)
	}
	// The test cites only the scenario; binary, huge, symlinked, and
	// node_modules files are never read.
	want := []string{"README.md", "components/agents/locks/a.json", "go.sum", "specs/platform/feed.spec.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("paths = %v, want %v", paths, want)
	}
	if readme := refs[0]; len(readme.Matches) != 2 || readme.Matches[0].Line != 1 || readme.Matches[1].Line != 4 {
		t.Errorf("README matches = %+v, want lines 1 and 4", readme.Matches)
	}

	_, body = get(t, h, "/references?id=ABC-DEF-01.01")
	refs = decode[[]FileReferences](t, body)
	if len(refs) != 2 || refs[0].Path != "pkg/feed_test.go" || !refs[0].IsTest {
		t.Errorf("scenario refs = %+v, want the test first", refs)
	}

	if status, _ := get(t, h, "/references?id=not-an-id"); status != http.StatusBadRequest {
		t.Errorf("bad id status = %d, want 400", status)
	}
}

func TestFileServesOnlyReadTextFiles(t *testing.T) {
	h := New(countingSource{newFake(t)}).Handler()
	status, body := get(t, h, "/file?path=pkg/feed_test.go")
	if status != http.StatusOK || !strings.Contains(decode[SpecFile](t, body).Content, "TestFeed") {
		t.Errorf("file = %d %s", status, body)
	}
	for _, path := range []string{"../secret.txt", "link.txt", "blob.bin", "huge.txt", "web/node_modules/dep/a.js", "/etc/passwd", "missing.go"} {
		if status, _ := get(t, h, "/file?path="+path); status != http.StatusNotFound {
			t.Errorf("GET file %s = %d, want 404", path, status)
		}
	}
	if status, _ := get(t, h, "/file"); status != http.StatusBadRequest {
		t.Errorf("missing path status = %d, want 400", status)
	}
}

func TestMarkersSkipMarkdownAndLockfiles(t *testing.T) {
	h := New(countingSource{newFake(t)}).Handler()
	status, body := get(t, h, "/markers")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	markers := decode[[]MarkerLine](t, body)
	if len(markers) != 1 {
		t.Fatalf("markers = %+v, want one", markers)
	}
	got := markers[0]
	if got.Path != "pkg/feed_test.go" || got.Line != 3 || got.Text != "// Verifies: ABC-DEF-01.01" ||
		strings.Join(got.Following, "|") != "// helper|func TestFeed(t *testing.T) {}" {
		t.Errorf("marker = %+v", got)
	}
}

func TestUnconfiguredServerAnswers404(t *testing.T) {
	h := New(nil).Handler()
	for _, target := range []string{"/refs", "/specs.json", "/markers", "/references?id=ABC-DEF-01", "/file?path=a"} {
		status, body := get(t, h, target)
		if status != http.StatusNotFound || !strings.Contains(body, "FD_UPSTREAM_REPO") {
			t.Errorf("GET %s = %d %q, want 404 naming FD_UPSTREAM_REPO", target, status, body)
		}
	}
}

func TestFailedDownloadIs502AndNotRetriedAtOnce(t *testing.T) {
	fake := newFake(t)
	fake.err = errors.New("GitHub answered 503")
	server := New(countingSource{fake})
	now := time.Unix(0, 0)
	server.now = func() time.Time { return now }
	h := server.Handler()
	for range 2 {
		status, body := get(t, h, "/specs.json")
		if status != http.StatusBadGateway || !strings.Contains(body, "GitHub answered 503") {
			t.Errorf("status = %d %q, want 502 with the cause", status, body)
		}
	}
	if fake.count("main") != 1 {
		t.Errorf("downloads = %d, want 1 within the error delay", fake.count("main"))
	}
	now = now.Add(defaultErrorTTL)
	fake.mu.Lock()
	fake.err = nil
	fake.mu.Unlock()
	if got := specPaths(t, h, "/specs.json"); got != "platform/feed.spec.md" || fake.count("main") != 2 {
		t.Errorf("after the delay = %q (%d downloads), want a retry", got, fake.count("main"))
	}
}

// An expired branch answers at once from the old copy while the new one
// downloads; the next request sees the new copy.
func TestExpiredBranchIsServedStaleWhileRefreshing(t *testing.T) {
	fake := newFake(t)
	server := New(countingSource{fake})
	now := time.Unix(0, 0)
	server.now = func() time.Time { return now }
	h := server.Handler()
	if got := specPaths(t, h, "/specs.json"); got != "platform/feed.spec.md" {
		t.Fatalf("first = %q", got)
	}
	if got := specPaths(t, h, "/specs.json"); got != "platform/feed.spec.md" || fake.count("main") != 1 {
		t.Errorf("within the TTL: %q after %d downloads, want the cache", got, fake.count("main"))
	}

	files := repoFiles()
	files["specs/new.md"] = "new\n"
	gate := make(chan struct{})
	fake.mu.Lock()
	fake.archives["main"] = archiveOf(t, files)
	fake.gate = gate
	fake.mu.Unlock()
	now = now.Add(defaultSnapshotTTL)
	if got := specPaths(t, h, "/specs.json"); got != "platform/feed.spec.md" {
		t.Errorf("while refreshing = %q, want the old copy", got)
	}
	close(gate)
	deadline := time.Now().Add(5 * time.Second)
	for specPaths(t, h, "/specs.json") != "new.md,platform/feed.spec.md" {
		if time.Now().After(deadline) {
			t.Fatal("the refreshed copy never arrived")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if fake.count("main") != 2 {
		t.Errorf("downloads = %d, want 2", fake.count("main"))
	}
}

func TestCacheKeepsTheMostRecentlyUsedBranches(t *testing.T) {
	fake := newFake(t)
	fake.list.Refs = []string{"main", "a", "b", "c"}
	for _, ref := range []string{"a", "b", "c"} {
		fake.archives[ref] = archiveOf(t, map[string]string{"specs/" + ref + ".md": ref})
	}
	server := New(countingSource{fake})
	server.maxRefs = 2
	now := time.Unix(0, 0)
	server.now = func() time.Time { return now }
	h := server.Handler()
	for _, ref := range []string{"a", "b", "a", "c", "a"} {
		now = now.Add(time.Second)
		specPaths(t, h, "/specs.json?ref="+ref)
	}
	// b was dropped for c; a stayed because it was used last.
	if fake.count("a") != 1 || fake.count("b") != 1 || fake.count("c") != 1 {
		t.Errorf("downloads a=%d b=%d c=%d, want 1 each", fake.count("a"), fake.count("b"), fake.count("c"))
	}
	specPaths(t, h, "/specs.json?ref=b")
	if fake.count("b") != 2 {
		t.Errorf("b downloads = %d, want 2 after eviction", fake.count("b"))
	}
}

func TestMentionsID(t *testing.T) {
	cases := map[string]bool{
		"ABC-DEF-01 -- title":      true,
		"(ABC-DEF-01)":             true,
		"see ABC-DEF-01.":          true,
		"ABC-DEF-01-":              true,
		"ABC-DEF-010":              false,
		"XABC-DEF-01":              false,
		"ABC-DEF-01.01":            false,
		"ABC-DEF-01-S1":            false,
		".ABC-DEF-01":              false,
		"x ABC-DEF-010 ABC-DEF-01": true,
	}
	for line, want := range cases {
		if got := MentionsID(line, "ABC-DEF-01"); got != want {
			t.Errorf("MentionsID(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestIsTestFile(t *testing.T) {
	cases := map[string]bool{
		"pkg/a_test.go":        true,
		"web/a.test.tsx":       true,
		"web/a.spec.mjs":       true,
		"scripts/a_test.sh":    true,
		"tests/e2e/helpers.go": true,
		"web/__tests__/a.ts":   true,
		"tests/README.md":      false,
		"pkg/main.go":          false,
	}
	for path, want := range cases {
		if got := IsTestFile(path); got != want {
			t.Errorf("IsTestFile(%q) = %v, want %v", path, got, want)
		}
	}
}
