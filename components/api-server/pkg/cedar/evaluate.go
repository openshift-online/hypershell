package cedar

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cedar-policy/cedar-go"
	"github.com/cedar-policy/cedar-go/types"
	xast "github.com/cedar-policy/cedar-go/x/exp/ast"
)

// PolicyInput is one policy's forbid-only Cedar text identified by the policy
// name, the unit the control plane annotates with @id (PLAT-PGP-14).
type PolicyInput struct {
	Name  string
	Cedar string
}

// Context holds the normalized Cedar context values of one request
// (PLAT-PGP-03): resource quantities already converted to millicores and bytes.
type Context struct {
	CPURequestMillis   int64
	CPULimitMillis     int64
	MemoryRequestBytes int64
	MemoryLimitBytes   int64
	GPUCount           int64
	Image              string
	SandboxName        string
}

// Request is the driver-neutral Cedar request the interceptor (and the admin
// test panel) evaluates: the principal, the gateway resource, and the context
// after fleet defaults are applied.
type Request struct {
	PrincipalSubject string
	PrincipalRoles   []string
	GatewayID        string
	GatewayName      string
	Context          Context
}

// Violation is one recognized context comparison of a matching statement that
// holds for the request: the offending value and the allowed bound, both in
// plain words (PLAT-PGP-14). Field is the schema context attribute.
type Violation struct {
	Field string
	Label string
	Value string
	Bound string
}

// Match is one forbid statement that matched the request.
type Match struct {
	Policy         string
	StatementIndex int
	Cedar          string
	Violations     []Violation
}

// Decision is the result of evaluating a policy set for one request. Allowed is
// false when any forbid statement matched. Errors carries Cedar evaluation
// errors (a statement whose condition errored does not apply in Cedar); callers
// on a fail-closed path treat a non-empty Errors as a failure.
type Decision struct {
	Allowed bool
	Matches []Match
	Errors  []string
}

// MatchedPolicies returns the distinct names of the policies whose statements
// matched, in evaluation order.
func (d Decision) MatchedPolicies() []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range d.Matches {
		if !seen[m.Policy] {
			seen[m.Policy] = true
			out = append(out, m.Policy)
		}
	}
	return out
}

type statement struct {
	policy      string
	policyIndex int
	index       int
	cedar       string
	ast         *xast.Policy
}

// Evaluator evaluates a compiled set of forbid-only policies together with the
// built-in base permit (PLAT-PGP-02). It is immutable after construction and
// safe for concurrent use.
type Evaluator struct {
	set        *cedar.PolicySet
	statements map[cedar.PolicyID]statement
}

const basePermitID = cedar.PolicyID("__hypershell_base_permit__")

// NewEvaluator compiles the policies into one Cedar policy set. Every statement
// is stamped with @id("<policy name>") (PLAT-PGP-14) and the base permit is
// added, so a request is allowed unless a forbid matches it.
func NewEvaluator(policies []PolicyInput) (*Evaluator, error) {
	e := &Evaluator{set: cedar.NewPolicySet(), statements: map[cedar.PolicyID]statement{}}
	base, err := cedar.NewPolicyListFromBytes("base.cedar", []byte(basePermitText))
	if err != nil {
		return nil, fmt.Errorf("parse base permit: %w", err)
	}
	e.set.Add(basePermitID, base[0])
	for pi, p := range policies {
		if p.Name == "" {
			return nil, fmt.Errorf("policy %d has no name", pi)
		}
		list, err := cedar.NewPolicyListFromBytes(p.Name+".cedar", []byte(p.Cedar))
		if err != nil {
			return nil, fmt.Errorf("parse policy %q: %w", p.Name, err)
		}
		for si, stmt := range list {
			if stmt.Effect() != cedar.Forbid {
				return nil, fmt.Errorf("policy %q statement %d is not a forbid", p.Name, si)
			}
			stmt.AST().Annotations = []xast.AnnotationType{{Key: "id", Value: types.String(p.Name)}}
			id := cedar.PolicyID(fmt.Sprintf("%s#%d", p.Name, si))
			e.set.Add(id, stmt)
			e.statements[id] = statement{
				policy:      p.Name,
				policyIndex: pi,
				index:       si,
				cedar:       string(stmt.MarshalCedar()),
				ast:         (*xast.Policy)(stmt.AST()),
			}
		}
	}
	return e, nil
}

// Empty reports whether the evaluator holds no forbid statement.
func (e *Evaluator) Empty() bool { return e == nil || len(e.statements) == 0 }

