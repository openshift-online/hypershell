package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// bundleTagPrefix is the product-level tag prefix the release-bundle pipeline
// publishes (pipelines/release-bundle). It is not fleet-identifying.
const bundleTagPrefix = "release-bundle-"

// bundleTagLimit bounds the tag listing. One page of recent bundle tags spans far
// more bundles than can be pending at once.
const bundleTagLimit = 50

// RegistryBundle is one published release bundle read from the registry: its tag,
// manifest digest, publication time, and (once read) the application revision its
// manifests were built from.
type RegistryBundle struct {
	Tag          string
	Digest       string
	PublishedAt  time.Time
	ManifestsRev string
}

// BundleRegistry lists published release bundles and reads their bundle.json.
type BundleRegistry interface {
	// ListBundles returns the recent release-bundle tags, newest first by tag.
	ListBundles(ctx context.Context) ([]RegistryBundle, error)
	// ManifestsRevision returns bundle.json's manifests.git.revision for the
	// manifest digest. Implementations cache it forever: bundle content is
	// immutable (FLT-DLV-12).
	ManifestsRevision(ctx context.Context, digest string) (string, error)
}

// QuayBundleRegistry reads a public Quay repository: the tag listing uses Quay's
// filtered tag API (a build repository holds thousands of build tags, so the
// generic tags/list call would page through all of them), and bundle.json is read
// over the standard OCI distribution API. The repository arrives from config
// (FD_DELIVERY_BUNDLE_REPOSITORY, host/namespace/repo); nothing is compiled in.
type QuayBundleRegistry struct {
	host   string // e.g. quay.io
	repo   string // namespace/.../repo
	scheme string // https, or http in tests
	client *http.Client

	mu        sync.Mutex
	revisions map[string]string // manifest digest -> manifests revision (immutable)
	token     string            // anonymous pull token, refreshed on a 401
}

// NewQuayBundleRegistry parses "host/namespace/repo". It returns nil when the
// value has no repository path.
func NewQuayBundleRegistry(repository string) *QuayBundleRegistry {
	scheme := "https"
	if rest, ok := strings.CutPrefix(repository, "http://"); ok {
		scheme, repository = "http", rest
	} else {
		repository = strings.TrimPrefix(repository, "https://")
	}
	host, repo, ok := strings.Cut(strings.Trim(repository, "/"), "/")
	if !ok || host == "" || repo == "" {
		return nil
	}
	return &QuayBundleRegistry{
		host:      host,
		repo:      repo,
		scheme:    scheme,
		client:    &http.Client{Timeout: 15 * time.Second},
		revisions: map[string]string{},
	}
}

type quayTagPage struct {
	Tags []struct {
		Name           string `json:"name"`
		ManifestDigest string `json:"manifest_digest"`
		StartTS        int64  `json:"start_ts"`
	} `json:"tags"`
}

// ListBundles implements BundleRegistry with one tag-list call (FLT-DLV-12).
func (q *QuayBundleRegistry) ListBundles(ctx context.Context) ([]RegistryBundle, error) {
	u := fmt.Sprintf("%s://%s/api/v1/repository/%s/tag/?onlyActiveTags=true&limit=%d&filter_tag_name=%s",
		q.scheme, q.host, q.repo, bundleTagLimit, url.QueryEscape("like:"+bundleTagPrefix+"%"))
	var page quayTagPage
	if err := q.getJSON(ctx, u, "", &page); err != nil {
		return nil, fmt.Errorf("list bundle tags: %w", err)
	}
	out := make([]RegistryBundle, 0, len(page.Tags))
	for _, t := range page.Tags {
		if !strings.HasPrefix(t.Name, bundleTagPrefix) || t.ManifestDigest == "" {
			continue
		}
		b := RegistryBundle{Tag: t.Name, Digest: t.ManifestDigest}
		if t.StartTS > 0 {
			b.PublishedAt = time.Unix(t.StartTS, 0).UTC()
		}
		q.mu.Lock()
		b.ManifestsRev = q.revisions[t.ManifestDigest]
		q.mu.Unlock()
		out = append(out, b)
	}
	// The tag carries the snapshot timestamp, so a lexical sort is chronological.
	sort.Slice(out, func(i, j int) bool { return out[i].Tag > out[j].Tag })
	return out, nil
}

