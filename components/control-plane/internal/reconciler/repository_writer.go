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
// repository_id is never trusted). It supports all seven importable Kinds:
// SandboxTemplate, ProviderSpec, ProviderBinding, SecretSource and Gateway (name
// identity, create/update/delete), AgentRuntime (name identity,
// create/update/delete), and InferenceRoute (tuple identity, immutable,
// create/delete). See security/rbac-enforcement.spec.md ("gRPC Authorization").
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
		return w.upsertGateway(ctx, repoID, res)
	case "AgentRuntime":
		return w.upsertAgentRuntime(ctx, repoID, res)
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

func (w *grpcImportWriter) upsertGateway(ctx context.Context, repoID string, res RenderedResource) error {
	owned, err := w.ownedGateways(ctx, repoID)
	if err != nil {
		return err
	}
	clusterID := specString(res.Spec, "cluster_id")
	externalDNS := specStringPtr(res.Spec, "external_dns")
	tlsMode := specStringPtr(res.Spec, "tls_mode")
	serviceType := specStringPtr(res.Spec, "service_type")
	image := specStringPtr(res.Spec, "image")
	supervisorImage := specStringPtr(res.Spec, "supervisor_image")
	oidc := specStringPtr(res.Spec, "oidc")
	route := specStringPtr(res.Spec, "route")
	credentialDriver := specStringPtr(res.Spec, "credential_driver")
	serverDNSNames := specStringSlice(res.Spec, "server_dns_names")

	for _, existing := range owned {
		if existing.GetName() != res.Name {
			continue
		}
		if existing.GetClusterId() == clusterID &&
			stringPtrEqual(existing.ExternalDns, externalDNS) &&
			stringPtrEqual(existing.TlsMode, tlsMode) &&
			stringPtrEqual(existing.ServiceType, serviceType) &&
			stringPtrEqual(existing.Image, image) &&
			stringPtrEqual(existing.SupervisorImage, supervisorImage) &&
			stringPtrEqual(existing.Oidc, oidc) &&
			stringPtrEqual(existing.Route, route) &&
			stringPtrEqual(existing.CredentialDriver, credentialDriver) &&
			stringSliceEqual(existing.GetServerDnsNames(), serverDNSNames) {
			return nil
		}
		// Only send cluster_id when the spec actually declares one. An empty
		// clusterID would be read by the handler as a reassignment to "" and
		// rejected; leaving it nil keeps the gateway on its current cluster while
		// the other changed fields still apply.
		var clusterIDPtr *string
		if clusterID != "" {
			clusterIDPtr = &clusterID
		}
		_, err := w.gateways.UpdateGateway(withOwnership(ctx, repoID), &pb.UpdateGatewayRequest{
			Id:               existing.GetMetadata().GetId(),
			ClusterId:        clusterIDPtr,
			ExternalDns:      externalDNS,
			TlsMode:          tlsMode,
			ServiceType:      serviceType,
			Image:            image,
			SupervisorImage:  supervisorImage,
			Oidc:             oidc,
			Route:            route,
			CredentialDriver: credentialDriver,
			ServerDnsNames:   serverDNSNames,
		})
		return err
	}
	_, err = w.gateways.CreateGateway(withOwnership(ctx, repoID), &pb.CreateGatewayRequest{
		Name:             res.Name,
		ClusterId:        clusterID,
		ExternalDns:      externalDNS,
		TlsMode:          tlsMode,
		ServiceType:      serviceType,
		Image:            image,
		SupervisorImage:  supervisorImage,
		Oidc:             oidc,
		Route:            route,
		CredentialDriver: credentialDriver,
		ServerDnsNames:   serverDNSNames,
	})
	return err
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
