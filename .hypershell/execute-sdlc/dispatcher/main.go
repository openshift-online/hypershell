package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

func main() { os.Exit(runMain()) }
func runMain() int {
	if len(os.Args) > 1 && os.Args[1] == "--worker" {
		return workerMain(os.Args[2:])
	}
	path := os.Getenv("FUNNEL_ADAPTER")
	if path == "" {
		fmt.Fprintln(os.Stderr, "Run .hypershell/execute-sdlc/funnel-dispatch.sh so the integration adapter can be located.")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	a := Adapter{path, os.Args[1:]}
	for _, arg := range a.Args {
		if arg == "--help" || arg == "-h" {
			cmd := a.command(ctx, "config")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return 2
			}
			return 0
		}
	}
	c, err := loadConfig(ctx, a)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err = c.dependencies(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	store, err := openStore(filepath.Join(c.StateDir, "state.sqlite3"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer store.db.Close()
	if c.RetryFailed != "" {
		if err := store.retryFailed(c.RetryFailed, c.Scope); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Retry requested for", c.RetryFailed)
		fmt.Println("The running dispatcher will pick it up when eligible. If none is running, start it with --watch.")
		return 0
	}
	if c.Status {
		rows, err := store.rows()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if os.Getenv("FULLSEND_HUMAN_OUTPUT") == "1" {
			writeStatusReport(os.Stdout, c, rows)
			return 0
		}
		encoder := json.NewEncoder(os.Stdout)
		for _, r := range rows {
			if err = encoder.Encode(r); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		return 0
	}
	if os.Getenv("GH_TOKEN") == "" {
		cmd := contextCommand(ctx, "gh", "auth", "token")
		data, err := cmd.Output()
		if err != nil {
			fmt.Fprintln(os.Stderr, "GitHub authentication failed. Run gh auth login.")
			return 1
		}
		os.Setenv("GH_TOKEN", strings.TrimSpace(string(data)))
	}
	engine, err := newEngine(c, a, store)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer engine.diagnostic.Close()
	done := make(chan int, 1)
	go func() { done <- engine.run(ctx) }()
	interactive := !c.Plain && !c.DryRun && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
	code := 0
	if interactive {
		_, noColor := os.LookupEnv("NO_COLOR")
		model := Model{engine: engine, ctx: ctx, cancel: cancel, width: 80, height: 24, plainColor: noColor, state: engine.snapshot()}
		program := tea.NewProgram(model, tea.WithOutput(os.Stderr), tea.WithAltScreen())
		final, runErr := program.Run()
		cancel()
		code = <-done
		if runErr != nil {
			fmt.Fprintln(os.Stderr, runErr)
			if code == 0 {
				code = 1
			}
		}
		if m, ok := final.(Model); ok && m.exit == 130 {
			code = 130
		}
	} else {
		ticker := time.NewTicker(400 * time.Millisecond)
		defer ticker.Stop()
		last := ""
	loop:
		for {
			select {
			case code = <-done:
				break loop
			case <-ticker.C:
				s := engine.snapshot()
				summary := fmt.Sprintf("%s | %d/%d running", s.Connection, s.Active, c.Concurrency)
				if s.Monitoring {
					summary = "Monitoring existing work. Scheduling resumes automatically when the other owner finishes."
				}
				for _, r := range s.Rows {
					if r.Scope == c.Scope {
						summary += fmt.Sprintf("\n%s | %s | %s", title(r), agentName(r.Agent), r.Status)
					}
				}
				if len(s.Errors) > 0 {
					summary += "\n" + strings.Join(s.Errors, "\n")
				}
				if summary != last {
					fmt.Fprintln(os.Stderr, summary)
					last = summary
				}
			}
		}
	}
	if code == 130 {
		fmt.Fprintln(os.Stderr, "Detached. Workers keep running. Restart this command to resume.")
	}
	if code != 0 && code != 130 {
		for _, problem := range engine.snapshot().Errors {
			fmt.Fprintln(os.Stderr, clean(problem))
		}
		fmt.Fprintln(os.Stderr, "Run needs attention. Details:", filepath.Join(c.LogDir, "dispatcher.log"))
	}
	if err = store.exitLogs(c.Scope); err != nil {
		fmt.Fprintln(os.Stderr, "Could not read log paths:", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}
