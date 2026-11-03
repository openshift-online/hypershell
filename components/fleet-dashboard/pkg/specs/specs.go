// Package specs serves the data API of the spec viewer
// (packages/spec-viewer-ui), which the fleet dashboard hosts at /specs/.
//
// The viewer is a separate UI that only shares this binary. It reads the specs
// and test files of any branch of the product repository (FD_UPSTREAM_REPO),
// downloaded from GitHub at runtime. Nothing is baked into the image: the
// specs mention fleet names, and the image is public (data-architecture spec
// §3.5, the fleet-identity firewall).
//
// The endpoints mirror the viewer's dev server
// (packages/spec-viewer-ui/server); keep the two in step. Every endpoint
// except refs takes `ref=BRANCH`; without it, the default branch is used.
//
//   - GET refs              the branches, and the default one
//   - GET specs.json        every markdown file under specs/, with its text
//   - GET references?id=ID  repository lines that mention ID as a whole word
//   - GET file?path=PATH    the text of one repository file
//   - GET markers           every `Verifies:` test marker line
//
// The viewer applies the marker grammar itself
// (packages/spec-viewer-ui/src/domain/traceability.ts).
package specs

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// Larger files are generated or vendored; skip them.
	maxFileBytes = 1_000_000
	// A huge repository must not exhaust the pod's memory.
	maxTotalBytes     = 256 * 1024 * 1024
	maxFiles          = 50_000
	maxMatchesPerFile = 50
	maxLineLength     = 300
	maxFollowingLines = 40
	binarySniffBytes  = 8000

	// Each cached branch holds its text files (about 20 MB for this
	// repository); keep a few, dropping the least recently used.
	defaultMaxRefs = 3
	// How long a branch download is served before it is fetched again. A
	// stale copy keeps answering while the new one downloads.
	defaultSnapshotTTL = 5 * time.Minute
	defaultRefsTTL     = 5 * time.Minute
	// A failed download is not retried for this long.
	defaultErrorTTL = 30 * time.Second
	downloadTimeout = 3 * time.Minute
)

// Directories that hold no hand-written repository content.
var skippedDirs = map[string]bool{".git": true, "node_modules": true}

// Spec directories the viewer leaves out (generated output).
var skippedSpecDirs = map[string]bool{"node_modules": true, "storybook-static": true}

// Same scope as scripts/check_spec_ids.py: lockfiles hold digests that can
// look like IDs, and markdown holds no markers.
var markerSkippedNames = map[string]bool{
	"pnpm-lock.yaml":    true,
	"package-lock.json": true,
	"yarn.lock":         true,
	"go.sum":            true,
}

var markerSkippedPrefixes = []string{"components/agents/locks/"}

var (
	idPattern     = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:[-.][A-Z0-9]+)+$`)
	markerStart   = regexp.MustCompile(`^\s*(?://|#)\s*Verifies:`)
	testFile      = regexp.MustCompile(`(?:_test\.go|\.(?:test|spec)\.[cm]?[jt]sx?|_test\.sh)$`)
	testFolder    = regexp.MustCompile(`(?:^|/)(?:tests?|e2e|e2e-live|__tests__)/`)
	commentStarts = []string{"//", "#", "/*", "*", "*/"}
)

// SpecFile is one markdown file under specs/.
type SpecFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ReferenceMatch is one line that mentions an ID.
type ReferenceMatch struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// FileReferences lists the lines of one file that mention an ID.
type FileReferences struct {
	Path    string           `json:"path"`
	IsTest  bool             `json:"isTest"`
	Matches []ReferenceMatch `json:"matches"`
}

// MarkerLine is a line that starts a `Verifies:` comment, with the lines after
// it up to and including the first line that is neither blank nor a comment.
type MarkerLine struct {
	Path      string   `json:"path"`
	Line      int      `json:"line"`
	Text      string   `json:"text"`
	Following []string `json:"following"`
}

// snapshot is the text of every readable file of one branch.
type snapshot struct {
	paths []string
	text  map[string]string
}

// entry is one cached branch. loading is non-nil while a download runs.
type entry struct {
	snap     *snapshot
	fetched  time.Time
	err      error
	failedAt time.Time
	used     time.Time
	loading  chan struct{}
}

// Server serves the spec viewer API from a Source.
type Server struct {
	source   Source
	now      func() time.Time
	maxRefs  int
	ttl      time.Duration
	refsTTL  time.Duration
	errorTTL time.Duration

	mu    sync.Mutex
	cache map[string]*entry

	refsMu sync.Mutex
	refs   *RefList
	refsAt time.Time
}

