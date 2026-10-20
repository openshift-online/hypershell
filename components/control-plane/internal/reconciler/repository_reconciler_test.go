package reconciler

import (
	"context"
	"errors"
	"sort"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// ---- fakes ----

type fakeRepoClient struct {
	pb.RepositoryServiceClient
	repos []*pb.Repository
}

func (f *fakeRepoClient) ListRepositories(ctx context.Context, in *pb.ListRepositoriesRequest, opts ...grpc.CallOption) (*pb.ListRepositoriesResponse, error) {
	if in.GetPage() > 1 {
		return &pb.ListRepositoriesResponse{}, nil
	}
	return &pb.ListRepositoriesResponse{Items: f.repos}, nil
}

type fakeFetcher struct {
	fsys  filesys.FileSystem
	root  string
	err   error
	calls []string // tokens seen
}

func (f *fakeFetcher) Fetch(ctx context.Context, repo *pb.Repository, token string) (filesys.FileSystem, string, error) {
	f.calls = append(f.calls, token)
	if f.err != nil {
		return nil, "", f.err
	}
	return f.fsys, f.root, nil
}

type fakeResolver struct {
	token string
	err   error
}

func (f *fakeResolver) Resolve(ctx context.Context, repo *pb.Repository) (string, error) {
	return f.token, f.err
}

// fakeImportWriter records writes and keeps a per-repository owned store so prune
// behaviour is observable across reconcile passes.
type fakeImportWriter struct {
	owned     map[string]map[ResourceRef]RenderedResource
	upserts   []RenderedResource
	deletes   []ResourceRef
	failKinds map[string]error
}

func newFakeImportWriter() *fakeImportWriter {
	return &fakeImportWriter{owned: map[string]map[ResourceRef]RenderedResource{}}
}

func (w *fakeImportWriter) Upsert(ctx context.Context, repoID string, res RenderedResource) error {
	if err := w.failKinds[res.Kind]; err != nil {
		return err
	}
	w.upserts = append(w.upserts, res)
	if w.owned[repoID] == nil {
		w.owned[repoID] = map[ResourceRef]RenderedResource{}
	}
	w.owned[repoID][ResourceRef{Kind: res.Kind, Name: res.Name}] = res
	return nil
}

func (w *fakeImportWriter) ListOwned(ctx context.Context, repoID string) ([]ResourceRef, error) {
	var refs []ResourceRef
	for ref := range w.owned[repoID] {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		return refs[i].Name < refs[j].Name
	})
	return refs, nil
}

func (w *fakeImportWriter) Delete(ctx context.Context, repoID string, ref ResourceRef) error {
	w.deletes = append(w.deletes, ref)
	delete(w.owned[repoID], ResourceRef{Kind: ref.Kind, Name: ref.Name})
	return nil
}

// ---- helpers ----

