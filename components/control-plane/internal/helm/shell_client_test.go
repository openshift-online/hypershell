package helm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHelm is a stand-in helm binary. It records its arguments, marks when it
// starts and finishes, sleeps FAKE_HELM_SLEEP seconds, and records a SIGTERM
// (or ignores it when FAKE_HELM_IGNORE_TERM is set). The sleep runs in the
// background with its output detached so the trap fires promptly and the
// process's stdout pipe closes when the script exits.
const fakeHelm = `#!/bin/sh
dir="$FAKE_HELM_DIR"
printf '%s\n' "$@" > "$dir/args"
if [ -n "$FAKE_HELM_IGNORE_TERM" ]; then
  trap 'echo TERM >> "$dir/signal"' TERM
else
  trap 'echo TERM >> "$dir/signal"; exit 143' TERM
fi
touch "$dir/started"
end=$(( $(date +%s) + ${FAKE_HELM_SLEEP:-0} ))
while [ "$(date +%s)" -lt "$end" ]; do
  sleep 0.05 >/dev/null 2>&1 </dev/null &
  wait $!
done
echo done > "$dir/finished"
`

func newFakeHelm(t *testing.T, sleepSeconds string) (*ShellClient, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "helm")
	if err := os.WriteFile(bin, []byte(fakeHelm), 0o755); err != nil {
		t.Fatalf("write fake helm: %v", err)
	}
	t.Setenv("FAKE_HELM_DIR", dir)
	t.Setenv("FAKE_HELM_SLEEP", sleepSeconds)
	return &ShellClient{HelmBinary: bin, ChartPath: "/charts/openshell.tgz"}, dir
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func waitFor(t *testing.T, dir, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !exists(dir, name) {
		if time.Now().After(deadline) {
			t.Fatalf("fake helm never wrote %s", name)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Helm shutdown survival: cancelling the controller's context (its shutdown signal)
// while an upgrade runs does not stop helm; the upgrade finishes.
func TestShellClientUpgradeSurvivesParentCancel(t *testing.T) {
	c, dir := newFakeHelm(t, "1")
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- c.Upgrade(ctx, "gw-ns", map[string]interface{}{"a": "b"}) }()

	waitFor(t, dir, "started")
	if got := c.InFlight(); got != 1 {
		t.Fatalf("InFlight during upgrade = %d, want 1", got)
	}
	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("Upgrade after parent cancel: %v", err)
	}
	if !exists(dir, "finished") {
		t.Fatal("helm did not run to completion after the parent context was cancelled")
	}
	if exists(dir, "signal") {
		t.Fatal("helm was signalled by the parent context's cancellation")
	}
	if got := c.InFlight(); got != 0 {
		t.Fatalf("InFlight after upgrade = %d, want 0", got)
	}
}

// Helm shutdown survival: once its own deadline (--timeout plus the margin) passes, helm
// gets SIGTERM, not SIGKILL, and the error says so.
func TestShellClientDeadlineSendsSIGTERM(t *testing.T) {
	c, dir := newFakeHelm(t, "30")
	c.OperationTimeout = 200 * time.Millisecond
	c.DeadlineMargin = 100 * time.Millisecond
	c.KillDelay = 5 * time.Second

	start := time.Now()
	err := c.Install(context.Background(), "gw-ns", map[string]interface{}{})
	if err == nil {
		t.Fatal("Install past its deadline returned nil")
	}
	if !strings.Contains(err.Error(), "SIGTERM") {
		t.Fatalf("error does not report the SIGTERM deadline: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("helm took %s to stop; SIGTERM should end it well before the kill delay", elapsed)
	}
	data, readErr := os.ReadFile(filepath.Join(dir, "signal"))
	if readErr != nil || !strings.Contains(string(data), "TERM") {
		t.Fatalf("fake helm did not receive SIGTERM (signal file: %q, %v)", data, readErr)
	}
	if exists(dir, "finished") {
		t.Fatal("helm finished despite its deadline")
	}
}

// A helm that ignores SIGTERM is killed after KillDelay, so a wedged helm
// cannot hold the controller past its termination grace period.
func TestShellClientKillsAfterKillDelay(t *testing.T) {
	c, dir := newFakeHelm(t, "30")
	t.Setenv("FAKE_HELM_IGNORE_TERM", "1")
	c.OperationTimeout = 200 * time.Millisecond
	c.DeadlineMargin = 100 * time.Millisecond
	c.KillDelay = 300 * time.Millisecond

	start := time.Now()
	if err := c.Uninstall(context.Background(), "gw-ns"); err == nil {
		t.Fatal("Uninstall past its deadline returned nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("helm ignoring SIGTERM ran for %s; want it killed after KillDelay", elapsed)
	}
	waitFor(t, dir, "signal")
	if exists(dir, "finished") {
		t.Fatal("helm finished despite being killed")
	}
}

// A helm operation is not started once the controller is shutting down.
func TestShellClientDoesNotStartAfterCancel(t *testing.T) {
	c, dir := newFakeHelm(t, "0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := c.Upgrade(ctx, "gw-ns", map[string]interface{}{}); err == nil {
		t.Fatal("Upgrade on a cancelled context returned nil")
	}
	if _, err := c.GetReleaseStatus(ctx, "gw-ns"); err == nil {
		t.Fatal("GetReleaseStatus on a cancelled context returned nil")
	}
	if exists(dir, "started") {
		t.Fatal("helm was started after the context was cancelled")
	}
}

// Every mutating command passes helm the operation timeout that the deadline
// and the controller's terminationGracePeriodSeconds are sized from.
func TestShellClientPassesOperationTimeout(t *testing.T) {
	c, dir := newFakeHelm(t, "0")
	ctx := context.Background()
	values := map[string]interface{}{}

	for _, op := range []struct {
		name string
		run  func() error
	}{
		{"install", func() error { return c.Install(ctx, "gw-ns", values) }},
		{"upgrade", func() error { return c.Upgrade(ctx, "gw-ns", values) }},
		{"uninstall", func() error { return c.Uninstall(ctx, "gw-ns") }},
		{"rollback", func() error { return c.Rollback(ctx, "gw-ns", 2) }},
	} {
		if err := op.run(); err != nil {
			t.Fatalf("%s: %v", op.name, err)
		}
		args, err := os.ReadFile(filepath.Join(dir, "args"))
		if err != nil {
			t.Fatalf("%s: read args: %v", op.name, err)
		}
		lines := strings.Split(strings.TrimSpace(string(args)), "\n")
		if lines[0] != op.name {
			t.Fatalf("%s: helm subcommand = %q", op.name, lines[0])
		}
		if !strings.Contains(string(args), "--timeout\n5m0s\n") {
			t.Fatalf("%s: helm args lack --timeout 5m0s:\n%s", op.name, args)
		}
	}
}

// Helm shutdown survival: a rollback started by stuck-release recovery runs
// through the same helper, so a controller shutdown does not interrupt it.
func TestShellClientRollbackSurvivesParentCancel(t *testing.T) {
	c, dir := newFakeHelm(t, "1")
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- c.Rollback(ctx, "gw-ns", 3) }()

	waitFor(t, dir, "started")
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Rollback after parent cancel: %v", err)
	}
	if !exists(dir, "finished") || exists(dir, "signal") {
		t.Fatal("rollback did not run to completion after the parent context was cancelled")
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if !strings.HasPrefix(string(args), "rollback\nopenshell-gateway\n3\n") {
		t.Fatalf("rollback args = %q", args)
	}
}

func TestShellClientRollbackRejectsInvalidRevision(t *testing.T) {
	c, dir := newFakeHelm(t, "0")
	if err := c.Rollback(context.Background(), "gw-ns", 0); err == nil {
		t.Fatal("Rollback to revision 0 returned nil")
	}
	if exists(dir, "started") {
		t.Fatal("helm ran for an invalid revision")
	}
}

func TestShellClientManifestArgs(t *testing.T) {
	c, dir := newFakeHelm(t, "0")
	if _, err := c.Manifest(context.Background(), "gw-ns", 4); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if got := strings.Join(strings.Fields(string(args)), " "); got != "get manifest openshell-gateway --namespace gw-ns --revision 4" {
		t.Fatalf("manifest args = %q", got)
	}
}

// parseHistory reads the JSON that helm v3.20 `history -o json` writes
// (cmd/helm/history.go releaseInfo; helm's time marshals as RFC 3339, and a
// zero time as "").
func TestParseHistory(t *testing.T) {
	out := `[{"revision":2,"updated":"2026-11-20T13:05:01.123456789Z","status":"pending-upgrade","chart":"openshell-0.1.8","app_version":"v0.1.8","description":"Preparing upgrade"},` +
		`{"revision":1,"updated":"2026-11-20T08:00:00-05:00","status":"deployed","chart":"openshell-0.1.7","app_version":"v0.1.7","description":"Install complete"},` +
		`{"revision":3,"updated":"","status":"failed","chart":"openshell-0.1.8","app_version":"v0.1.8","description":""}]`
	revs, err := parseHistory(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 3 || revs[0].Revision != 1 || revs[1].Revision != 2 || revs[2].Revision != 3 {
		t.Fatalf("revisions not sorted ascending: %+v", revs)
	}
	if want := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC); !revs[0].Updated.Equal(want) {
		t.Fatalf("revision 1 updated = %s, want %s", revs[0].Updated, want)
	}
	if revs[1].Status != "pending-upgrade" || revs[1].Updated.Nanosecond() != 123456789 {
		t.Fatalf("revision 2 = %+v", revs[1])
	}
	if !revs[2].Updated.IsZero() {
		t.Fatalf("empty updated parsed as %s, want zero", revs[2].Updated)
	}
	if _, err := parseHistory(`[{"revision":1,"updated":"yesterday"}]`); err == nil {
		t.Fatal("parseHistory accepted a malformed time")
	}
}

// A pending release is abandoned only once no helm process can still own it:
// helm --timeout, the deadline margin and the kill delay, plus a margin.
func TestPendingStaleAfter(t *testing.T) {
	if got, want := (&ShellClient{}).PendingStaleAfter(), 7*time.Minute; got != want {
		t.Fatalf("PendingStaleAfter = %s, want %s", got, want)
	}
	c := &ShellClient{OperationTimeout: time.Minute, DeadlineMargin: time.Second, KillDelay: time.Second}
	if got, want := c.PendingStaleAfter(), time.Minute+2*time.Second+PendingStaleMargin; got != want {
		t.Fatalf("PendingStaleAfter = %s, want %s", got, want)
	}
}
