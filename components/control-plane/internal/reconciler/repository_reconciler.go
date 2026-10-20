package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/cli/pkg/kustomize"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// ImportableKinds is the canonical set of Kinds the RepositoryReconciler imports
// from a `.hypershell` root, defined in data-model.spec.md ("Declarative
// `.hypershell` Import"): exactly seven. It is a SUBSET of the Kinds `hsctl apply`
// accepts; `hsctl apply` also accepts platform-admin Kinds (ManagedCluster, Role,
// RoleBinding) that are NOT importable through a Repository. `Repository` itself is
// explicitly EXCLUDED so that importing one repository cannot register further
// repositories (recursive/transitive import).
var ImportableKinds = map[string]struct{}{
	"AgentRuntime":    {},
	"SandboxTemplate": {},
	"ProviderSpec":    {},
	"ProviderBinding": {},
	"InferenceRoute":  {},
	"SecretSource":    {},
	"Gateway":         {},
}

// isImportableKind reports whether kind is one of the canonical importable Kinds.
// Repository and the platform-admin Kinds (ManagedCluster, Role, RoleBinding) are
// deliberately absent, so a `.hypershell` root that declares them is ignored by
// the import rather than upserted.
func isImportableKind(kind string) bool {
	_, ok := ImportableKinds[kind]
	return ok
}

// RenderedResource is one HyperShell resource extracted from the rendered
// kustomization: its Kind, its metadata.name, and its spec body. The reconciler
// never carries raw secret values here; a SecretSource declaration references a
// stored value by path, it does not embed the value.
type RenderedResource struct {
	Kind string
	Name string
	Spec map[string]interface{}
}

// ResourceRef identifies a Repository-owned resource already persisted in the API,
// used to drive prune decisions.
type ResourceRef struct {
	Kind string
	Name string
	ID   string
}

// SubtreeFetcher reads ONLY the import_path subtree of a Repository (default
// `.hypershell`) and returns an in-memory filesystem containing it plus the
// kustomize root path within that filesystem. Implementations SHALL NOT clone the
// repository, write to disk, or shell out: the whole read is in-process and
// in-memory (control-plane.spec.md "Import loop is in-process with no disk or
// external process"). token is the bearer credential resolved for this
// Repository.
type SubtreeFetcher interface {
	Fetch(ctx context.Context, repo *pb.Repository, token string) (fsys filesys.FileSystem, root string, err error)
}

// ConditionalSubtreeFetcher is an OPTIONAL capability a SubtreeFetcher MAY
// implement so the reconciler can skip the heavy subtree read when the upstream has
// not changed. HeadRevision returns an opaque revision id (the import branch's head
// commit SHA) for the Repository through a single lightweight request. The
// reconciler compares it with the revision it last imported and, when unchanged,
// skips Fetch, render, and every API write (control-plane.spec.md, "Conditional
// upstream fetch"). A fetcher that does not implement this interface is always read
// in full.
type ConditionalSubtreeFetcher interface {
	SubtreeFetcher
	HeadRevision(ctx context.Context, repo *pb.Repository, token string) (revision string, err error)
}

// CredentialResolver resolves the bearer token used to read a Repository's
// subtree. The platform GitHub App credential is the default transport; when the
// Repository declares a secret_source_id, that per-Repository credential takes
// precedence (control-plane.spec.md step 1).
type CredentialResolver interface {
	Resolve(ctx context.Context, repo *pb.Repository) (token string, err error)
}

// ImportWriter upserts and prunes the Repository-owned ext resources the import
// renders, over the generated gRPC write path. Every write is marked server-side
// with its owning Repository (the x-hypershell-import-repository-id binding), and
// prune is scoped to a single Repository so one import never deletes another
// Repository's resources or resources created directly over REST / `hsctl apply`.
type ImportWriter interface {
	// ListOwned returns every resource currently owned by repoID.
	ListOwned(ctx context.Context, repoID string) ([]ResourceRef, error)
	// Upsert creates the resource when absent and updates it when present, marking
	// it owned by repoID. An error for one resource is collected by the caller
	// rather than aborting the rest of the import. All seven importable Kinds have a
	// control-plane import write path.
	Upsert(ctx context.Context, repoID string, res RenderedResource) error
	// Delete removes one Repository-owned resource.
	Delete(ctx context.Context, repoID string, ref ResourceRef) error
}

