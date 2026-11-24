package policyinterceptor

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	extv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/extensionv1"
	gi "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/gatewayinterceptorv1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
	"github.com/openshift-online/hypershell/components/control-plane/pkg/version"
)

// Extension protocol negotiation (OpenShell extensibility overview, "Protocol
// Negotiation").
const (
	ProtocolMajor          = 1
	ProtocolMinor          = 0
	CapabilityContract     = "openshell.gateway-interceptor.contract"
	ImplementationName     = "hypershell/policy-interceptor"
	statusPermissionDenied = "PERMISSION_DENIED"
	statusInvalidArgument  = "INVALID_ARGUMENT"
)

var supportedCapabilities = []string{CapabilityContract}

// Server implements openshell.gateway_interceptor.v1.GatewayInterceptor.
type Server struct {
	gi.UnimplementedGatewayInterceptorServer

	GatewayID   string
	Audience    string
	Defaults    DefaultQuantities
	PoliciesURL string
	Store       *Store
	Events      *EventBuffer
	Metrics     *Metrics
	Lookup      SandboxLookup
	Log         *slog.Logger
	Now         func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// Manifest returns the interceptor manifest built from the static binding table.
func (s *Server) Manifest() *gi.InterceptorManifest {
	m := &gi.InterceptorManifest{
		Name:             InterceptorName,
		FailurePolicy:    FailClosed,
		ProviderProfiles: false,
		ExpectedAudience: s.Audience,
		Extension: &extv1.PeerMetadata{
			ProtocolVersion:       &extv1.ProtocolVersion{Major: ProtocolMajor, Minor: ProtocolMinor},
			ImplementationName:    ImplementationName,
			ImplementationVersion: version.Version,
			SupportedCapabilities: slices.Clone(supportedCapabilities),
		},
	}
	for _, b := range Bindings() {
		pb := &gi.InterceptorBinding{
			Id:            b.ID,
			Selector:      &gi.InterceptorSelector{Rpc: b.RPC},
			FailurePolicy: b.FailurePolicy,
		}
		for _, p := range b.Phases {
			pb.Phases = append(pb.Phases, phaseToProto(p))
		}
		m.Bindings = append(m.Bindings, pb)
	}
	return m
}

func phaseToProto(p string) gi.GatewayInterceptorPhase {
	switch p {
	case PhaseModifyOperation:
		return gi.GatewayInterceptorPhase_GATEWAY_INTERCEPTOR_PHASE_MODIFY_OPERATION
	case PhaseValidate:
		return gi.GatewayInterceptorPhase_GATEWAY_INTERCEPTOR_PHASE_VALIDATE
	default:
		return gi.GatewayInterceptorPhase_GATEWAY_INTERCEPTOR_PHASE_UNSPECIFIED
	}
}

// Describe returns the manifest after checking the gateway's protocol
// metadata. It works before any bundle loads, so the gateway can start.
func (s *Server) Describe(_ context.Context, req *gi.DescribeRequest) (*gi.InterceptorManifest, error) {
	gw := req.GetGateway()
	if gw == nil || gw.GetProtocolVersion() == nil {
		return nil, status.Error(codes.FailedPrecondition, "gateway protocol metadata is required")
	}
	if gw.GetProtocolVersion().GetMajor() != ProtocolMajor {
		return nil, status.Errorf(codes.FailedPrecondition,
			"gateway-interceptor protocol major %d is not supported (want %d)", gw.GetProtocolVersion().GetMajor(), ProtocolMajor)
	}
	var unmet []string
	for _, c := range gw.GetRequiredCapabilities() {
		if !slices.Contains(supportedCapabilities, c) {
			unmet = append(unmet, c)
		}
	}
	if len(unmet) > 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "unsupported required capabilities: %s", strings.Join(unmet, ", "))
	}
	return s.Manifest(), nil
}

// SnapshotProviderProfiles is not implemented; the manifest advertises
// provider_profiles = false.
func (s *Server) SnapshotProviderProfiles(context.Context, *gi.ProviderProfileSnapshotRequest) (*gi.ProviderProfileSnapshot, error) {
	return nil, status.Error(codes.Unimplemented, "the HyperShell policy interceptor does not vend provider profiles")
}

