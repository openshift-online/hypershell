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
// repository_id is never trusted). It supports all eight importable Kinds:
// SandboxTemplate, ProviderSpec, ProviderBinding, SecretSource and Gateway (name
// identity, create/update/delete), AgentRuntime (name identity,
// create/update/delete), InferenceRoute (tuple identity, immutable, create/delete),
// and CITrust (name identity, create/update/delete; its ci_provider_id and gateway_id
// foreign keys are resolved BY NAME, #871/#873). See
// security/rbac-enforcement.spec.md ("gRPC Authorization").
//
// Gateway and AgentRuntime are cluster-assigned, so their list reads are normally
// held to the caller's own cluster. The writer lists them with the same import
// ownership binding plus the repository_id read filter, which the API server exempts
// from the cluster caller binding (control-plane.spec.md, "import reads") so the
// reconciler can enumerate and prune a Repository's gateways and agent runtimes
// across clusters.
type grpcImportWriter struct {
	conn             *grpc.ClientConn
	sandboxTemplates pb.SandboxTemplateServiceClient
	providerSpecs    pb.ProviderSpecServiceClient
	providerBindings pb.ProviderBindingServiceClient
	secretSources    pb.SecretSourceServiceClient
	inferenceRoutes  pb.InferenceRouteServiceClient
	gateways         pb.GatewayServiceClient
	agentRuntimes    pb.AgentRuntimeServiceClient
	ciTrusts         pb.CITrustServiceClient
	ciProviders      pb.CIProviderServiceClient
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
		gateways:         pb.NewGatewayServiceClient(conn),
		agentRuntimes:    pb.NewAgentRuntimeServiceClient(conn),
		ciTrusts:         pb.NewCITrustServiceClient(conn),
		ciProviders:      pb.NewCIProviderServiceClient(conn),
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

	gws, err := w.ownedGateways(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for _, gw := range gws {
		refs = append(refs, ResourceRef{Kind: "Gateway", Name: gw.GetName(), ID: gw.GetMetadata().GetId()})
	}

	ars, err := w.ownedAgentRuntimes(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for _, ar := range ars {
		refs = append(refs, ResourceRef{Kind: "AgentRuntime", Name: ar.GetName(), ID: ar.GetMetadata().GetId()})
	}

	cts, err := w.ownedCITrusts(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for _, ct := range cts {
		refs = append(refs, ResourceRef{Kind: "CITrust", Name: ct.GetName(), ID: ct.GetMetadata().GetId()})
	}

	return refs, nil
}

// UpsertAll reconciles the whole desired set for one import pass. It fetches the
// reads a CITrust upsert shares - the Repository's owned CITrust and Gateway sets and
// the CIProvider registry - ONCE for the pass and threads them into every CITrust
// upsert, so a pass that imports N CITrusts is O(all) not O(N*all) (FINDING 4). A
// per-resource error is wrapped with the resource's Kind/Name and collected; it does
// not abort the rest of the pass.
func (w *grpcImportWriter) UpsertAll(ctx context.Context, repoID string, desired []RenderedResource) []error {
	// A single shared CITrust cache for the whole pass. It is populated LAZILY on the
	// first CITrust upsert, not here: because the desired set is ordered so Gateways
	// upsert before CITrusts (FINDING 1), loading the owned-Gateway set at first-CITrust
	// time lets a CITrust resolve a Gateway created earlier in the SAME pass, while
	// still reading each shared set only ONCE per pass (FINDING 4).
	citrustCache := &citrustPassCache{}

	var errs []error
	for _, res := range desired {
		if err := w.upsert(ctx, repoID, res, citrustCache); err != nil {
			errs = append(errs, fmt.Errorf("upsert %s/%s: %w", res.Kind, res.Name, err))
		}
	}
	return errs
}

// Upsert reconciles a single resource. It is a convenience entry point (used by
// focused tests) that shares no per-pass cache; the production import drives the
// whole pass through UpsertAll so shared reads happen once.
func (w *grpcImportWriter) Upsert(ctx context.Context, repoID string, res RenderedResource) error {
	return w.upsert(ctx, repoID, res, nil)
}

// upsert dispatches one resource to its per-Kind writer. citrustCache carries the
// per-pass shared reads for the CITrust path; it is nil for the single-resource entry
// point, in which case the CITrust path fetches them itself.
func (w *grpcImportWriter) upsert(ctx context.Context, repoID string, res RenderedResource, citrustCache *citrustPassCache) error {
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
		return w.upsertGateway(ctx, repoID, res)
	case "AgentRuntime":
		return w.upsertAgentRuntime(ctx, repoID, res)
	case "CITrust":
		return w.upsertCITrust(ctx, repoID, res, citrustCache)
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
	case "Gateway":
		_, err := w.gateways.DeleteGateway(ctx, &pb.DeleteGatewayRequest{Id: ref.ID})
		return err
	case "AgentRuntime":
		_, err := w.agentRuntimes.DeleteAgentRuntime(ctx, &pb.DeleteAgentRuntimeRequest{Id: ref.ID})
		return err
	case "CITrust":
		_, err := w.ciTrusts.DeleteCITrust(ctx, &pb.DeleteCITrustRequest{Id: ref.ID})
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

// ---- Gateway ----

// ownedGateways lists the gateways owned by repoID. It sets the repository_id read
// filter so the API server returns only this Repository's gateways and, together
// with the import ownership binding attached by the reconciler, exempts the read
// from the cluster caller binding so it spans clusters (control-plane.spec.md,
// "import reads"). The server-side filter is authoritative; the client-side check is
// a defence-in-depth guard.
func (w *grpcImportWriter) ownedGateways(ctx context.Context, repoID string) ([]*pb.Gateway, error) {
	ctx = withOwnership(ctx, repoID)
	var out []*pb.Gateway
	for page := int32(1); ; page++ {
		resp, err := w.gateways.ListGateways(ctx, &pb.ListGatewaysRequest{Page: page, Size: w.pageSize, RepositoryId: &repoID})
		if err != nil {
			return nil, fmt.Errorf("list gateways: %w", err)
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

// gatewayManifestPlatformFields are the platform-managed Gateway fields a declarative
// .hypershell manifest may NOT declare: the import is a non-privileged writer held to
// the same allowlist as a direct user (name + placement), so a manifest that declares
// one of these fails that gateway's import (control-plane.spec.md CTRL-CORE-15;
// data-model.spec.md PLAT-DATA-23). The server-side field policy (SEC-RBAC-18) is the
// authoritative backstop; this is the friendly early error that names the field.
var gatewayManifestPlatformFields = []string{
	"image", "supervisor_image", "server_dns_names", "oidc", "route",
	"tls_mode", "service_type", "external_dns", "credential_driver",
	"dev_build", "dev_build_metadata",
}

// gatewayManifestForbiddenField returns the name of the first platform-managed field a
// Gateway manifest declares, or "" when it declares only user-writable fields.
func gatewayManifestForbiddenField(spec map[string]interface{}) string {
	for _, field := range gatewayManifestPlatformFields {
		if v, ok := spec[field]; ok && v != nil {
			return field
		}
	}
	return ""
}

func (w *grpcImportWriter) upsertGateway(ctx context.Context, repoID string, res RenderedResource) error {
	// A .hypershell Gateway manifest may declare only name + placement; a platform-managed
	// field fails this gateway's import with a clear error naming the field (CTRL-CORE-15).
	// The error is collected per resource by the caller and does not abort the rest of the
	// import. The server rejects the field even if this check is bypassed (SEC-RBAC-18).
	if field := gatewayManifestForbiddenField(res.Spec); field != "" {
		return fmt.Errorf("gateway %s: field %q is platform-managed and cannot be declared in a .hypershell Gateway manifest", res.Name, field)
	}
	owned, err := w.ownedGateways(ctx, repoID)
	if err != nil {
		return err
	}
	clusterID := specString(res.Spec, "cluster_id")
	placement, err := gatewayPlacement(res.Spec)
	if err != nil {
		return fmt.Errorf("gateway %s: %w", res.Name, err)
	}

	for _, existing := range owned {
		if existing.GetName() != res.Name {
			continue
		}
		// The gateway is already imported. An import's writable surface is name + placement
		// (create-only); name is the match key here and placement cannot be mutated after
		// create, so there is nothing an import may update (PLAT-DATA-23). The platform
		// fields the control plane owns are left untouched. The re-import is a no-op.
		return nil
	}
	// On create, supply the user-writable allowlist only: name and exactly one of
	// cluster_id (pinned) or placement (late-bound), matching the api-server's
	// exactly-one-of contract (#625, data-model.spec.md). No platform-managed field is
	// sent; the control plane authors those itself (PLAT-DATA-23, CTRL-CORE-15).
	_, err = w.gateways.CreateGateway(withOwnership(ctx, repoID), &pb.CreateGatewayRequest{
		Name:      res.Name,
		ClusterId: clusterID,
		Placement: placement,
	})
	return err
}

// gatewayPlacement builds the gRPC GatewayPlacementIntent from a rendered Gateway
// spec's `placement` block, mapping the declarative late-binding manifest onto the
// api-server create contract (data-model.spec.md, "Gateway Placement and Late
// Binding"; gateway-provision-placement.spec.md WEB-PLC-01). A non-empty
// `selector` list maps to CapabilitySelectorPlacement. It returns (nil, nil) when
// the spec declares no placement at all (a pinned gateway). When a `placement`
// block IS present but declares no non-empty selector list, it returns an error
// naming the problem, so a malformed late-bound manifest fails with a clear cause
// rather than silently rendering a create with neither cluster_id nor placement.
func gatewayPlacement(spec map[string]interface{}) (*pb.GatewayPlacementIntent, error) {
	if spec == nil {
		return nil, nil
	}
	raw, ok := spec["placement"]
	if !ok || raw == nil {
		return nil, nil
	}
	placementMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("placement must be a mapping with a selector list")
	}
	if selector := specStringSlice(placementMap, "selector"); len(selector) > 0 {
		return &pb.GatewayPlacementIntent{
			Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
				CapabilitySelector: &pb.CapabilitySelectorPlacement{Selector: selector},
			},
		}, nil
	}
	return nil, fmt.Errorf("placement declares no non-empty selector list")
}

// ---- AgentRuntime ----

func (w *grpcImportWriter) ownedAgentRuntimes(ctx context.Context, repoID string) ([]*pb.AgentRuntime, error) {
	ctx = withOwnership(ctx, repoID)
	resp, err := w.agentRuntimes.ListAgentRuntimes(ctx, &pb.ListAgentRuntimesRequest{RepositoryId: &repoID})
	if err != nil {
		return nil, fmt.Errorf("list agent runtimes: %w", err)
	}
	var out []*pb.AgentRuntime
	for _, it := range resp.GetItems() {
		if it.GetRepositoryId() == repoID {
			out = append(out, it)
		}
	}
	return out, nil
}

func (w *grpcImportWriter) upsertAgentRuntime(ctx context.Context, repoID string, res RenderedResource) error {
	owned, err := w.ownedAgentRuntimes(ctx, repoID)
	if err != nil {
		return err
	}
	gatewayID := specString(res.Spec, "gateway_id")
	sandboxTemplateID := specString(res.Spec, "sandbox_template_id")
	clusterID := specStringPtr(res.Spec, "cluster_id")
	selector := specStringSlice(res.Spec, "selector")
	description := specStringPtr(res.Spec, "description")
	cron := specStringPtr(res.Spec, "cron")
	coordinatorImage := specStringPtr(res.Spec, "coordinator_image")
	concurrencyPolicy := specStringPtr(res.Spec, "concurrency_policy")
	loginRefreshSeconds := specInt32Ptr(res.Spec, "login_refresh_seconds")
	parameters := specStringPtr(res.Spec, "parameters")

	for _, existing := range owned {
		if existing.GetName() != res.Name {
			continue
		}
		if existing.GetGatewayId() == gatewayID &&
			existing.GetSandboxTemplateId() == sandboxTemplateID &&
			stringPtrEqual(stringToPtr(existing.GetClusterId()), clusterID) &&
			stringSliceEqual(existing.GetSelector(), selector) &&
			stringPtrEqual(existing.Description, description) &&
			stringPtrEqual(existing.Cron, cron) &&
			stringPtrEqual(existing.CoordinatorImage, coordinatorImage) &&
			stringPtrEqual(existing.ConcurrencyPolicy, concurrencyPolicy) &&
			int32PtrEqual(existing.LoginRefreshSeconds, loginRefreshSeconds) &&
			stringPtrEqual(existing.Parameters, parameters) {
			return nil
		}
		_, err := w.agentRuntimes.UpdateAgentRuntime(withOwnership(ctx, repoID), &pb.UpdateAgentRuntimeRequest{
			Id:                  existing.GetMetadata().GetId(),
			GatewayId:           &gatewayID,
			SandboxTemplateId:   &sandboxTemplateID,
			ClusterId:           clusterID,
			Selector:            selector,
			Description:         description,
			Cron:                cron,
			CoordinatorImage:    coordinatorImage,
			ConcurrencyPolicy:   concurrencyPolicy,
			LoginRefreshSeconds: loginRefreshSeconds,
			Parameters:          parameters,
		})
		return err
	}
	_, err = w.agentRuntimes.CreateAgentRuntime(withOwnership(ctx, repoID), &pb.CreateAgentRuntimeRequest{
		Name:                res.Name,
		GatewayId:           gatewayID,
		SandboxTemplateId:   sandboxTemplateID,
		ClusterId:           clusterID,
		Selector:            selector,
		Description:         description,
		Cron:                cron,
		CoordinatorImage:    coordinatorImage,
		ConcurrencyPolicy:   concurrencyPolicy,
		LoginRefreshSeconds: loginRefreshSeconds,
		Parameters:          parameters,
	})
	return err
}

// ---- CITrust ----

func (w *grpcImportWriter) ownedCITrusts(ctx context.Context, repoID string) ([]*pb.CITrust, error) {
	var out []*pb.CITrust
	for page := int32(1); ; page++ {
		resp, err := w.ciTrusts.ListCITrusts(ctx, &pb.ListCITrustsRequest{Page: page, Size: w.pageSize})
		if err != nil {
			return nil, fmt.Errorf("list CI trusts: %w", err)
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

// citrustPassCache holds the reads a CITrust upsert shares across one import pass:
// the Repository's owned CITrust set (for name-identity match and drift comparison),
// its owned Gateway set (for gateway_id by-name resolution scoped to the Repository),
// and the platform-wide CIProvider registry (for ci_provider_id by-name resolution).
// It is read ONCE per pass (lazily, on the first CITrust upsert) and reused by every
// later CITrust, so a pass is O(all) not O(N*all) (FINDING 4). err records a read
// failure so each CITrust upsert fails with the same cause rather than proceeding on
// partial reads.
type citrustPassCache struct {
	loaded        bool
	ownedCITrusts []*pb.CITrust
	ownedGateways []*pb.Gateway
	ciProviders   []*pb.CIProvider
	err           error
}

// ensureCITrustPassCache populates the shared cache on first use. Loading lazily at
// first-CITrust time (rather than at pass start) is what lets a CITrust resolve a
// Gateway created earlier in the SAME ordered pass (FINDING 1), while the loaded guard
// keeps it to a single read per pass (FINDING 4). A pass runs single-threaded, so no
// synchronization is needed.
func (w *grpcImportWriter) ensureCITrustPassCache(ctx context.Context, repoID string, cache *citrustPassCache) {
	if cache.loaded {
		return
	}
	cache.loaded = true
	cache.ownedCITrusts, cache.err = w.ownedCITrusts(ctx, repoID)
	if cache.err != nil {
		return
	}
	cache.ownedGateways, cache.err = w.ownedGateways(ctx, repoID)
	if cache.err != nil {
		return
	}
	cache.ciProviders, cache.err = w.listCIProviders(ctx)
}

// listCIProviders pages through the platform-wide CIProvider registry. CIProvider is
// not Repository-owned, so the list spans every registered provider.
func (w *grpcImportWriter) listCIProviders(ctx context.Context) ([]*pb.CIProvider, error) {
	var out []*pb.CIProvider
	for page := int32(1); ; page++ {
		resp, err := w.ciProviders.ListCIProviders(ctx, &pb.ListCIProvidersRequest{Page: page, Size: w.pageSize})
		if err != nil {
			return nil, fmt.Errorf("list CI providers: %w", err)
		}
		items := resp.GetItems()
		out = append(out, items...)
		if int32(len(items)) < w.pageSize {
			return out, nil
		}
	}
}

// resolveCIProviderIDByName resolves a CIProvider name to its id (#871/#873
// find-by-name) over the pre-fetched registry. CIProvider is a platform-wide
// registration (not importable), so the lookup spans every CIProvider rather than the
// Repository's owned set. It returns a clear error when the name matches no CIProvider
// so a bad manifest fails that CITrust's import with a cause rather than a silent FK
// violation.
func resolveCIProviderIDByName(providers []*pb.CIProvider, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("ci_provider_id is required")
	}
	for _, it := range providers {
		if it.GetName() == name {
			return it.GetMetadata().GetId(), nil
		}
	}
	return "", fmt.Errorf("ci_provider_id %q does not resolve to a registered CIProvider", name)
}

// resolveOwnedGatewayIDByName resolves a Gateway name to its id over the pre-fetched
// set of gateways OWNED by this Repository. Restricting resolution to the owned set
// both resolves the FK by name (#871/#873) and enforces the imported-CITrust tenancy
// of SEC-RBAC-25 at the writer: a manifest referencing a gateway the Repository does
// not own fails to resolve, so the control plane never attempts a cross-tenant CITrust
// write. The api-server enforces the same gate server-side (the authoritative
// backstop); this is the friendly early error.
func resolveOwnedGatewayIDByName(ownedGateways []*pb.Gateway, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("gateway_id is required")
	}
	for _, gw := range ownedGateways {
		if gw.GetName() == name {
			return gw.GetMetadata().GetId(), nil
		}
	}
	return "", fmt.Errorf("gateway_id %q does not resolve to a Gateway owned by this repository", name)
}

// upsertCITrust creates or updates a Repository-owned CITrust. name identity (the
// manifest's metadata.name) matches a desired trust against an owned one. Only the
// mutable fields are compared and updated - role and the full subject predicate
// (subject_repository, subject_project_path, subject_ref, subject_environment,
// subject_ref_protected); ci_provider_id and gateway_id are immutable after create. A
// changed subject.repository or subject.project_path therefore reconciles via update
// rather than being silently skipped (reconcile, don't create-or-skip; FINDING 2).
//
// The ci_provider_id and gateway_id foreign keys are declared BY NAME in the manifest
// and resolved to ids ONLY on the create branch (FINDING 3): on update they are
// immutable and unused, so resolving them eagerly would let a since-renamed
// CIProvider or Gateway fail an update that touches neither. cache carries the shared
// per-pass reads; when nil (the single-resource entry point) they are fetched here.
func (w *grpcImportWriter) upsertCITrust(ctx context.Context, repoID string, res RenderedResource, cache *citrustPassCache) error {
	if cache == nil {
		cache = &citrustPassCache{}
	}
	w.ensureCITrustPassCache(ctx, repoID, cache)
	if cache.err != nil {
		return cache.err
	}

	role := specString(res.Spec, "role")
	subject := specMap(res.Spec, "subject")
	subjectRepository := specStringPtr(subject, "repository")
	subjectProjectPath := specStringPtr(subject, "project_path")
	subjectRef := specStringPtr(subject, "ref")
	subjectEnvironment := specStringPtr(subject, "environment")
	subjectRefProtected := specBoolPtr(subject, "ref_protected")

	for _, existing := range cache.ownedCITrusts {
		if existing.GetName() != res.Name {
			continue
		}
		// ci_provider_id and gateway_id are immutable (not on the update request); only
		// role and the subject predicate can change. subject_repository and
		// subject_project_path are part of that comparison so a changed value is
		// reconciled, not skipped (FINDING 2).
		if existing.GetRole() == role &&
			stringPtrEqual(existing.SubjectRepository, subjectRepository) &&
			stringPtrEqual(existing.SubjectProjectPath, subjectProjectPath) &&
			stringPtrEqual(existing.SubjectRef, subjectRef) &&
			stringPtrEqual(existing.SubjectEnvironment, subjectEnvironment) &&
			boolPtrEqual(existing.SubjectRefProtected, subjectRefProtected) {
			return nil
		}
		_, err := w.ciTrusts.UpdateCITrust(withOwnership(ctx, repoID), &pb.UpdateCITrustRequest{
			Id:                  existing.GetMetadata().GetId(),
			Role:                &role,
			SubjectRepository:   subjectRepository,
			SubjectProjectPath:  subjectProjectPath,
			SubjectRef:          subjectRef,
			SubjectEnvironment:  subjectEnvironment,
			SubjectRefProtected: subjectRefProtected,
		})
		return err
	}

	// Create branch: resolve the immutable foreign keys by name only now that the
	// exists check has determined this is a create (FINDING 3).
	ciProviderID, err := resolveCIProviderIDByName(cache.ciProviders, specString(res.Spec, "ci_provider_id"))
	if err != nil {
		return fmt.Errorf("CI trust %s: %w", res.Name, err)
	}
	gatewayID, err := resolveOwnedGatewayIDByName(cache.ownedGateways, specString(res.Spec, "gateway_id"))
	if err != nil {
		return fmt.Errorf("CI trust %s: %w", res.Name, err)
	}
	name := res.Name
	_, err = w.ciTrusts.CreateCITrust(withOwnership(ctx, repoID), &pb.CreateCITrustRequest{
		Name:                &name,
		CiProviderId:        ciProviderID,
		GatewayId:           gatewayID,
		Role:                role,
		SubjectRepository:   subjectRepository,
		SubjectProjectPath:  subjectProjectPath,
		SubjectRef:          subjectRef,
		SubjectEnvironment:  subjectEnvironment,
		SubjectRefProtected: subjectRefProtected,
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

// stringToPtr wraps a value so a scalar proto field (returned as "") can be compared
// against an optional spec value through stringPtrEqual.
func stringToPtr(s string) *string { return &s }

// specMap reads a nested mapping field (for example a CITrust's `subject` block),
// returning an empty (non-nil) map when absent so callers can read sub-fields safely.
func specMap(spec map[string]interface{}, key string) map[string]interface{} {
	if spec == nil {
		return map[string]interface{}{}
	}
	raw, ok := spec[key]
	if !ok || raw == nil {
		return map[string]interface{}{}
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return m
}

// specBoolPtr reads an optional boolean field as *bool, returning nil when absent so
// an omitted field stays unset.
func specBoolPtr(spec map[string]interface{}, key string) *bool {
	if spec == nil {
		return nil
	}
	raw, ok := spec[key]
	if !ok || raw == nil {
		return nil
	}
	v, ok := raw.(bool)
	if !ok {
		return nil
	}
	return &v
}

// boolPtrEqual compares two optional bools treating nil and a pointer to false as
// equal, so a skip decision is stable whether the server returns nil or false.
func boolPtrEqual(a, b *bool) bool {
	var av, bv bool
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return av == bv
}

// specStringSlice reads a repeated string field, returning nil when absent so an
// omitted field compares equal to an empty server value.
func specStringSlice(spec map[string]interface{}, key string) []string {
	if spec == nil {
		return nil
	}
	raw, ok := spec[key]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stringSliceEqual reports whether two string slices carry the same ordered values,
// treating nil and empty as equal.
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// specInt32Ptr reads an optional integer field as *int32, returning nil when absent.
// YAML/JSON decoding may surface the value as int, int64, or float64, so all three
// are accepted.
func specInt32Ptr(spec map[string]interface{}, key string) *int32 {
	if spec == nil {
		return nil
	}
	raw, ok := spec[key]
	if !ok || raw == nil {
		return nil
	}
	var v int32
	switch n := raw.(type) {
	case int:
		v = int32(n)
	case int32:
		v = n
	case int64:
		v = int32(n)
	case float64:
		v = int32(n)
	default:
		return nil
	}
	return &v
}

// int32PtrEqual compares two optional int32s treating nil and a pointer to 0 as
// equal, so a skip decision is stable whether the server returns nil or 0.
func int32PtrEqual(a, b *int32) bool {
	var av, bv int32
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return av == bv
}
