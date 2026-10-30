package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Snapshot struct {
	ConnectionError              string
	RecoveryError                string
	Rows                         []Row
	Gateway, Connection, Message string
	Errors                       []string
	Busy, Monitoring, Recovering bool
	Active, Ready                int
	NextDiscovery                time.Time
	Done                         bool
	Exit                         int
}
type discovery struct {
	Jobs   []Job    `json:"jobs"`
	Errors []string `json:"errors"`
	Err    error
}
type authResult struct{ Err error }
type recoveryRequest struct {
	Done   chan error
	Finish bool
	Config Config
	Err    error
}
type retryRequest struct {
	RunID string
	Key   string
	Done  chan error
}
type Engine struct {
	retries    chan retryRequest
	c          Config
	a          Adapter
	store      *Store
	mu         sync.RWMutex
	state      Snapshot
	authMu     sync.Mutex
	authWake   chan struct{}
	recovery   chan recoveryRequest
	diagnostic *os.File
}

func newEngine(c Config, a Adapter, s *Store) (*Engine, error) {
	if err := os.MkdirAll(c.LogDir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(c.LogDir, "dispatcher.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &Engine{retries: make(chan retryRequest), c: c, a: a, store: s, authWake: make(chan struct{}, 1), recovery: make(chan recoveryRequest), diagnostic: f, state: Snapshot{Gateway: c.GatewayName, Connection: "Checking connection"}}, nil
}
func (e *Engine) snapshot() Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s := e.state
	s.Rows = append([]Row(nil), s.Rows...)
	s.Errors = append([]string(nil), s.Errors...)
	return s
}
func (e *Engine) update(fn func(*Snapshot)) { e.mu.Lock(); defer e.mu.Unlock(); fn(&e.state) }
func (e *Engine) note(message string) {
	fmt.Fprintf(e.diagnostic, "%s %s\n", time.Now().Format(time.RFC3339), message)
}
func (e *Engine) fail(err error) {
	e.note(err.Error())
	e.update(func(s *Snapshot) {
		s.Errors = append(s.Errors, err.Error())
		if len(s.Errors) > 8 {
			s.Errors = s.Errors[len(s.Errors)-8:]
		}
	})
}
func (e *Engine) publishRows() ([]Row, error) {
	rows, err := e.store.rows()
	if err != nil {
		return nil, err
	}
	active := 0
	for i := range rows {
		if rows[i].Status == "running" {
			active++
			rows[i].Activity = activity(rows[i].Log)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rank(rows[i].Status) < rank(rows[j].Status) })
	e.update(func(s *Snapshot) { s.Rows = rows; s.Active = active })
	return rows, nil
}
func rank(status string) int {
	switch status {
	case "running":
		return 0
	case "uncertain":
		return 1
	case "failed":
		return 2
	default:
		return 3
	}
}
func (e *Engine) authLoop(ctx context.Context, ch chan<- authResult) {
	delay := time.Duration(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		case <-e.authWake:
		}
		if e.snapshot().Recovering {
			delay = time.Second
			continue
		}
		e.authMu.Lock()
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		cmd := contextCommand(checkCtx, "openshell", "whoami", "--output", "json")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		cancel()
		e.authMu.Unlock()
		if err != nil {
			err = fmt.Errorf("Gateway check failed: %s", strings.TrimSpace(stderr.String()))
			delay = time.Duration(e.c.AuthRetry) * time.Second
		} else {
			delay = time.Duration(e.c.AuthCheck) * time.Second
		}
		select {
		case ch <- authResult{err}:
		case <-ctx.Done():
			return
		}
	}
}
func (e *Engine) requestRetry(ctx context.Context, key string, confirmedRun ...string) error {
	runID := ""
	if len(confirmedRun) > 0 {
		runID = confirmedRun[0]
	}
	done := make(chan error, 1)
	select {
	case e.retries <- retryRequest{Key: key, Done: done, RunID: runID}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (e *Engine) beginGateway(ctx context.Context) error {
	done := make(chan error, 1)
	select {
	case e.recovery <- recoveryRequest{Done: done}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (e *Engine) finishGateway(ctx context.Context, c Config, err error) {
	select {
	case e.recovery <- recoveryRequest{Finish: true, Config: c, Err: err}:
	case <-ctx.Done():
	}
}
func (e *Engine) run(ctx context.Context) (exit int) {
	defer func() { e.update(func(s *Snapshot) { s.Done = true; s.Exit = exit }) }()
	lockPath := filepath.Join(e.c.StateDir, "dispatcher.lock")
	var lock *os.File
	for {
		var err error
		lock, err = lockFile(lockPath)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			e.fail(err)
			return 1
		}
		e.update(func(s *Snapshot) {
			s.Monitoring = true
			s.Message = "Monitoring existing work. Scheduling resumes when the other owner finishes."
		})
		if _, err = e.publishRows(); err != nil {
			e.fail(err)
			return 1
		}
		select {
		case <-ctx.Done():
			return 130
		case <-time.After(500 * time.Millisecond):
		}
	}
	defer lock.Close()
	e.update(func(s *Snapshot) { s.Monitoring = false; s.Message = "Checking for work" })
	if err := e.store.recover(); err != nil {
		e.fail(err)
		return 1
	}
	if e.c.Retry != "" {
		if err := e.store.retry(e.c.Retry); err != nil {
			e.fail(err)
			return 1
		}
	}
	if _, err := e.publishRows(); err != nil {
		e.fail(err)
		return 1
	}
	// Authentication and discovery have independent cancellation and schedules.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	auth := make(chan authResult, 1)
	discovered := make(chan discovery, 1)
	if !e.c.DryRun {
		go e.authLoop(ctx, auth)
	}
	connected := e.c.DryRun
	candidates := []Job{}
	attempted := map[string]bool{}
	inFlight := false
	next := time.Time{}
	haveDiscovery := false
	lastRunning := map[string]string{}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return 130
		case request := <-e.retries:
			var err error
			if request.RunID != "" {
				err = e.store.retryConfirmedUnknown(request.Key, e.c.Scope, request.RunID)
			} else {
				err = e.store.retryFailed(request.Key, e.c.Scope)
			}
			if err == nil {
				for key := range attempted {
					if key == request.Key || strings.HasPrefix(key, request.Key+"/") {
						delete(attempted, key)
					}
				}
				next = time.Time{}
				e.note("Manual retry requested: " + request.Key)
			}
			request.Done <- err
		case request := <-e.recovery:
			if request.Finish {
				if request.Err != nil {
					e.fail(request.Err)
					e.update(func(s *Snapshot) { s.RecoveryError = request.Err.Error() })
				} else {
					e.c.GatewayName = request.Config.GatewayName
					e.update(func(s *Snapshot) { s.RecoveryError = "" })
				}
				e.update(func(s *Snapshot) {
					s.Recovering = false
					s.Gateway = e.c.GatewayName
					s.Connection = "Checking connection"
				})
				connected = false
				select {
				case e.authWake <- struct{}{}:
				default:
				}
			} else {
				if e.snapshot().Active != 0 {
					request.Done <- fmt.Errorf("workers are running; reconnect after they finish to preserve their gateway")
					continue
				}
				connected = false
				e.update(func(s *Snapshot) { s.Recovering = true; s.Connection = "Signing in" })
				request.Done <- nil
			}
		case result := <-auth:
			if e.snapshot().Recovering {
				continue
			}
			connected = result.Err == nil
			if result.Err != nil {
				e.note(result.Err.Error())
				e.update(func(s *Snapshot) {
					s.Connection = "Connection lost"
					s.ConnectionError = result.Err.Error()
					s.Message = "New work is paused. Press s to sign in or check the connection."
				})
			} else {
				e.update(func(s *Snapshot) { s.Connection = "Connected"; s.Message = ""; s.ConnectionError = "" })
			}
		case result := <-discovered:
			inFlight = false
			haveDiscovery = true
			next = time.Now().Add(time.Duration(e.c.Poll) * time.Second)
			candidates = result.Jobs
			if result.Err != nil {
				candidates = nil
				e.fail(result.Err)
			}
			e.update(func(s *Snapshot) {
				s.Busy = false
				s.NextDiscovery = next
				if result.Err == nil {
					s.Errors = result.Errors
				}
			})
			if e.c.DryRun {
				for _, job := range candidates {
					fmt.Printf("WOULD run %s %s\n", job.Key, job.URL)
				}
				if result.Err != nil || len(result.Errors) > 0 {
					return 1
				}
				return 0
			}
		case <-ticker.C:
		}
		if err := e.store.recover(); err != nil {
			e.fail(err)
			return 1
		}
		rows, err := e.publishRows()
		if err != nil {
			e.fail(err)
			return 1
		}
		running := map[string]string{}
		for _, row := range rows {
			if row.Status == "running" {
				running[row.Key] = row.RunID
			}
		}
		completed := false
		for key := range lastRunning {
			if _, ok := running[key]; !ok {
				completed = true
			}
		}
		lastRunning = running
		if completed && (e.c.Watch || e.c.Lifecycle == "jira") {
			next = time.Time{}
		}
		if !inFlight && (next.IsZero() || e.c.Watch && time.Now().After(next)) {
			inFlight = true
			e.update(func(s *Snapshot) { s.Busy = true })
			go func() {
				var result discovery
				callCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				result.Err = e.a.call(callCtx, "discover", nil, &result)
				select {
				case discovered <- result:
				case <-ctx.Done():
				}
			}()
		}
		if inFlight || !haveDiscovery || e.snapshot().Recovering {
			continue
		}
		// Oldest attempt first. Unchanged successful work is never launched again.
		started := map[string]float64{}
		for _, row := range rows {
			started[row.Key] = row.Started
		}
		sort.SliceStable(candidates, func(i, j int) bool { return started[candidates[i].Key] < started[candidates[j].Key] })
		ready := 0
		pending := false
		for _, job := range candidates {
			attempt := job.Key
			if job.Agent == "funnel-reconcile" {
				attempt += "/" + job.Fingerprint
			}
			if !e.c.Watch && attempted[attempt] {
				continue
			}
			if !eligible(job, rows, seconds()) {
				continue
			}
			ready++
			pending = true
			if len(running) >= e.c.Concurrency || !connected {
				continue
			}
			var allowed struct {
				Allowed bool `json:"allowed"`
			}
			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			err = e.a.call(checkCtx, "authorize", job, &allowed)
			cancel()
			if ctx.Err() != nil {
				return 130
			}
			if err != nil {
				e.fail(err)
				candidates = nil
				next = time.Now().Add(time.Duration(e.c.Poll) * time.Second)
				attempted[attempt] = true
				break
			}
			if !allowed.Allowed {
				next = time.Time{}
				continue
			}
			// Refresh immediately before each launch as well as in the independent timer.
			e.authMu.Lock()
			checkCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
			cmd := contextCommand(checkCtx, "openshell", "whoami", "--output", "json")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err = cmd.Run()
			cancel()
			e.authMu.Unlock()
			if err != nil {
				connected = false
				e.note(stderr.String())
				e.update(func(s *Snapshot) { s.Connection = "Connection lost" })
				break
			}
			if ctx.Err() != nil {
				return 130
			}
			if err = e.store.launch(e.c, e.a, job); err != nil {
				e.fail(err)
				return 1
			}
			attempted[attempt] = true
			running[job.Key] = "started"
			lastRunning[job.Key] = "started"
			rows, err = e.publishRows()
			if err != nil {
				e.fail(err)
				return 1
			}
		}
		e.update(func(s *Snapshot) { s.Ready = ready })
		if !e.c.Watch && len(running) == 0 && !pending && !next.IsZero() {
			if len(e.snapshot().Errors) > 0 {
				return 1
			}
			for _, row := range rows {
				if row.Scope == e.c.Scope && (row.Status == "failed" || row.Status == "uncertain") {
					return 1
				}
			}
			return 0
		}
	}
}