// New returns a server over source. A nil source disables the API: every
// endpoint answers 404 so the viewer can say what is missing.
func New(source Source) *Server {
	return &Server{
		source:   source,
		now:      time.Now,
		maxRefs:  defaultMaxRefs,
		ttl:      defaultSnapshotTTL,
		refsTTL:  defaultRefsTTL,
		errorTTL: defaultErrorTTL,
		cache:    map[string]*entry{},
	}
}

// Handler serves the API; mount it with the /specs/api prefix stripped.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /refs", s.handleRefs)
	mux.HandleFunc("GET /specs.json", s.handleSpecs)
	mux.HandleFunc("GET /references", s.handleReferences)
	mux.HandleFunc("GET /file", s.handleFile)
	mux.HandleFunc("GET /markers", s.handleMarkers)
	return mux
}

var errNotConfigured = errors.New("the spec viewer has no repository (set FD_UPSTREAM_REPO)")

// errUnknownRef is a client error: the branch is not in the listing.
type errUnknownRef string

func (e errUnknownRef) Error() string { return "Unknown branch: " + string(e) }

// refList returns the cached branch listing, fetching it when it expired.
func (s *Server) refList(ctx context.Context) (RefList, error) {
	if s.source == nil {
		return RefList{}, errNotConfigured
	}
	s.refsMu.Lock()
	defer s.refsMu.Unlock()
	if s.refs != nil && s.now().Sub(s.refsAt) < s.refsTTL {
		return *s.refs, nil
	}
	list, err := s.source.Branches(ctx)
	if err != nil {
		if s.refs != nil {
			// A stale listing beats none; try again after the error delay.
			s.refsAt = s.now().Add(s.errorTTL - s.refsTTL)
			slog.Default().Warn("spec viewer: listing branches failed; serving the old list", "err", err)
			return *s.refs, nil
		}
		return RefList{}, fmt.Errorf("listing branches: %w", err)
	}
	sort.Strings(list.Refs)
	s.refs, s.refsAt = &list, s.now()
	return list, nil
}

// resolve maps the ref parameter to a listed branch; empty means the default.
func (s *Server) resolve(ctx context.Context, ref string) (string, error) {
	list, err := s.refList(ctx)
	if err != nil {
		return "", err
	}
	if ref == "" {
		return list.Default, nil
	}
	// Only listed branches can be downloaded, so a request cannot make the
	// server fetch arbitrary commits.
	for _, name := range list.Refs {
		if name == ref {
			return ref, nil
		}
	}
	return "", errUnknownRef(ref)
}

// load returns the snapshot of the ref parameter's branch.
func (s *Server) load(r *http.Request) (*snapshot, error) {
	ref, err := s.resolve(r.Context(), r.URL.Query().Get("ref"))
	if err != nil {
		return nil, err
	}
	return s.snapshot(r.Context(), ref)
}

// snapshot returns the cached copy of ref. An expired copy is served while a
// new one downloads; with no copy, the caller waits for the download.
func (s *Server) snapshot(ctx context.Context, ref string) (*snapshot, error) {
	s.mu.Lock()
	now := s.now()
	e := s.cache[ref]
	if e == nil {
		e = &entry{}
		s.cache[ref] = e
	}
	e.used = now
	if e.snap != nil && now.Sub(e.fetched) < s.ttl {
		s.mu.Unlock()
		return e.snap, nil
	}
	if e.snap == nil && e.err != nil && now.Sub(e.failedAt) < s.errorTTL {
		err := e.err
		s.mu.Unlock()
		return nil, err
	}
	if e.loading == nil {
		e.loading = make(chan struct{})
		go s.download(ref, e)
	}
	loading, stale := e.loading, e.snap
	s.evict()
	s.mu.Unlock()
	if stale != nil {
		return stale, nil
	}
	select {
	case <-loading:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.snap == nil {
		return nil, e.err
	}
	return e.snap, nil
}

// download fetches ref into e. It runs detached from the request, so a
// closed browser tab does not waste a nearly finished download.
func (s *Server) download(ref string, e *entry) {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	snap, err := s.fetch(ctx, ref)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		slog.Default().Error("spec viewer: downloading a branch failed", "ref", ref, "err", err)
		e.err, e.failedAt = fmt.Errorf("downloading %s: %w", ref, err), s.now()
		if e.snap != nil {
			// Keep serving the old copy; retry after the error delay.
			e.fetched = s.now().Add(s.errorTTL - s.ttl)
		}
	} else {
		e.snap, e.fetched, e.err = snap, s.now(), nil
	}
	close(e.loading)
	e.loading = nil
}

