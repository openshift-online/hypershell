package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	RetryFailed string             `json:"retry"`
	Scope       string             `json:"scope"`
	Watch       bool               `json:"watch"`
	DryRun      bool               `json:"dry_run"`
	Plain       bool               `json:"plain"`
	Status      bool               `json:"status"`
	Retry       string             `json:"retry_uncertain"`
	Concurrency int                `json:"concurrency"`
	Poll        int                `json:"poll_interval"`
	AuthRetry   int                `json:"auth_retry"`
	AuthCheck   int                `json:"auth_check_interval"`
	Backoff     int                `json:"backoff"`
	BackoffMax  int                `json:"backoff_max"`
	Lifecycle   string             `json:"lifecycle"`
	Repo        string             `json:"repo"`
	FullsendDir string             `json:"fullsend_dir"`
	TargetRepo  string             `json:"target_repo"`
	StateDir    string             `json:"state_dir"`
	LogDir      string             `json:"log_dir"`
	Model       string             `json:"model"`
	GatewayName string             `json:"gateway_name"`
	Jira        json.RawMessage    `json:"jira_selection"`
	Environment map[string]*string `json:"environment"`
}
type Adapter struct {
	Path string
	Args []string
}

// Cancel the entire helper process group, including gh/acli child commands.
// Detached Fullsend workers never use this context.
func contextCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	return cmd
}
func (a Adapter) command(ctx context.Context, action string) *exec.Cmd {
	args := []string{a.Path, action}
	if action != "snapshot" {
		args = append(args, a.Args...)
	}
	if action == "gateway" {
		// Bubble Tea has released the terminal for this prompt. Keep the helper
		// in our foreground process group: Setpgid would stop input with SIGTTIN.
		cmd := exec.CommandContext(ctx, "python3", args...)
		cmd.WaitDelay = 2 * time.Second
		return cmd
	}
	return contextCommand(ctx, "python3", args...)
}
func (a Adapter) call(ctx context.Context, action string, input any, result any) error {
	cmd := a.command(ctx, action)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		cmd.Stdin = bytes.NewReader(b)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", action, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(out.Bytes(), result); err != nil {
		return fmt.Errorf("%s returned invalid JSON: %w", action, err)
	}
	return nil
}
func loadConfig(ctx context.Context, a Adapter) (Config, error) {
	var c Config
	err := a.call(ctx, "config", nil, &c)
	if err != nil {
		return c, err
	}
	for k, v := range c.Environment {
		if v == nil {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, *v)
		}
	}
	return c, nil
}
func (c Config) dependencies() error {
	if c.Status || c.RetryFailed != "" {
		return nil
	}
	names := []string{"gh", "fullsend", "git"}
	if !c.DryRun {
		names = append(names, "openshell")
		if c.Lifecycle == "jira" {
			names = append(names, "jq")
		}
	}
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("install %s before starting the dispatcher", name)
		}
	}
	return nil
}
func workerMain(args []string) int {
	if len(args) != 7 {
		fmt.Fprintln(os.Stderr, "invalid worker arguments")
		return 127
	}
	// db, key, run, backoff, cap, adapter, repo-root. fd 3 is the inherited run lock.
	s, err := openStore(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 127
	}
	defer s.db.Close()
	var payload string
	if err = s.db.QueryRow(`SELECT payload FROM jobs WHERE key=? AND run_id=?`, args[1], args[2]).Scan(&payload); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 127
	}
	var job Job
	if err = json.Unmarshal([]byte(payload), &job); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 127
	}
	base, cap := 0, 0
	if _, err = fmt.Sscan(args[3], &base); err != nil {
		return 127
	}
	if _, err = fmt.Sscan(args[4], &cap); err != nil {
		return 127
	}
	lock := os.NewFile(3, "run-lock")
	defer lock.Close()
	rc := runWorker(job, Adapter{Path: args[5]}, args[6], lock)
	if err = s.finishReliably(job.Key, args[2], rc, base, cap); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 127
	}
	return rc
}
func runWorker(job Job, a Adapter, root string, lock *os.File) int {
	env := map[string]string{"FULLSEND_WORK_ITEM_URL": job.URL, "ISSUE_NUMBER": job.Item, "REPOSITORY": job.Repo, "REPO_FULL_NAME": job.Repo, "LABEL_SCOPE": job.Scope, "DRY_RUN": envDefault("AGENT_DRY_RUN", "false"), "PR_NUMBER": job.PRNumber, "PR_URL": job.PRURL, "FUNNEL_PREVIOUS_PHASE": job.PreviousPhase, "FUNNEL_REPO_ROOT": root}
	if job.Agent == "funnel-ingest" || job.Agent == "funnel-reconcile" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		var credentials json.RawMessage
		if err := a.call(ctx, "snapshot", map[string]any{"selection": job.Jira, "scope": job.Scope}, &credentials); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 127
		}
		file, err := os.CreateTemp("", "funnel-jira-*.json")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 127
		}
		defer os.Remove(file.Name())
		_, writeErr := file.Write(credentials)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			fmt.Fprintln(os.Stderr, "cannot write private Jira credential snapshot")
			return 127
		}
		env["FUNNEL_JIRA_CREDENTIALS_FILE"] = file.Name()
	}
	if len(job.Command) == 0 {
		fmt.Fprintln(os.Stderr, "missing worker command")
		return 127
	}
	command := append([]string(nil), job.Command...)
	for i := 1; i+1 < len(command); i++ {
		if command[i] != "--target-repo" {
			continue
		}
		fmt.Fprintln(os.Stdout, "Preparing an independent Git checkout with local edits")
		checkout, cleanup, err := prepareCheckout(command[i+1], job.Repo)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 127
		}
		defer cleanup()
		command[i+1] = checkout
		break
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = mergeEnv(os.Environ(), env)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{lock}
	if err := cmd.Run(); err != nil {
		if code := cmd.ProcessState; code != nil && code.ExitCode() > 0 {
			return code.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 127
	}
	return 0
}
func mergeEnv(base []string, values map[string]string) []string {
	out := []string{}
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := values[key]; !ok {
			out = append(out, entry)
		}
	}
	for k, v := range values {
		out = append(out, k+"="+v)
	}
	return out
}
func envDefault(k, d string) string {
	if value := os.Getenv(k); value != "" {
		return value
	}
	return d
}
func (s *Store) launch(c Config, a Adapter, j Job) error {
	j.Repo = c.Repo
	j.Command = []string{"fullsend", "run", j.Agent, "--fullsend-dir", c.FullsendDir, "--target-repo", c.TargetRepo}
	if c.Model != "" {
		j.Command = append(j.Command, "--model", c.Model)
	}
	if j.Agent == "funnel-ingest" || j.Agent == "funnel-reconcile" {
		j.Jira = c.Jira
	}
	log := filepath.Join(c.LogDir, fmt.Sprintf("%s-%s-%d.log", j.Agent, safeName(j.Item), time.Now().UnixNano()))
	id, err := s.start(j, log)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		if finishErr := s.finish(j.Key, id, 127, c.Backoff, c.BackoffMax); finishErr != nil {
			return fmt.Errorf("%v; %w", err, finishErr)
		}
		return err
	}
	lock, err := s.runLock(id)
	if err != nil {
		return fail(err)
	}
	defer lock.Close()
	out, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fail(err)
	}
	defer out.Close()
	executable, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(a.Path)))
	cmd := exec.Command(executable, "--worker", s.path, j.Key, id, fmt.Sprint(c.Backoff), fmt.Sprint(c.BackoffMax), a.Path, root)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.ExtraFiles = []*os.File{lock}
	cmd.Stdout = out
	cmd.Stderr = out
	if err = cmd.Start(); err != nil {
		return fail(err)
	}
	go func() { _ = cmd.Wait() }() // Outcome is persisted by the detached worker, not this waiter.
	return nil
}
func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r) {
			return r
		}
		return '_'
	}, s)
}
