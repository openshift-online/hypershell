package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Requests are bound to an attempt, never to a reusable operating system PID.
// Old detached workers have no control record and cannot acknowledge requests.
func (s *Store) requestCancel(key, scope, id string) error {
	result, err := s.db.Exec(`UPDATE worker_control SET requested=1 WHERE run_id=? AND EXISTS
 (SELECT 1 FROM jobs WHERE key=? AND scope=? AND run_id=? AND status='running')`, id, key, scope, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("run changed or its worker predates cancellation support; no interrupt was sent")
	}
	return nil
}

func (s *Store) watchCancellation(ctx context.Context, key, id string, stop context.CancelFunc) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var requested bool
			err := s.db.QueryRowContext(ctx, `SELECT requested FROM worker_control WHERE run_id=?`, id).Scan(&requested)
			if err != nil {
				if ctx.Err() == nil {
					fmt.Fprintln(os.Stderr, "Cannot read cancellation request; retrying:", err)
				}
				continue
			}
			if !requested {
				continue
			}
			result, err := s.db.ExecContext(ctx, `UPDATE worker_control SET acknowledged=1 WHERE run_id=? AND requested=1 AND acknowledged=0
 AND EXISTS (SELECT 1 FROM jobs WHERE key=? AND run_id=? AND status='running')`, id, key, id)
			if err != nil {
				if ctx.Err() == nil {
					fmt.Fprintln(os.Stderr, "Cannot check cancellation request; retrying:", err)
				}
				continue
			}
			n, err := result.RowsAffected()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Cannot read cancellation acknowledgement:", err)
				continue
			}
			if n == 1 {
				fmt.Fprintln(os.Stderr, "Cancel requested. Interrupting Fullsend and waiting for cleanup; completed external changes are retained.")
				stop()
				return
			}
		}
	}
}

// Signal only Fullsend, leaving the supervisor alive to save the outcome and
// hold ownership. Fullsend owns remote interruption, post-script gates and
// cleanup. Never escalate to SIGKILL or impose a cleanup deadline.
func gracefulCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	return cmd
}