type ociManifest struct {
	Layers []struct {
		MediaType   string            `json:"mediaType"`
		Digest      string            `json:"digest"`
		Annotations map[string]string `json:"annotations"`
	} `json:"layers"`
}

type bundleFile struct {
	Manifests struct {
		Git struct {
			Revision string `json:"revision"`
		} `json:"git"`
	} `json:"manifests"`
}

// ManifestsRevision implements BundleRegistry: one manifest read and one blob
// read per new digest, cached forever.
func (q *QuayBundleRegistry) ManifestsRevision(ctx context.Context, digest string) (string, error) {
	q.mu.Lock()
	if rev, ok := q.revisions[digest]; ok {
		q.mu.Unlock()
		return rev, nil
	}
	q.mu.Unlock()

	var m ociManifest
	mu := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", q.scheme, q.host, q.repo, digest)
	if err := q.getRegistryJSON(ctx, mu, "application/vnd.oci.image.manifest.v1+json", &m); err != nil {
		return "", fmt.Errorf("read bundle manifest: %w", err)
	}
	layer := ""
	for _, l := range m.Layers {
		if l.Annotations["org.opencontainers.image.title"] == "bundle.json" {
			layer = l.Digest
			break
		}
	}
	if layer == "" {
		return "", fmt.Errorf("bundle manifest %s has no bundle.json layer", digest)
	}
	var b bundleFile
	bu := fmt.Sprintf("%s://%s/v2/%s/blobs/%s", q.scheme, q.host, q.repo, layer)
	if err := q.getRegistryJSON(ctx, bu, "", &b); err != nil {
		return "", fmt.Errorf("read bundle.json: %w", err)
	}
	rev := strings.TrimSpace(b.Manifests.Git.Revision)
	if rev == "" {
		return "", fmt.Errorf("bundle %s has no manifests revision", digest)
	}
	q.mu.Lock()
	q.revisions[digest] = rev
	q.mu.Unlock()
	return rev, nil
}

// wwwAuthenticate pulls realm, service, and scope out of a Bearer challenge.
var wwwAuthenticate = regexp.MustCompile(`(\w+)="([^"]*)"`)

// getRegistryJSON reads a registry v2 resource, answering one anonymous Bearer
// challenge when the registry asks for a pull token.
func (q *QuayBundleRegistry) getRegistryJSON(ctx context.Context, u, accept string, v any) error {
	q.mu.Lock()
	tok := q.token
	q.mu.Unlock()
	err := q.getJSON(ctx, u, accept, v, tok)
	var ch *authChallenge
	if !asChallenge(err, &ch) {
		return err
	}
	params := map[string]string{}
	for _, m := range wwwAuthenticate.FindAllStringSubmatch(ch.header, -1) {
		params[m[1]] = m[2]
	}
	if params["realm"] == "" {
		return err
	}
	tu, perr := url.Parse(params["realm"])
	if perr != nil {
		return err
	}
	qv := tu.Query()
	for _, k := range []string{"service", "scope"} {
		if params[k] != "" {
			qv.Set(k, params[k])
		}
	}
	tu.RawQuery = qv.Encode()
	var tr struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if terr := q.getJSON(ctx, tu.String(), "", &tr); terr != nil {
		return fmt.Errorf("anonymous registry token: %w", terr)
	}
	tok = tr.Token
	if tok == "" {
		tok = tr.AccessToken
	}
	q.mu.Lock()
	q.token = tok
	q.mu.Unlock()
	return q.getJSON(ctx, u, accept, v, tok)
}

// authChallenge is returned for a 401 that carries a WWW-Authenticate header.
type authChallenge struct{ header string }

func (a *authChallenge) Error() string { return "registry requires a token" }

func asChallenge(err error, out **authChallenge) bool {
	c, ok := err.(*authChallenge)
	if ok {
		*out = c
	}
	return ok
}

func (q *QuayBundleRegistry) getJSON(ctx context.Context, u, accept string, v any, token ...string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if len(token) > 0 && token[0] != "" {
		req.Header.Set("Authorization", "Bearer "+token[0])
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		if h := resp.Header.Get("WWW-Authenticate"); h != "" {
			return &authChallenge{header: h}
		}
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}
