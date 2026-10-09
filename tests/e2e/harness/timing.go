package harness

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type timingResult struct {
	id       string
	slug     string
	duration time.Duration
	outcome  string
}

// TimingReporter records phase wall times from serial or parallel subtests and
// renders them in canonical phase order.
type TimingReporter struct {
	mu      sync.Mutex
	started time.Time
	results map[string]timingResult
}

func NewTimingReporter(started time.Time) *TimingReporter {
	return &TimingReporter{started: started, results: make(map[string]timingResult)}
}

func (r *TimingReporter) Record(id, slug string, duration time.Duration, outcome string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[id] = timingResult{id: id, slug: slug, duration: duration, outcome: outcome}
}

func (r *TimingReporter) Summary(finished time.Time) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, 0, len(r.results))
	for id := range r.results {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	b.WriteString("\n==================== E2E TIMING ====================\n")
	for _, id := range ids {
		res := r.results[id]
		fmt.Fprintf(&b, "  %-5s %-32s %8s  %s\n", res.id, res.slug, formatDuration(res.duration), res.outcome)
	}
	b.WriteString("----------------------------------------------------\n")
	fmt.Fprintf(&b, "  %-38s %8s\n", "total", formatDuration(finished.Sub(r.started)))
	b.WriteString("====================================================\n")
	return b.String()
}

// AppendGitHubStepSummary adds the timing table to the current Actions step
// summary when GITHUB_STEP_SUMMARY is available. Local runs remain log-only.
func AppendGitHubStepSummary(title, summary string) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open GitHub step summary: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintf(f, "\n### %s\n\n```text\n%s```\n", title, strings.TrimPrefix(summary, "\n")); err != nil {
		return fmt.Errorf("write GitHub step summary: %w", err)
	}
	return nil
}

func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%.1fs", d.Seconds())
}
