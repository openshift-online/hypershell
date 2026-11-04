package main

import (
	"strings"
	"testing"
)

func waitingReview() Row {
	r := Row{Job: Job{Key: "832:code-review", Scope: "agent/scope/test", Agent: "code-review", Item: "832", Title: "RoleBinding authorization", PRURL: "https://github.com/test/repo/pull/834", Labels: []string{"agent/reviewable-code"}, CodeReadiness: &CodeReadiness{State: "open", Checks: "pending"}}, Status: "succeeded", Finished: seconds() - 60}
	return r
}

// Verifies: SDLC-LOC-09.01, SDLC-HOOK-14.01
func TestDeferredReviewRemainsVisibleAcrossViews(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	r := waitingReview()
	m.state.Rows = []Row{r}
	m.selected = r.Key
	m.index = 0
	m.height = 40
	for _, screen := range []string{"", "logs", "details"} {
		m.screen = screen
		if len(m.visibleRows()) != 1 {
			t.Fatalf("%s hid deferred review", screen)
		}
		view := m.View()
		if !strings.Contains(view, "Waiting for CI") {
			t.Fatalf("%s: %s", screen, view)
		}
		if screen == "details" && (!strings.Contains(view, "Approval status") || !strings.Contains(strings.Join(strings.Fields(view), " "), "Approval has not been applied")) {
			t.Fatal(view)
		}
		if screen == "" && !strings.Contains(view, "0 completed runs") {
			t.Fatal(view)
		}
	}
	if rowColor(r) != "3" {
		t.Fatal("waiting review must not use success color")
	}
	m.historyIssue = &r
	r.Activity = "history"
	m.historyRows = []Row{r}
	m.screen = ""
	if strings.Contains(rowStatus(r), "Waiting") {
		t.Fatal("history confused current readiness with attempt outcome")
	}
}

func TestReviewWaitingReflectsRequiredCIAndPRReadiness(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		checks      string
		draft       bool
		mergeable   *bool
		state, want string
	}{
		{"pending", false, &yes, "clean", "Waiting for CI"},
		{"failing", false, &yes, "blocked", "CI failed · needs repair"},
		{"green", true, &yes, "clean", "Waiting: PR is draft"},
		{"green", false, nil, "unknown", "Waiting for PR readiness"},
		{"green", false, &no, "dirty", "Conflicts · needs repair"},
		{"green", false, &yes, "clean", "Approval pending"},
	} {
		r := waitingReview()
		r.CodeReadiness.Checks = tc.checks
		r.CodeReadiness.Draft = tc.draft
		r.CodeReadiness.Mergeable = tc.mergeable
		r.CodeReadiness.MergeableState = tc.state
		if got := rowStatus(r); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
		r.Labels = []string{"agent/review-code-rejected"}
		if reviewWaiting(r) != "" {
			t.Fatal("superseded review still waits")
		}
		r.CodeReadiness = nil
		if !strings.HasPrefix(rowStatus(r), "Run completed") {
			t.Fatal("legacy payload should remain readable")
		}
	}
}

func TestFailedCIHandoffIsExplicitInCompletedReview(t *testing.T) {
	r := waitingReview()
	r.CodeReadiness.Checks = "failing"
	r.Labels = []string{"agent/review-code-rejected"}
	if rowStatus(r) != "CI failed · rejected" || rowColor(r) != "1" || reviewWaiting(r) != "" {
		t.Fatalf("bad handoff: %s", rowStatus(r))
	}
	if !strings.Contains(reviewWaitingDetail(r), "returned to code implementation") {
		t.Fatal("missing next step")
	}
}