// errNoBundle is returned on fail_closed bindings before the first bundle loads,
// so the gateway fails the call closed (PLAT-PGP-12.02).
var errNoBundle = status.Error(codes.Unavailable, "no platform policy bundle is loaded yet")

func allow() *gi.InterceptorResult { return &gi.InterceptorResult{Allowed: true} }

// Evaluate handles one bound operation phase.
func (s *Server) Evaluate(ctx context.Context, req *gi.InterceptorEvaluation) (*gi.InterceptorResult, error) {
	binding, ok := bindingByRPC(req.GetService(), req.GetMethod())
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "RPC %s/%s is not bound", req.GetService(), req.GetMethod())
	}
	var phase string
	switch req.GetPhase().(type) {
	case *gi.InterceptorEvaluation_ModifyOperation:
		phase = PhaseModifyOperation
	case *gi.InterceptorEvaluation_Validate:
		phase = PhaseValidate
	default:
		phase = "unsupported"
	}
	if !binding.HasPhase(phase) {
		s.record(binding, phase, "error")
		return nil, status.Errorf(codes.InvalidArgument, "phase %s is not bound for %s", phase, binding.RPC)
	}

	res, err := s.evaluate(ctx, binding, phase, req)
	switch {
	case err != nil:
		s.record(binding, phase, "error")
		s.log().WarnContext(ctx, "policy evaluation failed", "rpc", binding.RPC, "phase", phase,
			"failure_policy", binding.FailurePolicy, "error", err)
		return nil, err
	case !res.GetAllowed():
		s.record(binding, phase, "deny")
	default:
		s.record(binding, phase, "allow")
	}
	return res, nil
}

func (s *Server) record(b Binding, phase, result string) {
	if s.Metrics != nil {
		s.Metrics.Evaluations.WithLabelValues(b.Method(), phase, result).Inc()
	}
}

func (s *Server) evaluate(ctx context.Context, b Binding, phase string, req *gi.InterceptorEvaluation) (*gi.InterceptorResult, error) {
	snap := s.Store.Current()
	if snap == nil {
		if b.FailurePolicy == FailClosed {
			return nil, errNoBundle
		}
		return allow(), nil
	}
	if b.Service() != ServiceOpenShell {
		return allow(), nil
	}
	switch b.Method() {
	case "CreateSandbox":
		if phase == PhaseModifyOperation {
			return s.modifyCreate(ctx, snap, req)
		}
		return s.validateCreate(ctx, snap, req)
	case "StartSandbox", "ResumeSandbox":
		return s.recheck(ctx, snap, b, req)
	default:
		return allow(), nil
	}
}

func proposed(req *gi.InterceptorEvaluation) map[string]any {
	var st interface{ AsMap() map[string]any }
	if m := req.GetModifyOperation(); m != nil && m.GetProposedOperation() != nil {
		st = m.GetProposedOperation()
	} else if v := req.GetValidate(); v != nil && v.GetProposedOperation() != nil {
		st = v.GetProposedOperation()
	}
	if st == nil {
		return map[string]any{}
	}
	return st.AsMap()
}

// defaultsFor returns the fleet defaults to apply with this bundle revision
// (PLAT-PGP-08.05). The bundle's defaults are authoritative: they are the ones
// the bundle was validated against and they follow a defaults change within
// seconds, while the environment defaults change only when the pod rolls. When
// the two hashes differ the sidecar applies the bundle's defaults and records
// the mismatch in a metric and, once per revision, a log line. A bundle
// without defaults falls back to the environment.
func (s *Server) defaultsFor(snap *snapshot) DefaultQuantities {
	if snap == nil || snap.defaults == nil {
		return s.Defaults
	}
	if snap.bundle.Defaults.Hash == policybundle.DefaultsHash(s.Defaults.Parsed) {
		return s.Defaults
	}
	if s.Metrics != nil {
		s.Metrics.DefaultsMismatch.Inc()
	}
	if snap.mismatchLogged.CompareAndSwap(false, true) {
		s.log().Warn("sidecar environment fleet defaults differ from the policy bundle; applying the bundle defaults until the gateway pod rolls",
			"revision", snap.bundle.Revision, "bundle_hash", snap.bundle.Defaults.Hash, "env_hash", policybundle.DefaultsHash(s.Defaults.Parsed))
	}
	return *snap.defaults
}

