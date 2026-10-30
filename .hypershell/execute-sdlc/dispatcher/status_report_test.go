package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestStatusReportScopesAndSummarizes(t *testing.T) {
	c := Config{Scope: "agent/scope/test", Repo: "test/repo"}
	rows := []Row{
		{Job: Job{Scope: c.Scope, Item: "1", Title: "Current work", Agent: "code-implementation", Fingerprint: "INTERNAL", Command: []string{"DO_NOT_PRINT"}}, Status: "running", Started: seconds() - 60},
		{Job: Job{Scope: c.Scope, Item: "2", Title: "Previous work"}, Status: "succeeded"},
		{Job: Job{Scope: "another/scope", Title: "OTHER_SCOPE"}, Status: "failed"},
		{Job: Job{Scope: c.Scope, Item: "3", Agent: "code-review", Title: "Needs review"}, Status: "failed", Error: "Validation failed", Log: "/tmp/full path/review.log"},
	}
	var out bytes.Buffer
	writeStatusReport(&out, c, rows)
	for _, want := range []string{"1 running", "1 failed", "1 completed jobs", "Current work", "Validation failed", "/tmp/full path/review.log", "make fullsend"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, bad := range []string{"INTERNAL", "DO_NOT_PRINT", "OTHER_SCOPE", "Previous work"} {
		if strings.Contains(out.String(), bad) {
			t.Fatalf("leaked %q", bad)
		}
	}
	out.Reset()
	writeStatusReport(&out, c, nil)
	if !strings.Contains(out.String(), "No saved jobs") {
		t.Fatal("missing empty-state explanation")
	}
}
