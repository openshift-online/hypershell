package reconciler

import (
	"context"
	"fmt"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// importRepositoryMetadataKey is the gRPC metadata header that carries the id of
// the owning Repository on an import write, so the API server sets and enforces
// repository_id server-side (#585). It MUST stay identical to the canonical
// constant api-server/pkg/importRepositoryMetadataKey; it is duplicated here
// as a single wire-protocol string to keep the api-server HTTP stack
// (gorilla/mux, glog) out of the control-plane dependency graph. The matching test
// asserts the value.
const importRepositoryMetadataKey = "x-hypershell-import-repository-id"

// grpcImportWriter is the production ImportWriter. It upserts and prunes the
// Repository-owned ext resources over the generated gRPC write path, attaching the
// x-hypershell-import-repository-id ownership binding (#585) to every write so the
// API server sets and enforces Repository ownership server-side (the client-side
// repository_id is never trusted). It supports the five import-write ext Kinds
// that have both a repository_id ownership field and a gRPC write surface:
// SandboxTemplate, ProviderSpec, ProviderBinding, SecretSource (name identity,
// create/update/delete), and InferenceRoute (tuple identity, immutable,
// create/delete). See security/rbac-enforcement.spec.md ("gRPC Authorization").
//
// Gateway and AgentRuntime are accepted by the importable-Kinds filter but are not
// writable over the import path yet: Gateway has no repository_id ownership field
// on its message (it cannot be ownership-marked or pruned), and AgentRuntime has
// no gRPC Create/Update/Delete methods at all (only status). Upsert returns
// ErrKindNotWritable for them, which the reconciler collects and surfaces rather
// than silently dropping the declaration.
type grpcImportWriter struct {
	conn             *grpc.ClientConn
	sandboxTemplates pb.SandboxTemplateServiceClient
	providerSpecs    pb.ProviderSpecServiceClient
	providerBindings pb.ProviderBindingServiceClient
	secretSources    pb.SecretSourceServiceClient
	inferenceRoutes  pb.InferenceRouteServiceClient
	pageSize         int32
}

// NewGRPCImportWriter builds the production writer over the shared gRPC connection.
func NewGRPCImportWriter(conn *grpc.ClientConn) ImportWriter {
	return &grpcImportWriter{
		conn:             conn,
		sandboxTemplates: pb.NewSandboxTemplateServiceClient(conn),
		providerSpecs:    pb.NewProviderSpecServiceClient(conn),
		providerBindings: pb.NewProviderBindingServiceClient(conn),
		secretSources:    pb.NewSecretSourceServiceClient(conn),
		inferenceRoutes:  pb.NewInferenceRouteServiceClient(conn),
		pageSize:         100,
	}
}

// withOwnership attaches the import ownership binding so the API server sets
// repository_id server-side on create and authorizes the update/delete as this
// Repository's import.
func withOwnership(ctx context.Context, repoID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, importRepositoryMetadataKey, repoID)
}

