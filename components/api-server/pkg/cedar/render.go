package cedar

import (
	"fmt"
	"strconv"

	"github.com/cedar-policy/cedar-go"
	"github.com/cedar-policy/cedar-go/types"
	xast "github.com/cedar-policy/cedar-go/x/exp/ast"
)

// RenderedRule is the best-effort display of one forbid statement (PLAT-PGP-17).
// Cedar is always set to the statement source. When Recognized is true, Summary
// holds the plain-words bounds a user sees (for example "CPU request: at most
// 4"); when false, the caller shows the Cedar source with the policy message.
type RenderedRule struct {
	Recognized bool
	Summary    []string
	Cedar      string
	// Bounds is the structured form of Summary, one entry per recognized
	// comparison, so a console can attribute each bound to a resource row.
	Bounds []Bound
}

// Bound is one recognized comparison rendered as the allowed bound. Op is the
// forbid operator over Field (for example ">" for "at most"); Value is the bound
// in schema units and Display the bound formatted as a quantity.
type Bound struct {
	Field   string
	Label   string
	Op      string
	Value   int64
	Display string
	Text    string
}

// RenderPolicy renders each forbid statement in the policy text (PLAT-PGP-17).
// The renderer affects display only; enforcement, the default-request check, and
// the denial value-and-bound text always use the Cedar evaluation, not the
// renderer.
func RenderPolicy(cedarText string) ([]RenderedRule, error) {
	policies, err := cedar.NewPolicyListFromBytes("policy.cedar", []byte(cedarText))
	if err != nil {
		return nil, fmt.Errorf("parse cedar: %w", err)
	}
	out := make([]RenderedRule, 0, len(policies))
	for _, p := range policies {
		rule := RenderedRule{Cedar: string(p.MarshalCedar())}
		if summary, bounds, ok := renderStatement(p); ok {
			rule.Recognized = true
			rule.Summary = summary
			rule.Bounds = bounds
		}
		out = append(out, rule)
	}
	return out, nil
}

// comparison is a recognized "context.<field> <op> <literal>" clause.
type comparison struct {
	field string
	op    string
	bound int64
}

func renderStatement(p *cedar.Policy) ([]string, []Bound, bool) {
	xp := (*xast.Policy)(p.AST())
	// v1 recognizes a single "when" condition. Multiple when clauses are
	// AND-combined in Cedar, so they cannot be decomposed into independent
	// allowed bounds and are shown as Cedar source instead.
	if len(xp.Conditions) != 1 {
		return nil, nil, false
	}
	cond := xp.Conditions[0]
	// An "unless" is shown as Cedar.
	if cond.Condition != xast.ConditionWhen {
		return nil, nil, false
	}
	comparisons, ok := collectComparisons(cond.Body)
	if !ok || len(comparisons) == 0 {
		return nil, nil, false
	}
	summary := make([]string, 0, len(comparisons))
	bounds := make([]Bound, 0, len(comparisons))
	for _, c := range comparisons {
		text, ok := renderComparison(c)
		if !ok {
			return nil, nil, false
		}
		summary = append(summary, text)
		fd := fieldDisplays[c.field]
		bounds = append(bounds, Bound{Field: c.field, Label: fd.label, Op: c.op, Value: c.bound, Display: fd.format(c.bound), Text: text})
	}
	return summary, bounds, true
}

// collectComparisons walks a forbid condition that is either a single context
// comparison or a disjunction (||) of them, returning every leaf comparison. It
// returns ok=false for any other shape, including a conjunction (&&): for a
// forbid, a disjunction of bounds is the union of rejected regions, so each leaf
// is an independent allowed bound, but a conjunction rejects only when every
// operand holds and so cannot be shown as a list of independent bounds.
func collectComparisons(node xast.IsNode) ([]comparison, bool) {
	if or, ok := node.(xast.NodeTypeOr); ok {
		l, okL := collectComparisons(or.Left)
		r, okR := collectComparisons(or.Right)
		if !okL || !okR {
			return nil, false
		}
		return append(l, r...), true
	}
	c, ok := extractComparison(node)
	if !ok {
		return nil, false
	}
	return []comparison{c}, true
}

