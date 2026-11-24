package platformGatewayPolicies

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// GatewayLookup reports whether a live gateway with the id exists.
type GatewayLookup func(ctx context.Context, gatewayID string) (bool, *errors.ServiceError)

// maxEffectiveFindings bounds the findings returned with a gateway's effective
// policies; the full history is paged through the per-policy findings read.
const maxEffectiveFindings = 100

type handler struct {
	service    PolicyService
	generic    services.GenericService
	gateways   GatewayLookup
	visibility targetVisibility
	now        func() time.Time
}

func newHandler(service PolicyService, generic services.GenericService, gateways GatewayLookup, visibility targetVisibility) *handler {
	return &handler{service: service, generic: generic, gateways: gateways, visibility: visibility, now: time.Now}
}

// policyContext returns the global mode and the accepted fleet defaults every
// policy read is evaluated against (PLAT-PGP-06, PLAT-PGP-08).
func (h *handler) policyContext(ctx context.Context) (string, FleetDefaults, *errors.ServiceError) {
	st, svcErr := h.service.Settings(ctx)
	if svcErr != nil {
		return "", FleetDefaults{}, svcErr
	}
	defaults, svcErr := h.service.AcceptedDefaults(ctx)
	if svcErr != nil {
		return "", FleetDefaults{}, svcErr
	}
	return st.Mode, defaults, nil
}

// view builds the read-time facts for one policy. The policy endpoints are
// reachable only by callers who may see targets (the ext gate), so targets and
// rollouts are always included here.
func (h *handler) view(ctx context.Context, p *PlatformGatewayPolicy, globalMode string, defaults FleetDefaults) (policyView, *errors.ServiceError) {
	ve, svcErr := h.service.ValidateCedar(p.Cedar, defaults.Normalized)
	if svcErr != nil {
		return policyView{}, svcErr
	}
	since := h.now().Add(-24 * time.Hour)
	counts, svcErr := h.service.CountFindings(ctx, FindingQuery{PolicyName: p.Name, Since: since})
	if svcErr != nil {
		return policyView{}, svcErr
	}
	perGateway, svcErr := h.service.CountFindingsByGateway(ctx, p.Name, since)
	if svcErr != nil {
		return policyView{}, svcErr
	}
	return policyView{
		globalMode:      globalMode,
		validation:      ve,
		counts24h:       counts,
		gatewayCounts:   perGateway,
		includeTarget:   true,
		includeRollouts: true,
	}, nil
}

func (h *handler) present(ctx context.Context, p *PlatformGatewayPolicy) (interface{}, *errors.ServiceError) {
	mode, defaults, svcErr := h.policyContext(ctx)
	if svcErr != nil {
		return nil, svcErr
	}
	v, svcErr := h.view(ctx, p, mode, defaults)
	if svcErr != nil {
		return nil, svcErr
	}
	return PresentPolicy(p, v), nil
}

func (h *handler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			var items []PlatformGatewayPolicy
			paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
			if svcErr != nil {
				return nil, svcErr
			}
			mode, defaults, svcErr := h.policyContext(ctx)
			if svcErr != nil {
				return nil, svcErr
			}
			kind := "PlatformGatewayPolicyList"
			list := openapi.PlatformGatewayPolicyList{
				Kind:  &kind,
				Page:  openapi.PtrInt32(int32(paging.Page)),
				Size:  openapi.PtrInt32(int32(paging.Size)),
				Total: openapi.PtrInt32(int32(paging.Total)),
				Items: []openapi.PlatformGatewayPolicy{},
			}
			for i := range items {
				v, svcErr := h.view(ctx, &items[i], mode, defaults)
				if svcErr != nil {
					return nil, svcErr
				}
				list.Items = append(list.Items, PresentPolicy(&items[i], v))
			}
			if listArgs.Fields != nil {
				filtered, err := presenters.SliceFilter(listArgs.Fields, list.Items)
				if err != nil {
					return nil, err
				}
				return filtered, nil
			}
			return list, nil
		},
	}
	handlers.HandleList(w, r, cfg)
}