// modifyCreate adds the missing fleet defaults when an enforced policy turns
// injection on for this sandbox (snapshot.injectsDefaultsFor); otherwise it
// changes nothing and records a would_add_defaults finding (PLAT-PGP-08).
func (s *Server) modifyCreate(ctx context.Context, snap *snapshot, req *gi.InterceptorEvaluation) (*gi.InterceptorResult, error) {
	sr, err := ParseCreateSandbox(proposed(req))
	if err != nil {
		return s.invalidRequest(snap, err)
	}
	if !snap.bundle.ResourcePoliciesSupported {
		return allow(), nil
	}
	defaults := s.defaultsFor(snap)
	if snap.injectsDefaultsFor(sr.Name) {
		patches, err := sr.DefaultPatches(defaults)
		if err != nil {
			return s.invalidRequest(snap, err)
		}
		return &gi.InterceptorResult{Allowed: true, Patches: patches}, nil
	}
	missing, err := sr.MissingDefaults(defaults)
	if err != nil {
		// Strict audit (PLAT-PGP-07): an unparsable value changes nothing here;
		// the gateway and driver validate the request themselves.
		s.log().InfoContext(ctx, "cannot compute fleet defaults for audit", "error", err)
		return allow(), nil
	}
	if len(missing) > 0 {
		summary := cedar.Context{}
		if c, cerr := sr.Context(defaults); cerr == nil {
			summary = c
		}
		s.emit(snap, req, "CreateSandbox", OutcomeWouldAddDefaults, nil, summary, sr.RequestID)
		if s.Metrics != nil {
			s.Metrics.WouldAddDefaults.Inc()
		}
	}
	return allow(), nil
}

// invalidRequest handles a request whose resource values cannot be normalized.
// With an enforced policy the request cannot be evaluated, so it is denied
// with INVALID_ARGUMENT; with no enforced policy nothing changes (PLAT-PGP-07).
func (s *Server) invalidRequest(snap *snapshot, err error) (*gi.InterceptorResult, error) {
	if snap.hasEnforced() {
		return &gi.InterceptorResult{
			Allowed:    false,
			StatusCode: statusInvalidArgument,
			Reason:     "Platform policy cannot evaluate the sandbox resources: " + err.Error(),
		}, nil
	}
	return allow(), nil
}

// validateCreate evaluates the enforced set and denies with PERMISSION_DENIED
// when any enforced forbid matches (PLAT-PGP-13), then evaluates the
// audit-only set and records would_reject findings (PLAT-PGP-07).
func (s *Server) validateCreate(ctx context.Context, snap *snapshot, req *gi.InterceptorEvaluation) (*gi.InterceptorResult, error) {
	sr, err := ParseCreateSandbox(proposed(req))
	if err != nil {
		return s.invalidRequest(snap, err)
	}
	cctx, err := sr.Context(s.defaultsFor(snap))
	if err != nil {
		return s.invalidRequest(snap, err)
	}
	return s.decide(ctx, snap, req, "CreateSandbox", cctx, sr.RequestID)
}

