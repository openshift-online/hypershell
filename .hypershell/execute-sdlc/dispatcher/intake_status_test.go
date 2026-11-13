package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Verifies: SDLC-LOC-09.03
func TestPendingIntakeIsInformationalInBothListsAndDetails(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.state.Rows = nil
	m.state.WaitingForTriage = []WaitingIssue{{Number: 708, Title: "Fix reconciliation errors"}}
	for _, grouped := range []bool{false, true} {
		m.issueGrouping = grouped
		m.screen = ""
		view := m.View()
		if !strings.Contains(view, "1 issue waiting for triage") || strings.Contains(view, "need attention") {
			t.Fatal(view)
		}
		m.screen = "details"
		view = strings.Join(strings.Fields(m.View()), " ")
		for _, want := range []string{"Waiting for triage", "No additional labels are needed", "#708"} {
			if !strings.Contains(view, want) {
				t.Fatalf("missing %q: %s", want, view)
			}
		}
		if strings.Contains(view, "Needs attention") {
			t.Fatal("normal intake is an alert")
		}
	}
	m.state.Errors = []string{"Jira authentication failed"}
	view := m.View()
	if !strings.Contains(view, "Needs attention") || !strings.Contains(view, "Jira authentication failed") || !strings.Contains(view, "Waiting for triage") {
		t.Fatal("real error was hidden or combined with normal waiting")
	}
	m.state.WaitingForTriage = append(m.state.WaitingForTriage, WaitingIssue{Number: 709})
	m.screen = ""
	if !strings.Contains(m.View(), "2 issues waiting for triage") {
		t.Fatal("plural count missing")
	}
}

func TestWaitingDiscoveryContractAndSnapshotIsolation(t *testing.T) {
	var result discovery
	if err := json.Unmarshal([]byte(`{"jobs":[],"errors":[],"waiting_for_triage":[{"number":708,"title":"Queued issue","url":"https://github.com/test/repo/issues/708"}]}`), &result); err != nil {
		t.Fatal(err)
	}
	e := Engine{state: Snapshot{WaitingForTriage: result.WaitingForTriage}}
	snapshot := e.snapshot()
	snapshot.WaitingForTriage[0].Title = "changed"
	if e.snapshot().WaitingForTriage[0].Title != "Queued issue" {
		t.Fatal("snapshot changed shared queue data")
	}
}