// RepositoryReconciler is the system's single git-to-API sync component (the
// declarative `.hypershell` import reconciler). For each registered Repository it
// reads only the import_path subtree over the GitHub API, renders it in memory
// through the shared pkg/kustomize library, filters to the canonical importable
// Kinds, and reconciles (create/update/prune) the rendered ext resources into the
// API over the gRPC write path, marking each with its owning Repository. It does
// NOT provision any resource cluster-side: each imported Kind is reconciled into
// the cluster by its own per-Kind reconciler. See control-plane.spec.md
// ("Declarative `.hypershell` Import Reconciliation").
//
// The reconciler is driven two ways and is idempotent either way: the Repository
// watch stream reconciles a created or changed Repository in real time (Handle),
// while a slow periodic pass (Run) re-checks every Repository for upstream drift.
// Both make the upstream read conditional on the branch head revision, so repeated
// passes over unchanged repository state are side-effect free and cost no heavy
// GitHub reads.
type RepositoryReconciler struct {
	repos    pb.RepositoryServiceClient
	creds    CredentialResolver
	fetcher  SubtreeFetcher
	writer   ImportWriter
	interval time.Duration
	// listPageSize bounds the Repository enumeration page.
	listPageSize int32

	// mu guards lastRevision and repoLocks.
	mu sync.Mutex
	// lastRevision records the branch head revision last imported per Repository id,
	// so the conditional fetch can skip an unchanged subtree. It is in-memory only:
	// after a restart it is empty, so the first pass reads in full (control-plane.spec.md,
	// "Conditional upstream fetch").
	lastRevision map[string]string
	// repoLocks serializes reconciles of the SAME Repository so the periodic pass and
	// a concurrent watch event never race on list-then-write; distinct Repositories
	// still reconcile concurrently.
	repoLocks map[string]*sync.Mutex
}

// defaultDriftInterval is the periodic drift re-check cadence when the caller passes
// a non-positive interval. It is deliberately slow (hourly): real-time import is
// driven by the watch stream, and the conditional fetch keeps each periodic tick
// cheap.
const defaultDriftInterval = time.Hour

// NewRepositoryReconciler builds the import reconciler. interval is the periodic
// drift re-check cadence; a non-positive interval defaults to one hour.
func NewRepositoryReconciler(repos pb.RepositoryServiceClient, creds CredentialResolver, fetcher SubtreeFetcher, writer ImportWriter, interval time.Duration) *RepositoryReconciler {
	if interval <= 0 {
		interval = defaultDriftInterval
	}
	return &RepositoryReconciler{
		repos:        repos,
		creds:        creds,
		fetcher:      fetcher,
		writer:       writer,
		interval:     interval,
		listPageSize: 100,
		lastRevision: map[string]string{},
		repoLocks:    map[string]*sync.Mutex{},
	}
}

