package harness

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Outcome is a per-step result. The suite distinguishes three outcomes and never
// reports a skip as a pass.
type Outcome int

const (
	OutcomePass Outcome = iota
	OutcomeFail
	OutcomeSkip
)

func (o Outcome) String() string {
	switch o {
	case OutcomePass:
		return "PASS"
	case OutcomeFail:
		return "FAIL"
	case OutcomeSkip:
		return "SKIP"
	default:
		return "UNKNOWN"
	}
}

type stepResult struct {
	id      string
	slug    string
	outcome Outcome
	detail  string
}

// Reporter collects per-step outcomes and prints the final summary from
// TearDownSuite, equivalent to the Bash print_results. It is safe for concurrent
// use because P2/P3 steps record their outcomes from parallel subtests. The
// summary lists steps in canonical phase order (P0.1, P0.2, P1.1, ...) regardless
// of the order parallel subtests complete in.
type Reporter struct {
	mu      sync.Mutex
	results map[string]stepResult
}

// NewReporter returns an empty reporter.
func NewReporter() *Reporter {
	return &Reporter{results: make(map[string]stepResult)}
}

// Record stores a step's outcome. A later record for the same id overwrites the
// earlier one (a step records exactly once per run).
func (r *Reporter) Record(id, slug string, o Outcome, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[id] = stepResult{id: id, slug: slug, outcome: o, detail: detail}
}

// Counts returns the pass/fail/skip totals.
func (r *Reporter) Counts() (pass, fail, skip int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, res := range r.results {
		switch res.outcome {
		case OutcomePass:
			pass++
		case OutcomeFail:
			fail++
		case OutcomeSkip:
			skip++
		}
	}
	return pass, fail, skip
}

// Summary renders the roll-up in canonical phase order so a reader sees the
// result without parsing go test output.
func (r *Reporter) Summary() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, 0, len(r.results))
	for id := range r.results {
		ids = append(ids, id)
	}
	sort.Strings(ids) // P0.1 < P0.2 < P1.1 < ... for single-digit phase/step ids

	var b strings.Builder
	var pass, fail, skip int
	b.WriteString("\n==================== E2E RESULTS ====================\n")
	for _, id := range ids {
		res := r.results[id]
		switch res.outcome {
		case OutcomePass:
			pass++
		case OutcomeFail:
			fail++
		case OutcomeSkip:
			skip++
		}
		line := fmt.Sprintf("  [%s] %s %s", res.outcome, res.id, res.slug)
		if res.detail != "" {
			line += " -- " + res.detail
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("-----------------------------------------------------\n")
	fmt.Fprintf(&b, "  %d passed, %d failed, %d skipped\n", pass, fail, skip)
	b.WriteString("=====================================================\n")
	return b.String()
}