func (s *Server) fetch(ctx context.Context, ref string) (*snapshot, error) {
	archive, err := s.source.Archive(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer func() { _ = archive.Close() }()
	return readArchive(archive)
}

// evict drops the least recently used branches beyond maxRefs. Callers hold
// s.mu. A branch that is downloading stays.
func (s *Server) evict() {
	for len(s.cache) > s.maxRefs {
		var oldest string
		for ref, e := range s.cache {
			if e.loading != nil {
				continue
			}
			if oldest == "" || e.used.Before(s.cache[oldest].used) {
				oldest = ref
			}
		}
		if oldest == "" {
			return
		}
		delete(s.cache, oldest)
	}
}

// readArchive reads the text files of a GitHub tarball. Its entries sit under
// one top folder ("owner-repo-sha/"), which is dropped.
func readArchive(r io.Reader) (*snapshot, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("reading the archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	archive := tar.NewReader(gz)
	snap := &snapshot{text: map[string]string{}}
	total := 0
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading the archive: %w", err)
		}
		// Regular files only: symlinks could point anywhere.
		if header.Typeflag != tar.TypeReg || header.Size > maxFileBytes {
			continue
		}
		_, path, ok := strings.Cut(header.Name, "/")
		if !ok || path == "" || inSkippedDir(path) {
			continue
		}
		if len(snap.paths) >= maxFiles || total+int(header.Size) > maxTotalBytes {
			return nil, fmt.Errorf("the branch is larger than the spec viewer reads (%d files, %d bytes)", maxFiles, maxTotalBytes)
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			return nil, fmt.Errorf("reading %s from the archive: %w", path, err)
		}
		sniff := data
		if len(sniff) > binarySniffBytes {
			sniff = sniff[:binarySniffBytes]
		}
		if strings.IndexByte(string(sniff), 0) >= 0 {
			continue
		}
		total += len(data)
		snap.paths = append(snap.paths, path)
		snap.text[path] = string(data)
	}
	sort.Strings(snap.paths)
	return snap, nil
}

func inSkippedDir(path string) bool {
	segments := strings.Split(path, "/")
	for _, segment := range segments[:len(segments)-1] {
		if skippedDirs[segment] {
			return true
		}
	}
	return false
}

func (s *Server) handleRefs(w http.ResponseWriter, r *http.Request) {
	list, err := s.refList(r.Context())
	if err != nil {
		sendLoadError(w, err)
		return
	}
	sendJSON(w, list)
}

func (s *Server) handleSpecs(w http.ResponseWriter, r *http.Request) {
	snap, err := s.load(r)
	if err != nil {
		sendLoadError(w, err)
		return
	}
	files := []SpecFile{}
	for _, path := range snap.paths {
		relative, ok := strings.CutPrefix(path, "specs/")
		if !ok || !strings.HasSuffix(path, ".md") || inSkippedSpecDir(relative) {
			continue
		}
		files = append(files, SpecFile{Path: relative, Content: snap.text[path]})
	}
	sendJSON(w, files)
}

func inSkippedSpecDir(relative string) bool {
	segments := strings.Split(relative, "/")
	for _, segment := range segments[:len(segments)-1] {
		if skippedSpecDirs[segment] {
			return true
		}
	}
	return false
}

func (s *Server) handleReferences(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !idPattern.MatchString(id) {
		http.Error(w, "Not a requirement or scenario ID: "+id, http.StatusBadRequest)
		return
	}
	snap, err := s.load(r)
	if err != nil {
		sendLoadError(w, err)
		return
	}
	sendJSON(w, FindReferences(snap.paths, snap.text, id))
}

// FindReferences lists the files whose text mentions id as a whole
// identifier, test files first.
func FindReferences(paths []string, text map[string]string, id string) []FileReferences {
	results := []FileReferences{}
	for _, path := range paths {
		content := text[path]
		if !strings.Contains(content, id) {
			continue
		}
		var matches []ReferenceMatch
		for index, line := range strings.Split(content, "\n") {
			if !MentionsID(line, id) {
				continue
			}
			matches = append(matches, ReferenceMatch{Line: index + 1, Text: truncate(strings.TrimSpace(line))})
			if len(matches) >= maxMatchesPerFile {
				break
			}
		}
		if len(matches) > 0 {
			results = append(results, FileReferences{Path: path, IsTest: IsTestFile(path), Matches: matches})
		}
	}
	sort.SliceStable(results, func(a, b int) bool {
		if results[a].IsTest != results[b].IsTest {
			return results[a].IsTest
		}
		return results[a].Path < results[b].Path
	})
	return results
}