func inMemFS(t *testing.T, files map[string]string) filesys.FileSystem {
	t.Helper()
	fSys := filesys.MakeFsInMemory()
	for path, contents := range files {
		if err := fSys.WriteFile(path, []byte(contents)); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return fSys
}

func repo(id string) *pb.Repository {
	return &pb.Repository{Metadata: &pb.ObjectReference{Id: id}, Name: "repo-" + id, Url: "https://github.com/acme/" + id}
}

// ---- renderImportables: 7-kind filter, Repository/admin exclusion, identity ----

func TestRenderImportables_FilterAndIdentity(t *testing.T) {
	fSys := inMemFS(t, map[string]string{
		"/root/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - resources.yaml
`,
		"/root/resources.yaml": `apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
metadata:
  name: tmpl-1
spec:
  image: example.com/img:v1
---
apiVersion: hypershell.redhat.io/v1
kind: ProviderSpec
metadata:
  name: ps-1
spec:
  category: inference
---
apiVersion: hypershell.redhat.io/v1
kind: ProviderBinding
metadata:
  name: pb-1
spec:
  workspace_id: ws-1
  provider_spec_id: ps-1
---
apiVersion: hypershell.redhat.io/v1
kind: SecretSource
metadata:
  name: ss-1
spec:
  backend: vault
  path: secret/data/x
---
apiVersion: hypershell.redhat.io/v1
kind: InferenceRoute
metadata:
  name: ir-1
spec:
  workspace_id: ws-1
  provider_binding_id: pb-1
  model: gpt
---
apiVersion: hypershell.redhat.io/v1
kind: AgentRuntime
metadata:
  name: ar-1
spec:
  selector: []
---
apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: gw-1
spec:
  cluster_id: c1
---
apiVersion: hypershell.redhat.io/v1
kind: Repository
metadata:
  name: nested-repo
spec:
  url: https://github.com/acme/nested
---
apiVersion: hypershell.redhat.io/v1
kind: ManagedCluster
metadata:
  name: mc-1
spec:
  provider: aws
---
apiVersion: hypershell.redhat.io/v1
kind: Role
metadata:
  name: role-1
---
apiVersion: hypershell.redhat.io/v1
kind: RoleBinding
metadata:
  name: rb-1
`,
	})

	got, err := renderImportables(fSys, "/root")
	if err != nil {
		t.Fatalf("renderImportables() error = %v", err)
	}

	// Exactly the seven importable kinds, Repository + admin kinds excluded.
	want := map[string]string{
		"SandboxTemplate": "tmpl-1",
		"ProviderSpec":    "ps-1",
		"ProviderBinding": "pb-1",
		"SecretSource":    "ss-1",
		"InferenceRoute":  "ws-1|pb-1|gpt",
		"AgentRuntime":    "ar-1",
		"Gateway":         "gw-1",
	}
	if len(got) != len(want) {
		t.Fatalf("renderImportables() returned %d resources, want %d: %+v", len(got), len(want), got)
	}
	for _, r := range got {
		if r.Kind == "Repository" || r.Kind == "ManagedCluster" || r.Kind == "Role" || r.Kind == "RoleBinding" {
			t.Errorf("non-importable kind %s leaked through the filter", r.Kind)
		}
		if want[r.Kind] != r.Name {
			t.Errorf("kind %s identity = %q, want %q", r.Kind, r.Name, want[r.Kind])
		}
	}
}

func TestRenderImportables_MissingName(t *testing.T) {
	fSys := inMemFS(t, map[string]string{
		"/root/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - r.yaml
`,
		"/root/r.yaml": `apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
spec:
  image: example.com/img:v1
`,
	})
	_, err := renderImportables(fSys, "/root")
	if err == nil {
		t.Fatal("renderImportables() expected error for missing metadata.name, got nil")
	}
}

// ---- reconcileRepository: upsert, prune, ownership scoping, credential ----

func newReconciler(repos *fakeRepoClient, resolver CredentialResolver, fetcher SubtreeFetcher, writer ImportWriter) *RepositoryReconciler {
	return NewRepositoryReconciler(repos, resolver, fetcher, writer, 0)
}

func twoResourceFS(t *testing.T) filesys.FileSystem {
	return inMemFS(t, map[string]string{
		"/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - r.yaml
`,
		"/r.yaml": `apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
metadata:
  name: keep
spec:
  image: img:v1
---
apiVersion: hypershell.redhat.io/v1
kind: ProviderSpec
metadata:
  name: drop-me
spec:
  category: inference
`,
	})
}

func TestReconcileRepository_UpsertThenPrune(t *testing.T) {
	writer := newFakeImportWriter()
	fetcher := &fakeFetcher{fsys: twoResourceFS(t), root: "/"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "t"}, fetcher, writer)

	rp := repo("abc")
	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("first reconcile error = %v", err)
	}
	if len(writer.upserts) != 2 {
		t.Fatalf("first pass upserts = %d, want 2", len(writer.upserts))
	}
	if len(writer.deletes) != 0 {
		t.Fatalf("first pass deletes = %d, want 0", len(writer.deletes))
	}

	// Second pass: repository now declares only "keep"; "drop-me" must be pruned.
	fetcher.fsys = inMemFS(t, map[string]string{
		"/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - r.yaml
`,
		"/r.yaml": `apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
metadata:
  name: keep
spec:
  image: img:v1
`,
	})
	writer.upserts = nil
	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("second reconcile error = %v", err)
	}
	if len(writer.deletes) != 1 || writer.deletes[0].Name != "drop-me" {
		t.Fatalf("second pass deletes = %+v, want one delete of drop-me", writer.deletes)
	}
	if _, stillOwned := writer.owned["abc"][ResourceRef{Kind: "ProviderSpec", Name: "drop-me"}]; stillOwned {
		t.Error("drop-me still owned after prune")
	}
}