func (w *grpcImportWriter) ListOwned(ctx context.Context, repoID string) ([]ResourceRef, error) {
	var refs []ResourceRef

	sts, err := w.ownedSandboxTemplates(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for _, st := range sts {
		refs = append(refs, ResourceRef{Kind: "SandboxTemplate", Name: st.GetName(), ID: st.GetMetadata().GetId()})
	}

	pss, err := w.ownedProviderSpecs(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for _, ps := range pss {
		refs = append(refs, ResourceRef{Kind: "ProviderSpec", Name: ps.GetName(), ID: ps.GetMetadata().GetId()})
	}

	pbs, err := w.ownedProviderBindings(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for _, binding := range pbs {
		refs = append(refs, ResourceRef{Kind: "ProviderBinding", Name: binding.GetName(), ID: binding.GetMetadata().GetId()})
	}

	sss, err := w.ownedSecretSources(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for _, ss := range sss {
		refs = append(refs, ResourceRef{Kind: "SecretSource", Name: ss.GetName(), ID: ss.GetMetadata().GetId()})
	}

	irs, err := w.ownedInferenceRoutes(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for _, ir := range irs {
		refs = append(refs, ResourceRef{
			Kind: "InferenceRoute",
			Name: inferenceRouteIdentity(ir.GetWorkspaceId(), ir.GetProviderBindingId(), ir.GetModel()),
			ID:   ir.GetMetadata().GetId(),
		})
	}

	return refs, nil
}

func (w *grpcImportWriter) Upsert(ctx context.Context, repoID string, res RenderedResource) error {
	switch res.Kind {
	case "SandboxTemplate":
		return w.upsertSandboxTemplate(ctx, repoID, res)
	case "ProviderSpec":
		return w.upsertProviderSpec(ctx, repoID, res)
	case "ProviderBinding":
		return w.upsertProviderBinding(ctx, repoID, res)
	case "SecretSource":
		return w.upsertSecretSource(ctx, repoID, res)
	case "InferenceRoute":
		return w.upsertInferenceRoute(ctx, repoID, res)
	case "Gateway":
		return fmt.Errorf("%s: Gateway has no repository_id ownership field; import write path not available: %w", res.Name, ErrKindNotWritable)
	case "AgentRuntime":
		return fmt.Errorf("%s: AgentRuntime has no gRPC create/update/delete; import write path not available: %w", res.Name, ErrKindNotWritable)
	default:
		return fmt.Errorf("unsupported import kind %q", res.Kind)
	}
}

func (w *grpcImportWriter) Delete(ctx context.Context, repoID string, ref ResourceRef) error {
	ctx = withOwnership(ctx, repoID)
	switch ref.Kind {
	case "SandboxTemplate":
		_, err := w.sandboxTemplates.DeleteSandboxTemplate(ctx, &pb.DeleteSandboxTemplateRequest{Id: ref.ID})
		return err
	case "ProviderSpec":
		_, err := w.providerSpecs.DeleteProviderSpec(ctx, &pb.DeleteProviderSpecRequest{Id: ref.ID})
		return err
	case "ProviderBinding":
		_, err := w.providerBindings.DeleteProviderBinding(ctx, &pb.DeleteProviderBindingRequest{Id: ref.ID})
		return err
	case "SecretSource":
		_, err := w.secretSources.DeleteSecretSource(ctx, &pb.DeleteSecretSourceRequest{Id: ref.ID})
		return err
	case "InferenceRoute":
		_, err := w.inferenceRoutes.DeleteInferenceRoute(ctx, &pb.DeleteInferenceRouteRequest{Id: ref.ID})
		return err
	default:
		return fmt.Errorf("cannot prune unsupported import kind %q", ref.Kind)
	}
}

// ---- SandboxTemplate ----

func (w *grpcImportWriter) ownedSandboxTemplates(ctx context.Context, repoID string) ([]*pb.SandboxTemplate, error) {
	var out []*pb.SandboxTemplate
	for page := int32(1); ; page++ {
		resp, err := w.sandboxTemplates.ListSandboxTemplates(ctx, &pb.ListSandboxTemplatesRequest{Page: page, Size: w.pageSize})
		if err != nil {
			return nil, fmt.Errorf("list sandbox templates: %w", err)
		}
		items := resp.GetItems()
		for _, it := range items {
			if it.GetRepositoryId() == repoID {
				out = append(out, it)
			}
		}
		if int32(len(items)) < w.pageSize {
			return out, nil
		}
	}
}

func (w *grpcImportWriter) upsertSandboxTemplate(ctx context.Context, repoID string, res RenderedResource) error {
	owned, err := w.ownedSandboxTemplates(ctx, repoID)
	if err != nil {
		return err
	}
	image := specString(res.Spec, "image")
	namePrefix := specStringPtr(res.Spec, "name_prefix")
	policy := specStringPtr(res.Spec, "policy")

	for _, existing := range owned {
		if existing.GetName() != res.Name {
			continue
		}
		if existing.GetImage() == image &&
			stringPtrEqual(existing.NamePrefix, namePrefix) &&
			stringPtrEqual(existing.Policy, policy) {
			return nil
		}
		_, err := w.sandboxTemplates.UpdateSandboxTemplate(withOwnership(ctx, repoID), &pb.UpdateSandboxTemplateRequest{
			Id:         existing.GetMetadata().GetId(),
			Image:      &image,
			NamePrefix: namePrefix,
			Policy:     policy,
		})
		return err
	}
	_, err = w.sandboxTemplates.CreateSandboxTemplate(withOwnership(ctx, repoID), &pb.CreateSandboxTemplateRequest{
		Name:       res.Name,
		Image:      image,
		NamePrefix: namePrefix,
		Policy:     policy,
	})
	return err
}

// ---- ProviderSpec ----

func (w *grpcImportWriter) ownedProviderSpecs(ctx context.Context, repoID string) ([]*pb.ProviderSpec, error) {
	var out []*pb.ProviderSpec
	for page := int32(1); ; page++ {
		resp, err := w.providerSpecs.ListProviderSpecs(ctx, &pb.ListProviderSpecsRequest{Page: page, Size: w.pageSize})
		if err != nil {
			return nil, fmt.Errorf("list provider specs: %w", err)
		}
		items := resp.GetItems()
		for _, it := range items {
			if it.GetRepositoryId() == repoID {
				out = append(out, it)
			}
		}
		if int32(len(items)) < w.pageSize {
			return out, nil
		}
	}
}

func (w *grpcImportWriter) upsertProviderSpec(ctx context.Context, repoID string, res RenderedResource) error {
	owned, err := w.ownedProviderSpecs(ctx, repoID)
	if err != nil {
		return err
	}
	category := specString(res.Spec, "category")
	capability := specStringPtr(res.Spec, "capability")
	profile := specStringPtr(res.Spec, "profile")

	for _, existing := range owned {
		if existing.GetName() != res.Name {
			continue
		}
		// category is immutable (no field on UpdateProviderSpecRequest); only
		// capability and profile can change.
		if stringPtrEqual(existing.Capability, capability) && stringPtrEqual(existing.Profile, profile) {
			return nil
		}
		_, err := w.providerSpecs.UpdateProviderSpec(withOwnership(ctx, repoID), &pb.UpdateProviderSpecRequest{
			Id:         existing.GetMetadata().GetId(),
			Capability: capability,
			Profile:    profile,
		})
		return err
	}
	_, err = w.providerSpecs.CreateProviderSpec(withOwnership(ctx, repoID), &pb.CreateProviderSpecRequest{
		Name:       res.Name,
		Category:   category,
		Capability: capability,
		Profile:    profile,
	})
	return err
}

// ---- ProviderBinding ----

func (w *grpcImportWriter) ownedProviderBindings(ctx context.Context, repoID string) ([]*pb.ProviderBinding, error) {
	var out []*pb.ProviderBinding
	for page := int32(1); ; page++ {
		resp, err := w.providerBindings.ListProviderBindings(ctx, &pb.ListProviderBindingsRequest{Page: page, Size: w.pageSize})
		if err != nil {
			return nil, fmt.Errorf("list provider bindings: %w", err)
		}
		items := resp.GetItems()
		for _, it := range items {
			if it.GetRepositoryId() == repoID {
				out = append(out, it)
			}
		}
		if int32(len(items)) < w.pageSize {
			return out, nil
		}
	}
}

func (w *grpcImportWriter) upsertProviderBinding(ctx context.Context, repoID string, res RenderedResource) error {
	owned, err := w.ownedProviderBindings(ctx, repoID)
	if err != nil {
		return err
	}
	workspaceID := specString(res.Spec, "workspace_id")
	providerSpecID := specString(res.Spec, "provider_spec_id")
	secretSourceID := specStringPtr(res.Spec, "secret_source_id")
	refreshStrategy := specStringPtr(res.Spec, "refresh_strategy")

	for _, existing := range owned {
		if existing.GetName() != res.Name {
			continue
		}
		// workspace_id and provider_spec_id are immutable (not on the update
		// request); only secret_source_id and refresh_strategy can change.
		if stringPtrEqual(existing.SecretSourceId, secretSourceID) && stringPtrEqual(existing.RefreshStrategy, refreshStrategy) {
			return nil
		}
		_, err := w.providerBindings.UpdateProviderBinding(withOwnership(ctx, repoID), &pb.UpdateProviderBindingRequest{
			Id:              existing.GetMetadata().GetId(),
			SecretSourceId:  secretSourceID,
			RefreshStrategy: refreshStrategy,
		})
		return err
	}
	_, err = w.providerBindings.CreateProviderBinding(withOwnership(ctx, repoID), &pb.CreateProviderBindingRequest{
		Name:            res.Name,
		WorkspaceId:     workspaceID,
		ProviderSpecId:  providerSpecID,
		SecretSourceId:  secretSourceID,
		RefreshStrategy: refreshStrategy,
	})
	return err
}

// ---- SecretSource ----

func (w *grpcImportWriter) ownedSecretSources(ctx context.Context, repoID string) ([]*pb.SecretSource, error) {
	var out []*pb.SecretSource
	for page := int32(1); ; page++ {
		resp, err := w.secretSources.ListSecretSources(ctx, &pb.ListSecretSourcesRequest{Page: page, Size: w.pageSize})
		if err != nil {
			return nil, fmt.Errorf("list secret sources: %w", err)
		}
		items := resp.GetItems()
		for _, it := range items {
			if it.GetRepositoryId() == repoID {
				out = append(out, it)
			}
		}
		if int32(len(items)) < w.pageSize {
			return out, nil
		}
	}
}

func (w *grpcImportWriter) upsertSecretSource(ctx context.Context, repoID string, res RenderedResource) error {
	owned, err := w.ownedSecretSources(ctx, repoID)
	if err != nil {
		return err
	}
	purpose := specString(res.Spec, "purpose")
	backend := specString(res.Spec, "backend")
	pathValue := specString(res.Spec, "path")
	keyMappings := specStringPtr(res.Spec, "key_mappings")

	for _, existing := range owned {
		if existing.GetName() != res.Name {
			continue
		}
		if existing.GetPurpose() == purpose &&
			existing.GetBackend() == backend &&
			existing.GetPath() == pathValue &&
			stringPtrEqual(existing.KeyMappings, keyMappings) {
			return nil
		}
		_, err := w.secretSources.UpdateSecretSource(withOwnership(ctx, repoID), &pb.UpdateSecretSourceRequest{
			Id:          existing.GetMetadata().GetId(),
			Purpose:     &purpose,
			Backend:     &backend,
			Path:        &pathValue,
			KeyMappings: keyMappings,
		})
		return err
	}
	_, err = w.secretSources.CreateSecretSource(withOwnership(ctx, repoID), &pb.CreateSecretSourceRequest{
		Name:           res.Name,
		AgentRuntimeId: specString(res.Spec, "agent_runtime_id"),
		Purpose:        purpose,
		Backend:        backend,
		Path:           pathValue,
		KeyMappings:    keyMappings,
	})
	return err
}

// ---- InferenceRoute ----

func (w *grpcImportWriter) ownedInferenceRoutes(ctx context.Context, repoID string) ([]*pb.InferenceRoute, error) {
	var out []*pb.InferenceRoute
	for page := int32(1); ; page++ {
		resp, err := w.inferenceRoutes.ListInferenceRoutes(ctx, &pb.ListInferenceRoutesRequest{Page: page, Size: w.pageSize})
		if err != nil {
			return nil, fmt.Errorf("list inference routes: %w", err)
		}
		items := resp.GetItems()
		for _, it := range items {
			if it.GetRepositoryId() == repoID {
				out = append(out, it)
			}
		}
		if int32(len(items)) < w.pageSize {
			return out, nil
		}
	}
}

// upsertInferenceRoute creates the route when its (workspace_id,
// provider_binding_id, model) tuple is not already owned. InferenceRoute is
// immutable (no Update RPC): a field change is a new tuple, realized as a create
// here plus a prune of the old tuple by the reconciler.
func (w *grpcImportWriter) upsertInferenceRoute(ctx context.Context, repoID string, res RenderedResource) error {
	owned, err := w.ownedInferenceRoutes(ctx, repoID)
	if err != nil {
		return err
	}
	workspaceID := specString(res.Spec, "workspace_id")
	providerBindingID := specString(res.Spec, "provider_binding_id")
	model := specString(res.Spec, "model")
	identity := inferenceRouteIdentity(workspaceID, providerBindingID, model)

	for _, existing := range owned {
		if inferenceRouteIdentity(existing.GetWorkspaceId(), existing.GetProviderBindingId(), existing.GetModel()) == identity {
			return nil
		}
	}
	_, err = w.inferenceRoutes.CreateInferenceRoute(withOwnership(ctx, repoID), &pb.CreateInferenceRouteRequest{
		WorkspaceId:       workspaceID,
		ProviderBindingId: providerBindingID,
		Model:             model,
	})
	return err
}

// specStringPtr reads an optional string field, returning nil when absent or
// empty so an omitted field stays unset rather than being written as "".
func specStringPtr(spec map[string]interface{}, key string) *string {
	v := specString(spec, key)
	if v == "" {
		return nil
	}
	return &v
}

// stringPtrEqual compares two optional strings treating nil and empty as equal,
// so a skip decision is stable whether the server returns nil or "".
func stringPtrEqual(a, b *string) bool {
	av, bv := "", ""
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return av == bv
}