func extractComparison(node xast.IsNode) (comparison, bool) {
	switch n := node.(type) {
	case xast.NodeTypeLessThan:
		return buildComparison(n.Left, n.Right, "<")
	case xast.NodeTypeLessThanOrEqual:
		return buildComparison(n.Left, n.Right, "<=")
	case xast.NodeTypeGreaterThan:
		return buildComparison(n.Left, n.Right, ">")
	case xast.NodeTypeGreaterThanOrEqual:
		return buildComparison(n.Left, n.Right, ">=")
	case xast.NodeTypeEquals:
		return buildComparison(n.Left, n.Right, "==")
	case xast.NodeTypeNotEquals:
		return buildComparison(n.Left, n.Right, "!=")
	default:
		return comparison{}, false
	}
}

// buildComparison recognizes "context.<field> <op> <long>" and the mirrored
// "<long> <op> context.<field>" (flipping the operator).
func buildComparison(left, right xast.IsNode, op string) (comparison, bool) {
	if field, ok := contextField(left); ok {
		if bound, ok := longLiteral(right); ok {
			return comparison{field: field, op: op, bound: bound}, true
		}
	}
	if field, ok := contextField(right); ok {
		if bound, ok := longLiteral(left); ok {
			return comparison{field: field, op: flipOp(op), bound: bound}, true
		}
	}
	return comparison{}, false
}

func contextField(node xast.IsNode) (string, bool) {
	access, ok := node.(xast.NodeTypeAccess)
	if !ok {
		return "", false
	}
	variable, ok := access.Arg.(xast.NodeTypeVariable)
	if !ok || string(variable.Name) != "context" {
		return "", false
	}
	return string(access.Value), true
}

func longLiteral(node xast.IsNode) (int64, bool) {
	value, ok := node.(xast.NodeValue)
	if !ok {
		return 0, false
	}
	long, ok := value.Value.(types.Long)
	if !ok {
		return 0, false
	}
	return int64(long), true
}

func flipOp(op string) string {
	switch op {
	case "<":
		return ">"
	case "<=":
		return ">="
	case ">":
		return "<"
	case ">=":
		return "<="
	default:
		return op // == and != are symmetric
	}
}

type fieldDisplay struct {
	label  string
	format func(int64) string
}

var fieldDisplays = map[string]fieldDisplay{
	CtxCPURequestMillis:   {"CPU request", formatCPU},
	CtxCPULimitMillis:     {"CPU limit", formatCPU},
	CtxMemoryRequestBytes: {"Memory request", formatMemory},
	CtxMemoryLimitBytes:   {"Memory limit", formatMemory},
	CtxGPUCount:           {"GPU count", formatInt},
}

// renderComparison turns a forbid comparison into the allowed bound in plain
// words. A forbid matches the rejected region, so the rendered text states what
// a user is allowed instead.
func renderComparison(c comparison) (string, bool) {
	fd, ok := fieldDisplays[c.field]
	if !ok {
		return "", false
	}
	value := fd.format(c.bound)
	switch c.op {
	case ">":
		return fmt.Sprintf("%s: at most %s", fd.label, value), true
	case ">=":
		return fmt.Sprintf("%s: less than %s", fd.label, value), true
	case "<":
		return fmt.Sprintf("%s: at least %s", fd.label, value), true
	case "<=":
		return fmt.Sprintf("%s: more than %s", fd.label, value), true
	case "==":
		return fmt.Sprintf("%s: must not be %s", fd.label, value), true
	case "!=":
		return fmt.Sprintf("%s: must be %s", fd.label, value), true
	default:
		return "", false
	}
}

func formatCPU(millis int64) string {
	if millis%1000 == 0 {
		return strconv.FormatInt(millis/1000, 10)
	}
	return strconv.FormatInt(millis, 10) + "m"
}

func formatMemory(bytes int64) string {
	const (
		ki = int64(1024)
		mi = ki * 1024
		gi = mi * 1024
		ti = gi * 1024
	)
	switch {
	case bytes != 0 && bytes%ti == 0:
		return strconv.FormatInt(bytes/ti, 10) + "Ti"
	case bytes != 0 && bytes%gi == 0:
		return strconv.FormatInt(bytes/gi, 10) + "Gi"
	case bytes != 0 && bytes%mi == 0:
		return strconv.FormatInt(bytes/mi, 10) + "Mi"
	case bytes != 0 && bytes%ki == 0:
		return strconv.FormatInt(bytes/ki, 10) + "Ki"
	default:
		return strconv.FormatInt(bytes, 10) + " bytes"
	}
}

func formatInt(n int64) string { return strconv.FormatInt(n, 10) }