func (h *handler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			item, svcErr := h.service.Get(r.Context(), mux.Vars(r)["id"])
			if svcErr != nil {
				return nil, svcErr
			}
			return h.present(r.Context(), item)
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *handler) Create(w http.ResponseWriter, r *http.Request) {
	var body openapi.PlatformGatewayPolicy
	cfg := &handlers.HandlerConfig{
		Body: &body,
		Action: func() (interface{}, *errors.ServiceError) {
			target, svcErr := encodeTarget(convertTarget(body.Target))
			if svcErr != nil {
				return nil, svcErr
			}
			item := &PlatformGatewayPolicy{
				Name:             strings.TrimSpace(body.Name),
				Description:      deref(body.Description),
				Target:           target,
				UserMessage:      deref(body.UserMessage),
				DocumentationUrl: strings.TrimSpace(deref(body.DocumentationUrl)),
				Cedar:            body.Cedar,
			}
			created, svcErr := h.service.Create(r.Context(), item)
			if svcErr != nil {
				return nil, svcErr
			}
			return h.present(r.Context(), created)
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *handler) Patch(w http.ResponseWriter, r *http.Request) {
	var body openapi.PlatformGatewayPolicyPatchRequest
	cfg := &handlers.HandlerConfig{
		Body: &body,
		Action: func() (interface{}, *errors.ServiceError) {
			patch := PolicyPatch{
				Description:      body.Description,
				EnforcementMode:  body.EnforcementMode,
				UserMessage:      body.UserMessage,
				DocumentationUrl: body.DocumentationUrl,
				Cedar:            body.Cedar,
			}
			if body.Target != nil {
				t := convertTarget(body.Target)
				patch.Target = &t
			}
			updated, svcErr := h.service.Update(r.Context(), mux.Vars(r)["id"], patch)
			if svcErr != nil {
				return nil, svcErr
			}
			return h.present(r.Context(), updated)
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h *handler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			return nil, h.service.Delete(r.Context(), mux.Vars(r)["id"])
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}

const forbidsDefaultsPrefix = "policy forbids the default sandbox request"

// Evaluate is the editor dry run (WEB-PADM-05/08/09): validation diagnostics,
// rendered rules, and the sample request decided with enforcement semantics.
func (h *handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	var body openapi.PlatformGatewayPolicyEvaluationRequest
	cfg := &handlers.HandlerConfig{
		Body: &body,
		Action: func() (interface{}, *errors.ServiceError) {
			defaults, svcErr := h.service.AcceptedDefaults(r.Context())
			if svcErr != nil {
				return nil, svcErr
			}
			ve, svcErr := h.service.ValidateCedar(body.Cedar, defaults.Normalized)
			if svcErr != nil {
				return nil, svcErr
			}
			out := openapi.PlatformGatewayPolicyEvaluation{
				Valid:           openapi.PtrBool(ve == nil),
				ForbidsDefaults: openapi.PtrBool(false),
				Diagnostics:     presentDiagnostics(ve),
				Rendered:        presentRendered(body.Cedar),
				Defaults:        PresentFleetDefaults(defaults),
			}
			if ve != nil {
				for _, d := range ve.Details {
					if strings.HasPrefix(d.Message, forbidsDefaultsPrefix) {
						out.ForbidsDefaults = openapi.PtrBool(true)
					}
				}
			}
			name := strings.TrimSpace(deref(body.Name))
			if name == "" {
				name = "draft-policy"
			}
			evaluator, err := cedar.NewEvaluator([]cedar.PolicyInput{{Name: name, Cedar: body.Cedar}})
			if err != nil {
				// The draft does not compile into an evaluable forbid set; the
				// diagnostics already explain why, so no sample result is given.
				return out, nil
			}
			req, svcErr := sampleRequest(body.Sample, defaults.Normalized)
			if svcErr != nil {
				return nil, svcErr
			}
			decision := evaluator.Evaluate(req)
			out.SampleAllowed = openapi.PtrBool(decision.Allowed)
			for _, m := range decision.Matches {
				out.RejectingStatements = append(out.RejectingStatements, int32(m.StatementIndex))
			}
			if !decision.Allowed {
				out.DenialText = openapi.PtrString(cedar.DenialReason(decision, map[string]cedar.PolicyMessage{
					name: {UserMessage: deref(body.UserMessage), DocumentationURL: deref(body.DocumentationUrl)},
				}, ""))
			}
			return out, nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

// sampleRequest builds the Cedar request for a sample; omitted resources take
// the fleet defaults, as the interceptor applies them (WEB-PADM-08).
func sampleRequest(s *openapi.PolicySampleRequest, d cedar.Defaults) (cedar.Request, *errors.ServiceError) {
	ctx := d.DefaultContext()
	req := cedar.Request{PrincipalSubject: "sample-user", GatewayName: "sample-gateway"}
	if s == nil {
		req.Context = ctx
		return req, nil
	}
	parse := func(raw *string, target *int64, fn func(string) (int64, error), field string) *errors.ServiceError {
		if raw == nil || strings.TrimSpace(*raw) == "" {
			return nil
		}
		v, err := fn(strings.TrimSpace(*raw))
		if err != nil {
			return errors.Validation("sample %s: %s", field, err)
		}
		*target = v
		return nil
	}
	if e := parse(s.CpuRequest, &ctx.CPURequestMillis, cedar.ParseMillis, "cpu_request"); e != nil {
		return req, e
	}
	if e := parse(s.CpuLimit, &ctx.CPULimitMillis, cedar.ParseMillis, "cpu_limit"); e != nil {
		return req, e
	}
	if e := parse(s.MemoryRequest, &ctx.MemoryRequestBytes, cedar.ParseBytes, "memory_request"); e != nil {
		return req, e
	}
	if e := parse(s.MemoryLimit, &ctx.MemoryLimitBytes, cedar.ParseBytes, "memory_limit"); e != nil {
		return req, e
	}
	if s.GpuCount != nil {
		ctx.GPUCount = *s.GpuCount
	}
	ctx.Image = deref(s.Image)
	ctx.SandboxName = deref(s.SandboxName)
	req.Context = ctx
	req.PrincipalRoles = s.Roles
	return req, nil
}

// Findings lists the audit findings that name one policy (WEB-PADM-04).
func (h *handler) Findings(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			p, svcErr := h.service.Get(r.Context(), mux.Vars(r)["id"])
			if svcErr != nil {
				return nil, svcErr
			}
			q := r.URL.Query()
			page, size := pageArgs(q.Get("page"), q.Get("size"))
			outcome := q.Get("outcome")
			if outcome != "" && outcome != OutcomeWouldReject && outcome != OutcomeWouldAddDefaults {
				return nil, errors.Validation("outcome must be would_reject or would_add_defaults")
			}
			items, total, svcErr := h.service.ListFindings(r.Context(), FindingQuery{
				PolicyName: p.Name,
				GatewayID:  q.Get("gateway_id"),
				Outcome:    outcome,
				Limit:      size,
				Offset:     (page - 1) * size,
			})
			if svcErr != nil {
				return nil, svcErr
			}
			return presentFindingList(items, page, size, total), nil
		},
	}
	handlers.HandleList(w, r, cfg)
}

func presentFindingList(items []Finding, page, size int, total int64) openapi.PolicyAuditFindingList {
	kind := "PolicyAuditFindingList"
	out := openapi.PolicyAuditFindingList{
		Kind:  &kind,
		Page:  openapi.PtrInt32(int32(page)),
		Size:  openapi.PtrInt32(int32(len(items))),
		Total: openapi.PtrInt32(int32(total)),
		Items: []openapi.PolicyAuditFinding{},
	}
	for _, f := range items {
		out.Items = append(out.Items, PresentFinding(f))
	}
	return out
}

func pageArgs(rawPage, rawSize string) (int, int) {
	page, size := 1, 100
	if n, err := strconv.Atoi(rawPage); err == nil && n > 0 {
		page = n
	}
	if n, err := strconv.Atoi(rawSize); err == nil && n > 0 {
		size = n
	}
	if size > 500 {
		size = 500
	}
	return page, size
}

func (h *handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			st, svcErr := h.service.Settings(r.Context())
			if svcErr != nil {
				return nil, svcErr
			}
			return PresentSettings(st)
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *handler) PatchSettings(w http.ResponseWriter, r *http.Request) {
	var body openapi.PlatformPolicySettingsPatchRequest
	cfg := &handlers.HandlerConfig{
		Body: &body,
		Action: func() (interface{}, *errors.ServiceError) {
			actor := ""
			if payload, err := auth.GetAuthPayload(r); err == nil && payload != nil {
				actor = payload.Username
			}
			st, svcErr := h.service.SetMode(r.Context(), body.Mode, actor)
			if svcErr != nil {
				return nil, svcErr
			}
			return PresentSettings(st)
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

// EffectivePolicies is the gateway read surface (PLAT-PGP-19, SEC-RBAC-31):
// the policies that apply to the gateway, the fleet defaults, the global mode,
// and the gateway's own findings. Targets are included only for callers who
// may see them.
func (h *handler) EffectivePolicies(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			gatewayID := mux.Vars(r)["gateway_id"]
			exists, svcErr := h.gateways(ctx, gatewayID)
			if svcErr != nil {
				return nil, svcErr
			}
			if !exists {
				return nil, errors.NotFound("Gateway with id='%s' not found", gatewayID)
			}
			return h.effective(ctx, gatewayID)
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *handler) effective(ctx context.Context, gatewayID string) (openapi.GatewayEffectivePolicies, *errors.ServiceError) {
	mode, defaults, svcErr := h.policyContext(ctx)
	if svcErr != nil {
		return openapi.GatewayEffectivePolicies{}, svcErr
	}
	seeTargets, svcErr := h.visibility.canSeeTargets(ctx)
	if svcErr != nil {
		return openapi.GatewayEffectivePolicies{}, svcErr
	}
	policies, svcErr := h.service.All(ctx)
	if svcErr != nil {
		return openapi.GatewayEffectivePolicies{}, svcErr
	}
	out := openapi.GatewayEffectivePolicies{
		GatewayId:     openapi.PtrString(gatewayID),
		GlobalMode:    openapi.PtrString(mode),
		SchemaVersion: openapi.PtrString(cedar.SchemaVersion),
		Defaults:      PresentFleetDefaults(defaults),
		Policies:      []openapi.EffectivePolicy{},
		Findings:      []openapi.PolicyAuditFinding{},
	}
	for _, p := range policies {
		target, err := DecodeTarget(p.Target)
		if err != nil {
			return openapi.GatewayEffectivePolicies{}, errors.GeneralError("%s", err)
		}
		if !target.Applies(gatewayID) {
			continue
		}
		out.Policies = append(out.Policies, presentEffective(p, target, mode, gatewayID, seeTargets))
	}
	findings, _, svcErr := h.service.ListFindings(ctx, FindingQuery{GatewayID: gatewayID, Limit: maxEffectiveFindings})
	if svcErr != nil {
		return openapi.GatewayEffectivePolicies{}, svcErr
	}
	for _, f := range findings {
		out.Findings = append(out.Findings, PresentFinding(f))
	}
	counts, svcErr := h.service.CountFindings(ctx, FindingQuery{GatewayID: gatewayID, Since: h.now().Add(-24 * time.Hour)})
	if svcErr != nil {
		return openapi.GatewayEffectivePolicies{}, svcErr
	}
	out.FindingsLast24h = presentCounts(counts)
	return out, nil
}

func presentEffective(p *PlatformGatewayPolicy, target Target, globalMode, gatewayID string, seeTargets bool) openapi.EffectivePolicy {
	effective := EffectiveMode(globalMode, p.EnforcementMode)
	reason := "audit"
	switch {
	case p.EnforcementMode == ModeEnforce:
		reason = "policy_enforced"
	case globalMode == ModeEnforce:
		reason = "platform_enforced"
	}
	out := openapi.EffectivePolicy{
		Id:                  openapi.PtrString(p.ID),
		Name:                openapi.PtrString(p.Name),
		UserMessage:         openapi.PtrString(p.UserMessage),
		DocumentationUrl:    openapi.PtrString(p.DocumentationUrl),
		EnforcementMode:     openapi.PtrString(p.EnforcementMode),
		EffectiveMode:       openapi.PtrString(effective),
		EffectiveModeReason: openapi.PtrString(reason),
		BuiltIn:             openapi.PtrBool(p.BuiltIn),
		Cedar:               openapi.PtrString(p.Cedar),
		Rendered:            presentRendered(p.Cedar),
		Condition:           openapi.PtrString(ConditionPending),
		UpdatedAt:           openapi.PtrTime(p.UpdatedAt),
	}
	if st, err := DecodeStatus(p.Status); err == nil {
		if g, ok := st.Gateways[gatewayID]; ok {
			out.Condition = openapi.PtrString(g.Condition)
			if g.Revision != "" {
				out.Revision = openapi.PtrString(g.Revision)
			}
		}
	}
	if seeTargets {
		out.Target = presentTarget(target)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
