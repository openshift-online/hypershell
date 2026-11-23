package cedar

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cedar-policy/cedar-go"
	"github.com/cedar-policy/cedar-go/types"
	xast "github.com/cedar-policy/cedar-go/x/exp/ast"
	"github.com/cedar-policy/cedar-go/x/exp/schema/validate"
)

// Diagnostic is one write-time validation finding, attributed to the 1-based
// line of the offending statement (0 when no line applies) with a suggested fix
// when one is known (WEB-PADM-05).
type Diagnostic struct {
	Line       int
	Statement  int
	Message    string
	Suggestion string
}

// ValidationError carries the write-time validation diagnostics for a policy
// (PLAT-PGP-04). The API server surfaces these to the author so a rejected
// write names the offending statement, attribute, or rule.
type ValidationError struct {
	Diagnostics []string
	Details     []Diagnostic
}

func (e *ValidationError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "cedar policy validation failed"
	}
	return "cedar policy validation failed: " + strings.Join(e.Diagnostics, "; ")
}

func newValidationError(details []Diagnostic) *ValidationError {
	e := &ValidationError{Details: details}
	for _, d := range details {
		e.Diagnostics = append(e.Diagnostics, d.Message)
	}
	return e
}

var parseLineRE = regexp.MustCompile(`:(\d+):\d+`)
var unknownAttrRE = regexp.MustCompile("attribute `([A-Za-z0-9_]+)` in context")

// ValidatePolicy applies the write-time validation contract of PLAT-PGP-04 to
// forbid-only Cedar policy text against the HyperShell Cedar schema and the
// supplied fleet defaults. It enforces, and reports a ValidationError for any
// failure of, all of:
//
//  1. the text parses and validates against the current Cedar schema; unknown
//     attributes and wrong types are rejected with the cedar-go diagnostic;
//  2. every statement is a forbid; a permit is rejected (PLAT-PGP-02);
//  3. the default request (every fleet default applied, no user value set) is
//     not forbidden by the policy;
//  4. author-written statement annotations are rejected; the control plane owns
//     the @id annotation (PLAT-PGP-14).
//
// A policy that parses and passes every rule returns nil.
func ValidatePolicy(cedarText string, defaults Defaults) error {
	policies, err := cedar.NewPolicyListFromBytes("policy.cedar", []byte(cedarText))
	if err != nil {
		d := Diagnostic{Message: fmt.Sprintf("parse error: %v", err), Suggestion: "check the statement syntax: forbid (principal, action, resource) when { ... };"}
		if m := parseLineRE.FindStringSubmatch(err.Error()); m != nil {
			d.Line, _ = strconv.Atoi(m[1])
		}
		return newValidationError([]Diagnostic{d})
	}
	if len(policies) == 0 {
		return newValidationError([]Diagnostic{{Message: "policy must contain at least one forbid statement", Suggestion: "add a statement such as forbid (principal, action, resource) when { context.cpuRequestMillis > 4000 };"}})
	}
	rs, err := resolvedSchema()
	if err != nil {
		return err
	}
	v := validate.New(rs, validate.WithStrict())
	var diags []Diagnostic
	for i, p := range policies {
		id := fmt.Sprintf("policy%d", i)
		line := p.Position().Line
		// Rule 2: forbid-only.
		if p.Effect() != cedar.Forbid {
			diags = append(diags, Diagnostic{Line: line, Statement: i, Message: fmt.Sprintf("%s: a permit is not allowed; platform policies can only use forbid", id), Suggestion: "replace permit with forbid and describe what to reject"})
		}
		// Rule 4: no author-written annotations.
		if len(p.Annotations()) > 0 {
			diags = append(diags, Diagnostic{Line: line, Statement: i, Message: fmt.Sprintf("%s: author-written statement annotations are not allowed; the control plane owns @id", id), Suggestion: "remove the annotation; HyperShell adds @id with the policy name"})
		}
		// Rule 1: schema validation (unknown attributes, wrong types).
		if err := v.Policy(id, (*xast.Policy)(p.AST())); err != nil {
			for _, msg := range collectDiagnostics(err) {
				diags = append(diags, Diagnostic{Line: line, Statement: i, Message: msg, Suggestion: suggestFix(msg)})
			}
		}
	}
	if len(diags) > 0 {
		return newValidationError(diags)
	}
	// Rule 3: the default request must not be forbidden.
	forbidden, err := forbidsDefaultRequest(policies, defaults)
	if err != nil {
		return fmt.Errorf("default-request check: %w", err)
	}
	if forbidden {
		return newValidationError([]Diagnostic{{
			Message:    "policy forbids the default sandbox request; it would reject every sandbox that omits a value",
			Suggestion: "widen the bound so the fleet default values are allowed",
		}})
	}
	return nil
}

// contextAttributes lists the schema context attributes with their type and
// unit, used to suggest a fix for an unknown attribute.
var contextAttributes = []struct{ name, kind string }{
	{CtxCPURequestMillis, "a Long in millicores"},
	{CtxCPULimitMillis, "a Long in millicores"},
	{CtxMemoryRequestBytes, "a Long in bytes"},
	{CtxMemoryLimitBytes, "a Long in bytes"},
	{CtxGPUCount, "a Long"},
	{CtxImage, "a String"},
	{CtxSandboxName, "a String"},
}

// suggestFix proposes a fix for a cedar-go schema diagnostic: for an unknown
// context attribute, the closest schema attribute (WEB-PADM-05).
func suggestFix(msg string) string {
	if m := unknownAttrRE.FindStringSubmatch(msg); m != nil {
		unknown := strings.ToLower(m[1])
		for _, a := range contextAttributes {
			if strings.HasPrefix(strings.ToLower(a.name), unknown) || strings.HasPrefix(unknown, strings.ToLower(a.name)) {
				return fmt.Sprintf("context.%s is not available; use context.%s (%s)", m[1], a.name, a.kind)
			}
		}
		names := make([]string, 0, len(contextAttributes))
		for _, a := range contextAttributes {
			names = append(names, a.name)
		}
		return fmt.Sprintf("context.%s is not available; use one of: %s", m[1], strings.Join(names, ", "))
	}
	if strings.Contains(msg, "unexpected type") {
		return "compare resource attributes with integer literals in schema units (millicores, bytes)"
	}
	return ""
}

// forbidsDefaultRequest evaluates the forbid-only policy together with the
// built-in base permit against the default request and reports whether a forbid
// statement matches it (PLAT-PGP-04 rule 3).
func forbidsDefaultRequest(policies cedar.PolicyList, defaults Defaults) (bool, error) {
	eval := cedar.NewPolicySet()
	for i, p := range policies {
		eval.Add(cedar.PolicyID(fmt.Sprintf("policy%d", i)), p)
	}
	base, err := cedar.NewPolicyListFromBytes("base.cedar", []byte(basePermitText))
	if err != nil {
		return false, fmt.Errorf("parse base permit: %w", err)
	}
	eval.Add("__hypershell_base_permit__", base[0])

	decision, _ := cedar.Authorize(eval, types.EntityMap{}, defaults.request())
	return decision == cedar.Deny, nil
}

// collectDiagnostics flattens a cedar-go validation error, which may join
// several per-statement diagnostics, into individual messages.
func collectDiagnostics(err error) []string {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var out []string
		for _, e := range joined.Unwrap() {
			out = append(out, collectDiagnostics(e)...)
		}
		if len(out) > 0 {
			return out
		}
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return nil
	}
	lines := strings.Split(msg, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if trimmed := strings.TrimSpace(l); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