// MentionsID reports whether line holds id as a whole identifier: ABC-DEF-01
// matches "ABC-DEF-01 --" and "(ABC-DEF-01)", but not "ABC-DEF-010", "XABC-DEF-01", or the
// scenario ID "ABC-DEF-01.01".
func MentionsID(line, id string) bool {
	for start := 0; ; {
		at := strings.Index(line[start:], id)
		if at < 0 {
			return false
		}
		at += start
		end := at + len(id)
		if !isBeforeIDChar(line, at) && !continuesID(line, end) {
			return true
		}
		start = at + 1
	}
}

func isWordChar(c byte) bool {
	return c == '_' || ('0' <= c && c <= '9') || ('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z')
}

func isBeforeIDChar(line string, at int) bool {
	if at == 0 {
		return false
	}
	c := line[at-1]
	return isWordChar(c) || c == '.' || c == '-'
}

func continuesID(line string, end int) bool {
	if end >= len(line) {
		return false
	}
	c := line[end]
	if isWordChar(c) {
		return true
	}
	return (c == '-' || c == '.') && end+1 < len(line) && isWordChar(line[end+1]) && line[end+1] != '_'
}

// IsTestFile reports whether a repository path is a test.
func IsTestFile(path string) bool {
	return !strings.HasSuffix(path, ".md") && (testFile.MatchString(path) || testFolder.MatchString(path))
}

func truncate(line string) string {
	if utf8.RuneCountInString(line) <= maxLineLength {
		return line
	}
	return string([]rune(line)[:maxLineLength])
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Missing path", http.StatusBadRequest)
		return
	}
	snap, err := s.load(r)
	if err != nil {
		sendLoadError(w, err)
		return
	}
	// Only files the snapshot read can be served, so no path can leave it.
	content, ok := snap.text[path]
	if !ok {
		http.Error(w, "Not a text file in the repository: "+path, http.StatusNotFound)
		return
	}
	sendJSON(w, SpecFile{Path: path, Content: content})
}

func (s *Server) handleMarkers(w http.ResponseWriter, r *http.Request) {
	snap, err := s.load(r)
	if err != nil {
		sendLoadError(w, err)
		return
	}
	markers := []MarkerLine{}
	for _, path := range snap.paths {
		if IsMarkerCandidate(path) {
			markers = append(markers, MarkerLines(path, snap.text[path])...)
		}
	}
	sendJSON(w, markers)
}

// IsMarkerCandidate reports whether a file may hold `Verifies:` markers.
func IsMarkerCandidate(path string) bool {
	if strings.HasSuffix(path, ".md") || markerSkippedNames[path[strings.LastIndex(path, "/")+1:]] {
		return false
	}
	for _, prefix := range markerSkippedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

// MarkerLines returns the marker lines of one file, each with the lines up
// to the test it marks.
func MarkerLines(path, text string) []MarkerLine {
	if !strings.Contains(text, "Verifies:") {
		return nil
	}
	lines := strings.Split(text, "\n")
	var found []MarkerLine
	for index, line := range lines {
		if !markerStart.MatchString(line) {
			continue
		}
		following := []string{}
		for _, next := range lines[index+1:] {
			if len(following) >= maxFollowingLines {
				break
			}
			following = append(following, truncate(next))
			if !isCommentOrBlank(next) {
				break
			}
		}
		found = append(found, MarkerLine{Path: path, Line: index + 1, Text: line, Following: following})
	}
	return found
}

func isCommentOrBlank(line string) bool {
	stripped := strings.TrimSpace(line)
	if stripped == "" {
		return true
	}
	for _, start := range commentStarts {
		if strings.HasPrefix(stripped, start) {
			return true
		}
	}
	return false
}

func sendJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(body); err != nil {
		slog.Default().Warn("spec viewer: writing a response failed", "err", err)
	}
}

func sendLoadError(w http.ResponseWriter, err error) {
	var unknown errUnknownRef
	switch {
	case errors.Is(err, errNotConfigured), errors.As(err, &unknown):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, context.Canceled):
		// The browser went away; nobody reads the answer.
	default:
		// GitHub errors name only the public repository and a status, so
		// they are safe to show and say what to fix.
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