// decide is the shared enforced-then-audit evaluation.
func (s *Server) decide(ctx context.Context, snap *snapshot, req *gi.InterceptorEvaluation, operation string, cctx cedar.Context, requestID string) (*gi.InterceptorResult, error) {
	creq := cedarRequest(req.GetPrincipal(), s.GatewayID, cctx)
	result := allow()
	if snap.hasEnforced() {
		d := snap.enforced.Evaluate(creq)
		if len(d.Errors) > 0 {
			// A forbid whose condition errored does not apply in Cedar; on this
			// fail-closed path that is a failure, not an allow.
			return nil, status.Errorf(codes.Internal, "enforced policy evaluation error: %s", strings.Join(d.Errors, "; "))
		}
		if !d.Allowed {
			result = &gi.InterceptorResult{
				Allowed:    false,
				StatusCode: statusPermissionDenied,
				Reason:     cedar.DenialReason(d, snap.messages, s.PoliciesURL),
				LogAnnotations: map[string]string{
					"hypershell.policy/revision": snap.bundle.Revision,
					"hypershell.policy/policies": strings.Join(d.MatchedPolicies(), ","),
				},
			}
			if s.Metrics != nil {
				s.Metrics.Denials.Inc()
			}
		}
	}
	if snap.audit != nil && !snap.audit.Empty() {
		d := snap.audit.Evaluate(creq)
		if len(d.Errors) > 0 {
			s.log().WarnContext(ctx, "audit-only policy evaluation error", "errors", strings.Join(d.Errors, "; "))
		}
		if names := d.MatchedPolicies(); len(names) > 0 {
			s.emit(snap, req, operation, OutcomeWouldReject, names, cctx, requestID)
			if s.Metrics != nil {
				s.Metrics.WouldReject.Inc()
			}
		}
	}
	return result, nil
}

// recheck re-evaluates an existing sandbox on start or resume (PLAT-PGP-13,
// PLAT-PGP-16). validate.current_state is always empty (AGT-EXT-06), so the
// sandbox resources come from the SandboxLookup; when they cannot be read the
// call is allowed, logged, and counted.
func (s *Server) recheck(ctx context.Context, snap *snapshot, b Binding, req *gi.InterceptorEvaluation) (*gi.InterceptorResult, error) {
	if !snap.hasEnforced() && (snap.audit == nil || snap.audit.Empty()) {
		return allow(), nil
	}
	op := proposed(req)
	name := str(op["name"])
	workspace := ""
	if ws, ok := field(op, "workspaceScope", "workspace_scope").(map[string]any); ok {
		workspace = str(ws["workspace"])
	}
	lookup := s.Lookup
	if lookup == nil {
		lookup = NoSandboxLookup{}
	}
	cctx, err := lookup.Lookup(ctx, workspace, name)
	if err != nil {
		if s.Metrics != nil {
			s.Metrics.RecheckSkipped.WithLabelValues(b.Method()).Inc()
		}
		s.log().InfoContext(ctx, "sandbox re-check skipped; allowing", "rpc", b.RPC, "sandbox", name, "reason", err)
		return allow(), nil
	}
	return s.decide(ctx, snap, req, b.Method(), cctx, str(field(op, "requestId", "request_id")))
}

// cedarRequest builds the Cedar request from the gateway principal map (kind,
// subject, roles as a comma-separated list).
func cedarRequest(principal map[string]string, gatewayID string, cctx cedar.Context) cedar.Request {
	var roles []string
	for _, r := range strings.Split(principal["roles"], ",") {
		if r = strings.TrimSpace(r); r != "" {
			roles = append(roles, r)
		}
	}
	return cedar.Request{
		PrincipalSubject: principalSubject(principal),
		PrincipalRoles:   roles,
		GatewayID:        gatewayID,
		GatewayName:      gatewayID,
		Context:          cctx,
	}
}

func principalSubject(p map[string]string) string {
	if sub := p["subject"]; sub != "" {
		return sub
	}
	if id := p["sandbox_id"]; id != "" {
		return "sandbox:" + id
	}
	if kind := p["kind"]; kind != "" {
		return kind
	}
	return "unknown"
}

func (s *Server) emit(snap *snapshot, req *gi.InterceptorEvaluation, operation, outcome string, policies []string, cctx cedar.Context, requestID string) {
	if s.Events == nil {
		return
	}
	s.Events.Add(Event{
		ID:               eventID(s.GatewayID, operation, outcome, snap.bundle.Revision, requestID),
		OccurredAt:       s.now().UTC(),
		PrincipalSubject: principalSubject(req.GetPrincipal()),
		Operation:        operation,
		Outcome:          outcome,
		PolicyNames:      policies,
		BundleRevision:   snap.bundle.Revision,
		RequestSummary:   cctx.Summary(),
	})
}
