package agentWorkspaces

import (
	"context"
	"regexp"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// workspaceNamePattern restricts an AgentWorkspace name to a lowercase DNS label
// (RFC 1123): the name is provisioned onto the gateway as the workspace identifier, so
// it is format-validated at the API boundary (security.spec.md "Validate All User
// Input") rather than relying on the gateway alone.
var workspaceNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const workspaceNameMaxLen = 63

// agentWorkspaceEventSource is the event broker Source string for AgentWorkspace
// mutations. The gRPC watch handler filters the shared broker fan-out on this
// value, so it MUST match the Source written here on create/update/delete.
const agentWorkspaceEventSource = "AgentWorkspaces"

type AgentWorkspaceService interface {
	Get(ctx context.Context, id string) (*AgentWorkspace, *errors.ServiceError)
	Create(ctx context.Context, agentWorkspace *AgentWorkspace) (*AgentWorkspace, *errors.ServiceError)
	Replace(ctx context.Context, agentWorkspace *AgentWorkspace) (*AgentWorkspace, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (AgentWorkspaceList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (AgentWorkspaceList, *errors.ServiceError)
	// ResolveView returns the AgentRuntime's workspace plus its sub-resources
	// (provider bindings, memberships, inference route) for embedding in the
	// AgentRuntime reconcile payload. It returns nil when the runtime has no
	// workspace yet.
	ResolveView(ctx context.Context, agentRuntimeID string) (*WorkspaceViewData, *errors.ServiceError)
}

// WorkspaceViewData is the resolved workspace plus sub-resources, mapped by the
// plugin's lookup seam into agentRuntimes.WorkspaceView. It carries only references,
// never secret material.
type WorkspaceViewData struct {
	ID               string
	Name             string
	GatewayId        string
	ProviderBindings []ProviderBindingRow
	Memberships      []MembershipRow
	Route            *InferenceRouteRow
}

func NewAgentWorkspaceService(dao AgentWorkspaceDao, events services.EventService) AgentWorkspaceService {
	return &sqlAgentWorkspaceService{dao: dao, events: events}
}

var _ AgentWorkspaceService = &sqlAgentWorkspaceService{}

type sqlAgentWorkspaceService struct {
	dao    AgentWorkspaceDao
	events services.EventService
}

func (s *sqlAgentWorkspaceService) Get(ctx context.Context, id string) (*AgentWorkspace, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("AgentWorkspace", "id", id, err)
	}
	return item, nil
}

func (s *sqlAgentWorkspaceService) Create(ctx context.Context, agentWorkspace *AgentWorkspace) (*AgentWorkspace, *errors.ServiceError) {
	if svcErr := validateWorkspaceNameFormat(agentWorkspace.Name); svcErr != nil {
		return nil, svcErr
	}
	if svcErr := s.validateName(ctx, agentWorkspace); svcErr != nil {
		return nil, svcErr
	}
	if svcErr := s.validateSingleWorkspace(ctx, agentWorkspace); svcErr != nil {
		return nil, svcErr
	}
	item, err := s.dao.Create(ctx, agentWorkspace)
	if err != nil {
		return nil, errors.GeneralError("Unable to create agent workspace: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    agentWorkspaceEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("AgentWorkspace", evErr)
	}
	return item, nil
}

func (s *sqlAgentWorkspaceService) Replace(ctx context.Context, agentWorkspace *AgentWorkspace) (*AgentWorkspace, *errors.ServiceError) {
	if svcErr := validateWorkspaceNameFormat(agentWorkspace.Name); svcErr != nil {
		return nil, svcErr
	}
	if svcErr := s.validateName(ctx, agentWorkspace); svcErr != nil {
		return nil, svcErr
	}
	item, err := s.dao.Replace(ctx, agentWorkspace)
	if err != nil {
		return nil, errors.GeneralError("Unable to update agent workspace: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    agentWorkspaceEventSource,
		SourceID:  item.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("AgentWorkspace", evErr)
	}
	return item, nil
}

func (s *sqlAgentWorkspaceService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete agent workspace: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    agentWorkspaceEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("AgentWorkspace", evErr)
	}
	return nil
}

func (s *sqlAgentWorkspaceService) All(ctx context.Context) (AgentWorkspaceList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all agent workspaces: %s", err)
	}
	return items, nil
}

func (s *sqlAgentWorkspaceService) FindByIDs(ctx context.Context, ids []string) (AgentWorkspaceList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get agent workspaces: %s", err)
	}
	return items, nil
}

func (s *sqlAgentWorkspaceService) ResolveView(ctx context.Context, agentRuntimeID string) (*WorkspaceViewData, *errors.ServiceError) {
	workspaces, err := s.dao.FindByAgentRuntime(ctx, agentRuntimeID)
	if err != nil {
		return nil, errors.GeneralError("Unable to resolve agent workspace: %s", err)
	}
	if len(workspaces) == 0 {
		return nil, nil
	}
	ws := workspaces[0]
	bindings, err := s.dao.ProviderBindingsForWorkspace(ctx, ws.ID)
	if err != nil {
		return nil, errors.GeneralError("Unable to resolve provider bindings: %s", err)
	}
	memberships, err := s.dao.MembershipsForWorkspace(ctx, ws.ID)
	if err != nil {
		return nil, errors.GeneralError("Unable to resolve workspace memberships: %s", err)
	}
	route, err := s.dao.InferenceRouteForWorkspace(ctx, ws.ID)
	if err != nil {
		return nil, errors.GeneralError("Unable to resolve inference route: %s", err)
	}
	return &WorkspaceViewData{
		ID:               ws.ID,
		Name:             ws.Name,
		GatewayId:        ws.GatewayId,
		ProviderBindings: bindings,
		Memberships:      memberships,
		Route:            route,
	}, nil
}

// validateWorkspaceNameFormat rejects a workspace name that is not a lowercase DNS
// label with a 400, before the collision check, because the name is provisioned onto
// the gateway as the workspace identifier.
func validateWorkspaceNameFormat(name string) *errors.ServiceError {
	if len(name) == 0 || len(name) > workspaceNameMaxLen || !workspaceNamePattern.MatchString(name) {
		return errors.Validation("agent workspace name must be a lowercase DNS label (letters, digits, and hyphens, starting and ending alphanumeric, at most %d characters)", workspaceNameMaxLen)
	}
	return nil
}

// validateName enforces the advisory 409 workspace-name collision: a name already
// taken on the same gateway by a DIFFERENT agent_runtime_id is a collision
// (data-model.spec.md "Workspace Name Collision"). Re-declaring the same name under
// the same owner is NOT a collision (the idempotent no-op of
// agent-workspace-gateway-api.spec.md "Idempotent Workspace Provisioning API"). The
// gateway is the authoritative enforcement point; this check reports the error at
// request time.
func (s *sqlAgentWorkspaceService) validateName(ctx context.Context, ws *AgentWorkspace) *errors.ServiceError {
	if ws.GatewayId == "" || ws.Name == "" {
		return nil
	}
	existing, err := s.dao.FindByGatewayName(ctx, ws.GatewayId, ws.Name)
	if err != nil {
		return errors.GeneralError("Unable to validate agent workspace name: %s", err)
	}
	for _, other := range existing {
		if other.ID == ws.ID {
			continue
		}
		if other.AgentRuntimeId != ws.AgentRuntimeId {
			return errors.Conflict("agent workspace name %q is already in use on gateway %s", ws.Name, ws.GatewayId)
		}
	}
	return nil
}

// validateSingleWorkspace enforces the one-workspace-per-AgentRuntime invariant: an
// AgentRuntime owns exactly one AgentWorkspace (data-model.spec.md "AgentRuntime as
// a Top-Level Resource"). It runs only on create.
func (s *sqlAgentWorkspaceService) validateSingleWorkspace(ctx context.Context, ws *AgentWorkspace) *errors.ServiceError {
	if ws.AgentRuntimeId == "" {
		return nil
	}
	existing, err := s.dao.FindByAgentRuntime(ctx, ws.AgentRuntimeId)
	if err != nil {
		return errors.GeneralError("Unable to validate agent workspace ownership: %s", err)
	}
	for _, other := range existing {
		if other.ID != ws.ID {
			return errors.Conflict("agent runtime %s already owns an agent workspace", ws.AgentRuntimeId)
		}
	}
	return nil
}