// Run drives the SLOW periodic drift re-check until ctx is cancelled. It reconciles
// every registered Repository once immediately (which also seeds the Repositories
// already registered at startup, since the watch stream does not replay them), then
// again on each tick. Real-time response to a created or changed Repository is
// handled by the watch stream through Handle, not this loop.
func (r *RepositoryReconciler) Run(ctx context.Context) error {
	log.Printf("INFO repository import reconciler started (drift re-check interval=%s, event-driven via Repository watch)", r.interval)
	r.reconcileOnce(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

// Handle imports a single Repository in response to a watch event, driving the
// event-driven import. A create or update forces a full re-import (bypassing the
// conditional revision check) because a changed Repository field - its branch or
// import_path - can select a different subtree even when the branch head revision is
// unchanged. A delete drops the cached revision; pruning a deleted Repository's
// resources is out of scope (the api-server blocks deleting a referenced Repository).
func (r *RepositoryReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Repository]) error {
	repo := event.Resource
	if repo == nil {
		return nil
	}
	repoID := repo.GetMetadata().GetId()
	if event.Type == watcher.EventDeleted {
		r.forget(repoID)
		return nil
	}
	// Return the error without logging it here: WatchRepositories logs a failed
	// event-driven import once, so logging again would double-report it.
	return r.reconcileRepository(ctx, repo, true)
}

// reconcileOnce enumerates every registered Repository and reconciles each one. A
// failure reconciling one Repository is logged and does not abort the others: the
// loop is best-effort per Repository and retried on the next tick.
func (r *RepositoryReconciler) reconcileOnce(ctx context.Context) {
	repos, err := r.listRepositories(ctx)
	if err != nil {
		log.Printf("WARN repository import: listing repositories failed: %v", err)
		return
	}
	for _, repo := range repos {
		// force=false: the periodic pass uses the conditional head-revision check and
		// skips the heavy read when the subtree is unchanged.
		if err := r.reconcileRepository(ctx, repo, false); err != nil {
			log.Printf("WARN repository import: reconciling repository %s (%s) failed: %v", repo.GetName(), repo.GetMetadata().GetId(), err)
		}
	}
}

// listRepositories pages through every registered Repository.
func (r *RepositoryReconciler) listRepositories(ctx context.Context) ([]*pb.Repository, error) {
	var out []*pb.Repository
	for page := int32(1); ; page++ {
		resp, err := r.repos.ListRepositories(ctx, &pb.ListRepositoriesRequest{Page: page, Size: r.listPageSize})
		if err != nil {
			return nil, fmt.Errorf("list repositories (page %d): %w", page, err)
		}
		items := resp.GetItems()
		out = append(out, items...)
		if int32(len(items)) < r.listPageSize {
			break
		}
	}
	return out, nil
}

// reconcileRepository performs the import for one Repository: resolve the
// credential, (conditionally) read the subtree, render it, filter to the importable
// Kinds, upsert each, then prune Repository-owned resources no longer present.
// Errors from individual resources are collected so one bad declaration does not
// hide the rest, and prune is scoped to this Repository only. When force is false
// and the fetcher supports a head-revision probe, an unchanged subtree is skipped
// before any heavy read. Reconciles of the same Repository are serialized so the
// periodic pass and a concurrent watch event never race.
func (r *RepositoryReconciler) reconcileRepository(ctx context.Context, repo *pb.Repository, force bool) error {
	repoID := repo.GetMetadata().GetId()
	if repoID == "" {
		return fmt.Errorf("repository %q has no id", repo.GetName())
	}

	unlock := r.lockRepository(repoID)
	defer unlock()

	token, err := r.creds.Resolve(ctx, repo)
	if err != nil {
		return fmt.Errorf("resolve credential: %w", err)
	}

	// Conditional upstream fetch: when the fetcher can report the branch head
	// revision, skip the heavy tree+blob read if nothing changed since the last
	// successful import. A forced reconcile (watch event) always reads in full
	// because a Repository field change can select a different subtree at the same
	// revision. A probe failure is non-fatal: fall back to the full read.
	var revision string
	if cond, ok := r.fetcher.(ConditionalSubtreeFetcher); ok {
		rev, revErr := cond.HeadRevision(ctx, repo, token)
		switch {
		case revErr != nil:
			log.Printf("WARN repository import: head revision probe for %s (%s) failed, reading in full: %v", repo.GetName(), repoID, revErr)
		case !force && rev != "" && r.revisionUnchanged(repoID, rev):
			return nil
		default:
			revision = rev
		}
	}

	fSys, root, err := r.fetcher.Fetch(ctx, repo, token)
	if err != nil {
		return fmt.Errorf("fetch import subtree: %w", err)
	}

	desired, err := renderImportables(fSys, root)
	if err != nil {
		return fmt.Errorf("render import subtree: %w", err)
	}

	var errs []error

	// Track what we intend to own after this pass so prune can remove the rest.
	desiredKeys := make(map[ResourceRef]struct{}, len(desired))
	for _, res := range desired {
		desiredKeys[ResourceRef{Kind: res.Kind, Name: res.Name}] = struct{}{}
		if err := r.writer.Upsert(ctx, repoID, res); err != nil {
			errs = append(errs, fmt.Errorf("upsert %s/%s: %w", res.Kind, res.Name, err))
		}
	}

	owned, err := r.writer.ListOwned(ctx, repoID)
	if err != nil {
		errs = append(errs, fmt.Errorf("list owned resources: %w", err))
		return errors.Join(errs...)
	}
	for _, ref := range owned {
		if _, keep := desiredKeys[ResourceRef{Kind: ref.Kind, Name: ref.Name}]; keep {
			continue
		}
		if err := r.writer.Delete(ctx, repoID, ref); err != nil {
			errs = append(errs, fmt.Errorf("prune %s/%s: %w", ref.Kind, ref.Name, err))
		} else {
			log.Printf("INFO repository import: pruned %s/%s owned by repository %s", ref.Kind, ref.Name, repoID)
		}
	}

	// Record the imported revision only after a fully successful pass, so any failure
	// leaves the cache stale and the next pass reads again rather than skipping.
	if revision != "" && len(errs) == 0 {
		r.recordRevision(repoID, revision)
	}

	return errors.Join(errs...)
}

// lockRepository serializes reconciles of a single Repository. It returns the
// unlock function the caller defers.
func (r *RepositoryReconciler) lockRepository(repoID string) func() {
	r.mu.Lock()
	lk := r.repoLocks[repoID]
	if lk == nil {
		lk = &sync.Mutex{}
		r.repoLocks[repoID] = lk
	}
	r.mu.Unlock()
	lk.Lock()
	return lk.Unlock
}

// revisionUnchanged reports whether rev matches the revision last imported for
// repoID.
func (r *RepositoryReconciler) revisionUnchanged(repoID, rev string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastRevision[repoID] == rev
}

// recordRevision stores the revision imported for repoID.
func (r *RepositoryReconciler) recordRevision(repoID, rev string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRevision[repoID] = rev
}

// forget drops all per-Repository state (cached revision and reconcile lock) for
// repoID when the Repository is deleted, so neither map grows without bound.
func (r *RepositoryReconciler) forget(repoID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lastRevision, repoID)
	delete(r.repoLocks, repoID)
}