// Evaluate decides the request with Cedar. Matches lists every forbid statement
// that matched, ordered by policy then statement, each with the recognized
// value-and-bound violations (PLAT-PGP-14).
func (e *Evaluator) Evaluate(req Request) Decision {
	if e.Empty() {
		return Decision{Allowed: true}
	}
	decision, diag := cedar.Authorize(e.set, req.entities(), req.cedarRequest())
	out := Decision{Allowed: decision == cedar.Allow}
	for _, r := range diag.Reasons {
		st, ok := e.statements[r.PolicyID]
		if !ok {
			continue
		}
		out.Matches = append(out.Matches, Match{
			Policy:         st.policy,
			StatementIndex: st.index,
			Cedar:          st.cedar,
			Violations:     violations(st.ast, req.Context),
		})
	}
	sort.SliceStable(out.Matches, func(i, j int) bool {
		a := e.statements[cedar.PolicyID(fmt.Sprintf("%s#%d", out.Matches[i].Policy, out.Matches[i].StatementIndex))]
		b := e.statements[cedar.PolicyID(fmt.Sprintf("%s#%d", out.Matches[j].Policy, out.Matches[j].StatementIndex))]
		if a.policyIndex != b.policyIndex {
			return a.policyIndex < b.policyIndex
		}
		return a.index < b.index
	})
	for _, de := range diag.Errors {
		out.Errors = append(out.Errors, de.String())
	}
	sort.Strings(out.Errors)
	return out
}

func (r Request) cedarRequest() cedar.Request {
	return cedar.Request{
		Principal: types.NewEntityUID(EntityTypeUser, types.String(principalID(r.PrincipalSubject))),
		Action:    types.NewEntityUID(ActionEntityType, ActionCreateSandbox),
		Resource:  types.NewEntityUID(EntityTypeGateway, types.String(gatewayID(r.GatewayID))),
		Context:   r.Context.record(),
	}
}

func (r Request) entities() types.EntityMap {
	roles := make([]types.Value, 0, len(r.PrincipalRoles))
	for _, role := range r.PrincipalRoles {
		roles = append(roles, types.String(role))
	}
	principal := types.NewEntityUID(EntityTypeUser, types.String(principalID(r.PrincipalSubject)))
	gateway := types.NewEntityUID(EntityTypeGateway, types.String(gatewayID(r.GatewayID)))
	return types.EntityMap{
		principal: {
			UID: principal,
			Attributes: types.NewRecord(types.RecordMap{
				"subject": types.String(r.PrincipalSubject),
				"roles":   types.NewSet(roles...),
			}),
		},
		gateway: {
			UID: gateway,
			Attributes: types.NewRecord(types.RecordMap{
				"id":   types.String(r.GatewayID),
				"name": types.String(r.GatewayName),
			}),
		},
	}
}

func principalID(subject string) string {
	if subject == "" {
		return "anonymous"
	}
	return subject
}

func gatewayID(id string) string {
	if id == "" {
		return "gateway"
	}
	return id
}

func (c Context) record() types.Record {
	return types.NewRecord(types.RecordMap{
		CtxCPURequestMillis:   types.Long(c.CPURequestMillis),
		CtxCPULimitMillis:     types.Long(c.CPULimitMillis),
		CtxMemoryRequestBytes: types.Long(c.MemoryRequestBytes),
		CtxMemoryLimitBytes:   types.Long(c.MemoryLimitBytes),
		CtxGPUCount:           types.Long(c.GPUCount),
		CtxImage:              types.String(c.Image),
		CtxSandboxName:        types.String(c.SandboxName),
	})
}

// value returns the normalized context value for a resource field.
func (c Context) value(field string) (int64, bool) {
	switch field {
	case CtxCPURequestMillis:
		return c.CPURequestMillis, true
	case CtxCPULimitMillis:
		return c.CPULimitMillis, true
	case CtxMemoryRequestBytes:
		return c.MemoryRequestBytes, true
	case CtxMemoryLimitBytes:
		return c.MemoryLimitBytes, true
	case CtxGPUCount:
		return c.GPUCount, true
	default:
		return 0, false
	}
}

// Summary returns the normalized resource values as a map keyed by the schema
// context attribute names, the shape stored in audit findings (PLAT-DATA-30):
// normalized values only, never a secret request field (PLAT-PGP-14).
func (c Context) Summary() map[string]int64 {
	return map[string]int64{
		CtxCPURequestMillis:   c.CPURequestMillis,
		CtxCPULimitMillis:     c.CPULimitMillis,
		CtxMemoryRequestBytes: c.MemoryRequestBytes,
		CtxMemoryLimitBytes:   c.MemoryLimitBytes,
		CtxGPUCount:           c.GPUCount,
	}
}

