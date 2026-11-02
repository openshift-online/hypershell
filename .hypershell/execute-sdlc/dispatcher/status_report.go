package main

import (
	"fmt"
	"io"
	"sort"
)

// Make's status command is a readable view; direct --status retains JSON output.
func writeStatusReport(out io.Writer, c Config, rows []Row) {
	counts := map[string]int{}
	visible := []Row{}
	for _, r := range rows {
		if r.Scope != c.Scope {
			continue
		}
		counts[r.Status]++
		if r.Status != "succeeded" {
			visible = append(visible, r)
		}
	}
	fmt.Fprintf(out, "FULLSEND / Status\n%s · %s\n\n", clean(c.Scope), clean(c.Repo))
	fmt.Fprintf(out, "%d running · %d failed · %d unknown outcomes · %d completed jobs\n", counts["running"], counts["failed"], counts["uncertain"], counts["succeeded"])
	fmt.Fprintln(out, "Saved local state; gateway connectivity is not checked.")
	sort.SliceStable(visible, func(i, j int) bool { return rank(visible[i].Status) < rank(visible[j].Status) })
	if len(visible) > 0 {
		fmt.Fprintln(out, "\n"+cell("ISSUE / TITLE", 44)+"  "+cell("AGENT", 24)+"  "+cell("STATUS", 24)+"  DURATION")
		for _, r := range visible {
			label := r.Status
			switch r.Status {
			case "running":
				label = "Running"
			case "failed":
				label = "Failed · " + rowStatus(r)
			case "uncertain":
				label = "Outcome unknown"
			}
			fmt.Fprintln(out, cell(clean(title(r)), 44)+"  "+cell(clean(r.Agent), 24)+"  "+cell(label, 24)+"  "+durationLabel(r))
		}
	} else if len(counts) == 0 {
		fmt.Fprintln(out, "\nNo saved jobs for this scope yet.")
	} else {
		fmt.Fprintln(out, "\nNo running or failed jobs to inspect.")
	}
	for _, r := range visible {
		if r.Status != "failed" && r.Status != "uncertain" {
			continue
		}
		fmt.Fprintf(out, "\n%s · %s\n", clean(title(r)), clean(r.Agent))
		if r.Error != "" {
			fmt.Fprintln(out, "  "+middle(clean(r.Error), 160))
		}
		if r.Log != "" {
			fmt.Fprintln(out, "  Log: "+clean(r.Log))
		}
	}
	fmt.Fprintln(out, "\nNext: make fullsend opens the dashboard. Enter views logs; H shows history; c shows completed jobs.")
}