// renderImportables renders the kustomize root and returns only the resources
// whose Kind is importable. It rejects Repository (to prevent recursive import)
// and the platform-admin Kinds by simply not including them; a declaration with
// no metadata.name is an error because it cannot be owned or pruned.
func renderImportables(fSys filesys.FileSystem, root string) ([]RenderedResource, error) {
	resMap, err := kustomize.Build(fSys, root)
	if err != nil {
		return nil, err
	}

	var out []RenderedResource
	for _, res := range resMap.Resources() {
		kind := res.GetKind()
		if !isImportableKind(kind) {
			// Repository, ManagedCluster, Role, RoleBinding, and anything else are
			// deliberately ignored by the import.
			log.Printf("INFO repository import: ignoring non-importable kind %s/%s", kind, res.GetName())
			continue
		}
		doc, err := res.Map()
		if err != nil {
			return nil, fmt.Errorf("%s/%s: read rendered resource: %w", kind, res.GetName(), err)
		}
		spec, err := extractSpec(doc)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", kind, res.GetName(), err)
		}
		identity, err := resourceIdentity(kind, res.GetName(), spec)
		if err != nil {
			return nil, err
		}
		out = append(out, RenderedResource{Kind: kind, Name: identity, Spec: spec})
	}

	// Deterministic order keeps logs and tests stable.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// resourceIdentity derives the stable identity used to match a desired resource
// against an already-owned one for upsert and prune. Most Kinds are identified by
// metadata.name. InferenceRoute carries no name on its API surface and is
// immutable (Create/Delete only), so it is identified by its
// (workspace_id, provider_binding_id, model) tuple: changing any field is a new
// identity, which the reconciler realizes as a create of the new plus a prune of
// the old. The writer derives the same identity from the persisted proto message,
// so render-side and API-side identities compare equal.
func resourceIdentity(kind, name string, spec map[string]interface{}) (string, error) {
	if kind == "InferenceRoute" {
		identity := inferenceRouteIdentity(
			specString(spec, "workspace_id"),
			specString(spec, "provider_binding_id"),
			specString(spec, "model"),
		)
		if identity == "||" {
			return "", fmt.Errorf("importable InferenceRoute declaration is missing workspace_id, provider_binding_id, and model")
		}
		return identity, nil
	}
	if name == "" {
		return "", fmt.Errorf("importable %s declaration has no metadata.name", kind)
	}
	return name, nil
}

// inferenceRouteIdentity composes the stable identity of an InferenceRoute.
func inferenceRouteIdentity(workspaceID, providerBindingID, model string) string {
	return workspaceID + "|" + providerBindingID + "|" + model
}

// specString reads a string field from a spec map, returning "" when absent or
// not a string.
func specString(spec map[string]interface{}, key string) string {
	if spec == nil {
		return ""
	}
	v, ok := spec[key].(string)
	if !ok {
		return ""
	}
	return v
}

// extractSpec pulls the spec map out of a rendered resource document. A resource
// with no spec yields an empty (non-nil) map so callers can read fields safely.
func extractSpec(doc map[string]interface{}) (map[string]interface{}, error) {
	raw, ok := doc["spec"]
	if !ok || raw == nil {
		return map[string]interface{}{}, nil
	}
	spec, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("spec is not a mapping")
	}
	return spec, nil
}