// DefaultContext is the context of the default request: every fleet default
// applied and no user value set (PLAT-PGP-04 rule 3).
func (d Defaults) DefaultContext() Context {
	return Context{
		CPURequestMillis:   d.CPURequestMillis,
		CPULimitMillis:     d.CPULimitMillis,
		MemoryRequestBytes: d.MemoryRequestBytes,
		MemoryLimitBytes:   d.MemoryLimitBytes,
		GPUCount:           d.GPUCount,
	}
}

// violations walks a matching statement's when-conditions and returns every
// recognized context comparison (through && and ||) that holds for the request,
// so the denial names the offending value and the bound. The decision itself
// always comes from the Cedar evaluation; this only explains it (PLAT-PGP-17).
func violations(p *xast.Policy, ctx Context) []Violation {
	var out []Violation
	for _, cond := range p.Conditions {
		if cond.Condition != xast.ConditionWhen {
			continue
		}
		for _, c := range leafComparisons(cond.Body) {
			v, ok := ctx.value(c.field)
			if !ok || !c.holds(v) {
				continue
			}
			fd := fieldDisplays[c.field]
			bound, ok := renderComparison(c)
			if !ok {
				continue
			}
			out = append(out, Violation{
				Field: c.field,
				Label: fd.label,
				Value: fd.format(v),
				Bound: strings.TrimPrefix(bound, fd.label+": "),
			})
		}
	}
	return out
}

// leafComparisons collects every recognized resource comparison reachable
// through conjunctions and disjunctions.
func leafComparisons(node xast.IsNode) []comparison {
	switch n := node.(type) {
	case xast.NodeTypeAnd:
		return append(leafComparisons(n.Left), leafComparisons(n.Right)...)
	case xast.NodeTypeOr:
		return append(leafComparisons(n.Left), leafComparisons(n.Right)...)
	}
	if c, ok := extractComparison(node); ok {
		if _, known := fieldDisplays[c.field]; known {
			return []comparison{c}
		}
	}
	return nil
}

func (c comparison) holds(v int64) bool {
	switch c.op {
	case "<":
		return v < c.bound
	case "<=":
		return v <= c.bound
	case ">":
		return v > c.bound
	case ">=":
		return v >= c.bound
	case "==":
		return v == c.bound
	case "!=":
		return v != c.bound
	default:
		return false
	}
}

// PolicyMessage is the user-facing metadata of one policy used to build the
// denial reason (PLAT-PGP-14).
type PolicyMessage struct {
	UserMessage      string
	DocumentationURL string
}

// DenialReason builds the InterceptorResult.reason for a denied request
// (PLAT-PGP-14): one line per matching policy, naming the policy and showing
// its userMessage, plus the offending value and the bound for every recognized
// comparison that holds. The reason ends with the policy documentationUrl when
// exactly one policy matched, otherwise with policiesURL (the gateway Policies
// tab). Only normalized resource values appear; no request field is copied.
func DenialReason(d Decision, messages map[string]PolicyMessage, policiesURL string) string {
	names := d.MatchedPolicies()
	if len(names) == 0 {
		return ""
	}
	violationsByPolicy := map[string][]Violation{}
	for _, m := range d.Matches {
		violationsByPolicy[m.Policy] = append(violationsByPolicy[m.Policy], m.Violations...)
	}
	var b strings.Builder
	if len(names) == 1 {
		b.WriteString("Rejected by platform policy:")
	} else {
		fmt.Fprintf(&b, "Rejected by %d platform policies:", len(names))
	}
	for _, name := range names {
		b.WriteString("\n- ")
		b.WriteString(name)
		if msg := strings.TrimSpace(messages[name].UserMessage); msg != "" {
			b.WriteString(": ")
			b.WriteString(msg)
		}
		seen := map[string]bool{}
		for _, v := range violationsByPolicy[name] {
			text := fmt.Sprintf("%s is %s; allowed: %s", v.Label, v.Value, v.Bound)
			if seen[text] {
				continue
			}
			seen[text] = true
			b.WriteString(" (")
			b.WriteString(text)
			b.WriteString(")")
		}
	}
	link := policiesURL
	if len(names) == 1 {
		if doc := strings.TrimSpace(messages[names[0]].DocumentationURL); doc != "" {
			link = doc
		}
	}
	if link != "" {
		b.WriteString("\nMore information: ")
		b.WriteString(link)
	}
	return b.String()
}