func TestReconcileRepository_PruneScopedPerRepository(t *testing.T) {
	writer := newFakeImportWriter()
	// repoA owns a resource not present in repoB's import; reconciling repoB must
	// not prune repoA's resource.
	writer.owned["A"] = map[ResourceRef]RenderedResource{
		{Kind: "SandboxTemplate", Name: "a-only"}: {Kind: "SandboxTemplate", Name: "a-only"},
	}
	fetcher := &fakeFetcher{fsys: inMemFS(t, map[string]string{
		"/kustomization.yaml": "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - r.yaml\n",
		"/r.yaml": `apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
metadata:
  name: b-only
spec:
  image: img:v1
`,
	}), root: "/"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "t"}, fetcher, writer)

	if err := r.reconcileRepository(context.Background(), repo("B"), false); err != nil {
		t.Fatalf("reconcile error = %v", err)
	}
	if len(writer.deletes) != 0 {
		t.Fatalf("reconciling repository B pruned %+v; must not touch repository A", writer.deletes)
	}
	if _, ok := writer.owned["A"][ResourceRef{Kind: "SandboxTemplate", Name: "a-only"}]; !ok {
		t.Error("repository A resource was removed by reconciling repository B")
	}
}

func TestReconcileRepository_CredentialPassedToFetcher(t *testing.T) {
	writer := newFakeImportWriter()
	fetcher := &fakeFetcher{fsys: twoResourceFS(t), root: "/"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "secret-token"}, fetcher, writer)
	if err := r.reconcileRepository(context.Background(), repo("abc"), false); err != nil {
		t.Fatalf("reconcile error = %v", err)
	}
	if len(fetcher.calls) != 1 || fetcher.calls[0] != "secret-token" {
		t.Fatalf("fetcher received tokens %v, want [secret-token]", fetcher.calls)
	}
}

// fakeConditionalFetcher implements ConditionalSubtreeFetcher so the conditional
// upstream fetch can be exercised. revision is the current branch head; fetchCount
// and revCount record how often each path ran so a test can assert the heavy read
// was skipped.
type fakeConditionalFetcher struct {
	fsys       filesys.FileSystem
	root       string
	revision   string
	revErr     error
	fetchCount int
	revCount   int
}

func (f *fakeConditionalFetcher) Fetch(ctx context.Context, repo *pb.Repository, token string) (filesys.FileSystem, string, error) {
	f.fetchCount++
	return f.fsys, f.root, nil
}

func (f *fakeConditionalFetcher) HeadRevision(ctx context.Context, repo *pb.Repository, token string) (string, error) {
	f.revCount++
	return f.revision, f.revErr
}

func TestReconcileRepository_ConditionalSkipsUnchanged(t *testing.T) {
	writer := newFakeImportWriter()
	fetcher := &fakeConditionalFetcher{fsys: twoResourceFS(t), root: "/", revision: "sha-1"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "t"}, fetcher, writer)
	rp := repo("abc")

	// First periodic pass: no recorded revision yet, so it reads in full and records sha-1.
	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("first reconcile error = %v", err)
	}
	if fetcher.fetchCount != 1 {
		t.Fatalf("first pass fetchCount = %d, want 1", fetcher.fetchCount)
	}
	writer.upserts = nil

	// Second periodic pass at the same revision: skip the heavy read and make no write.
	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("second reconcile error = %v", err)
	}
	if fetcher.fetchCount != 1 {
		t.Errorf("second pass fetchCount = %d; an unchanged subtree must skip the heavy read", fetcher.fetchCount)
	}
	if len(writer.upserts) != 0 || len(writer.deletes) != 0 {
		t.Errorf("unchanged pass wrote: upserts=%+v deletes=%+v", writer.upserts, writer.deletes)
	}
}

func TestReconcileRepository_ConditionalReadsOnChange(t *testing.T) {
	writer := newFakeImportWriter()
	fetcher := &fakeConditionalFetcher{fsys: twoResourceFS(t), root: "/", revision: "sha-1"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "t"}, fetcher, writer)
	rp := repo("abc")

	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("first reconcile error = %v", err)
	}
	// Upstream advances; the periodic drift re-check must read and reconcile again.
	fetcher.revision = "sha-2"
	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("second reconcile error = %v", err)
	}
	if fetcher.fetchCount != 2 {
		t.Errorf("changed-revision fetchCount = %d, want 2 (a new commit must trigger a full read)", fetcher.fetchCount)
	}
}

func TestHandle_WatchEventForcesImport(t *testing.T) {
	writer := newFakeImportWriter()
	fetcher := &fakeConditionalFetcher{fsys: twoResourceFS(t), root: "/", revision: "sha-1"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "t"}, fetcher, writer)
	rp := repo("abc")

	// Seed the revision cache with a full pass.
	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("seed reconcile error = %v", err)
	}
	// A watch update at the SAME revision (e.g. the Repository's import_path changed)
	// must still force a full re-import, because a field change can select a different
	// subtree at an unchanged branch head.
	if err := r.Handle(context.Background(), watcher.Event[*pb.Repository]{Type: watcher.EventUpdated, Resource: rp}); err != nil {
		t.Fatalf("Handle error = %v", err)
	}
	if fetcher.fetchCount != 2 {
		t.Errorf("watch-event fetchCount = %d, want 2 (an event must force a full read regardless of revision)", fetcher.fetchCount)
	}
}

func TestHandle_DeleteForgetsRevision(t *testing.T) {
	writer := newFakeImportWriter()
	fetcher := &fakeConditionalFetcher{fsys: twoResourceFS(t), root: "/", revision: "sha-1"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "t"}, fetcher, writer)
	rp := repo("abc")

	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("seed reconcile error = %v", err)
	}
	// A delete event drops the cached revision and performs no read.
	if err := r.Handle(context.Background(), watcher.Event[*pb.Repository]{Type: watcher.EventDeleted, Resource: rp}); err != nil {
		t.Fatalf("Handle delete error = %v", err)
	}
	if r.revisionUnchanged("abc", "sha-1") {
		t.Error("delete did not forget the cached revision")
	}
	// A later periodic pass at the same revision now reads in full again.
	if err := r.reconcileRepository(context.Background(), rp, false); err != nil {
		t.Fatalf("post-delete reconcile error = %v", err)
	}
	if fetcher.fetchCount != 2 {
		t.Errorf("post-delete fetchCount = %d, want 2 (forgetting the revision must force a re-read)", fetcher.fetchCount)
	}
}

// errUpsertFailed is a sentinel used to confirm the reconciler collects (never
// swallows) a writer error for one resource while continuing the rest of the import.
var errUpsertFailed = errors.New("upsert failed")

func TestReconcileRepository_CollectsUpsertErrors(t *testing.T) {
	writer := newFakeImportWriter()
	// A writer error on one Kind must surface as a collected error, not abort the
	// import. All seven Kinds are writable now, so this tests generic error
	// collection rather than a not-writable classification.
	writer.failKinds = map[string]error{"Gateway": errUpsertFailed}
	fetcher := &fakeFetcher{fsys: inMemFS(t, map[string]string{
		"/kustomization.yaml": "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - r.yaml\n",
		"/r.yaml": `apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: gw-1
spec:
  cluster_id: c1
---
apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
metadata:
  name: tmpl-1
spec:
  image: img:v1
`,
	}), root: "/"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "t"}, fetcher, writer)
	err := r.reconcileRepository(context.Background(), repo("abc"), false)
	if err == nil {
		t.Fatal("expected a collected error for the failing upsert, got nil")
	}
	if !errors.Is(err, errUpsertFailed) {
		t.Errorf("error = %v, want it to wrap errUpsertFailed", err)
	}
	// The healthy resource was still upserted despite the failure on the other Kind.
	if _, ok := writer.owned["abc"][ResourceRef{Kind: "SandboxTemplate", Name: "tmpl-1"}]; !ok {
		t.Error("the healthy SandboxTemplate was not upserted; a single failure aborted the import")
	}
}

// TestReconcileRepository_AllSevenKindsImport confirms every importable Kind flows
// through the writer's upsert path (no not-writable branch) in a single import.
func TestReconcileRepository_AllSevenKindsImport(t *testing.T) {
	writer := newFakeImportWriter()
	fetcher := &fakeFetcher{fsys: inMemFS(t, map[string]string{
		"/kustomization.yaml": "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - r.yaml\n",
		"/r.yaml": `apiVersion: hypershell.redhat.io/v1
kind: SandboxTemplate
metadata:
  name: tmpl-1
spec:
  image: img:v1
---
apiVersion: hypershell.redhat.io/v1
kind: ProviderSpec
metadata:
  name: ps-1
spec:
  category: inference
---
apiVersion: hypershell.redhat.io/v1
kind: ProviderBinding
metadata:
  name: pb-1
spec:
  workspace_id: ws-1
  provider_spec_id: ps-1
---
apiVersion: hypershell.redhat.io/v1
kind: SecretSource
metadata:
  name: ss-1
spec:
  backend: vault
  path: secret/data/x
---
apiVersion: hypershell.redhat.io/v1
kind: InferenceRoute
metadata:
  name: ir-1
spec:
  workspace_id: ws-1
  provider_binding_id: pb-1
  model: gpt
---
apiVersion: hypershell.redhat.io/v1
kind: AgentRuntime
metadata:
  name: ar-1
spec:
  gateway_id: gw-1
---
apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: gw-1
spec:
  cluster_id: c1
`,
	}), root: "/"}
	r := newReconciler(&fakeRepoClient{}, &fakeResolver{token: "t"}, fetcher, writer)
	if err := r.reconcileRepository(context.Background(), repo("abc"), false); err != nil {
		t.Fatalf("reconcile error = %v", err)
	}
	if len(writer.upserts) != 7 {
		t.Fatalf("upserts = %d, want 7 (all importable kinds)", len(writer.upserts))
	}
	for _, kind := range []string{"SandboxTemplate", "ProviderSpec", "ProviderBinding", "SecretSource", "InferenceRoute", "AgentRuntime", "Gateway"} {
		found := false
		for _, u := range writer.upserts {
			if u.Kind == kind {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("kind %s was not upserted; it hit a not-writable branch", kind)
		}
	}
}

func TestReconcileOnce_EnumeratesRepositories(t *testing.T) {
	writer := newFakeImportWriter()
	fetcher := &fakeFetcher{fsys: twoResourceFS(t), root: "/"}
	repos := &fakeRepoClient{repos: []*pb.Repository{repo("one"), repo("two")}}
	r := newReconciler(repos, &fakeResolver{token: "t"}, fetcher, writer)
	r.reconcileOnce(context.Background())
	if len(fetcher.calls) != 2 {
		t.Fatalf("fetched %d repositories, want 2", len(fetcher.calls))
	}
}

// ---- ownership metadata ----

func TestWithOwnership_AppendsImportBinding(t *testing.T) {
	ctx := withOwnership(context.Background(), "repo-123")
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("no outgoing metadata set")
	}
	got := md.Get(importRepositoryMetadataKey)
	if len(got) != 1 || got[0] != "repo-123" {
		t.Fatalf("metadata %q = %v, want [repo-123]", importRepositoryMetadataKey, got)
	}
	if importRepositoryMetadataKey != "x-hypershell-import-repository-id" {
		t.Fatalf("import metadata key drifted from the #585 wire contract: %q", importRepositoryMetadataKey)
	}
}
