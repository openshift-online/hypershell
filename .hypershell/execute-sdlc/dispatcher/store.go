package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

type Job struct {
	Key               string          `json:"key"`
	Scope             string          `json:"scope"`
	Agent             string          `json:"agent"`
	Item              string          `json:"item"`
	Title             string          `json:"title,omitempty"`
	URL               string          `json:"url"`
	Fingerprint       string          `json:"fingerprint"`
	SourceFingerprint string          `json:"source_fingerprint,omitempty"`
	Labels            []string        `json:"labels,omitempty"`
	PRNumber          string          `json:"pr_number"`
	PRURL             string          `json:"pr_url"`
	PreviousPhase     string          `json:"previous_phase,omitempty"`
	Repo              string          `json:"repo,omitempty"`
	Command           []string        `json:"command,omitempty"`
	Jira              json.RawMessage `json:"jira_auth,omitempty"`
}
type Row struct {
	Job
	Status   string  `json:"status"`
	Failures int     `json:"failures"`
	Next     float64 `json:"next_at"`
	Started  float64 `json:"started"`
	Finished float64 `json:"finished"`
	RunID    string  `json:"run_id"`
	Log      string  `json:"log"`
	Error    string  `json:"error"`
	Activity string  `json:"activity,omitempty"`
}
type Store struct {
	db   *sql.DB
	path string
}

func seconds() float64 { return float64(time.Now().UnixNano()) / 1e9 }
func hashString(s string) string {
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func openStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Configure every connection, including replacements. IMMEDIATE acquires the
	// write reservation before finish reads; deferred read-to-write upgrades can
	// fail immediately with SQLITE_BUSY despite a busy timeout.
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String() + "?_pragma=busy_timeout(30000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS jobs (key TEXT PRIMARY KEY,scope TEXT NOT NULL,agent TEXT NOT NULL,item TEXT NOT NULL,fingerprint TEXT NOT NULL,payload TEXT NOT NULL,status TEXT NOT NULL,failures INTEGER NOT NULL DEFAULT 0,next_at REAL NOT NULL DEFAULT 0,started REAL NOT NULL DEFAULT 0,finished REAL,run_id TEXT,log TEXT,error TEXT);
 CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY,job_key TEXT NOT NULL,started REAL NOT NULL,finished REAL,exit_code INTEGER,log TEXT NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db, path}, nil
}
func (s *Store) rows() ([]Row, error) {
	rs, err := s.db.Query(`SELECT payload,status,failures,next_at,started,coalesce(finished,0),coalesce(run_id,''),coalesce(log,''),coalesce(error,'') FROM jobs ORDER BY started DESC`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	rows := []Row{}
	for rs.Next() {
		var r Row
		var payload string
		if err = rs.Scan(&payload, &r.Status, &r.Failures, &r.Next, &r.Started, &r.Finished, &r.RunID, &r.Log, &r.Error); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(payload), &r.Job); err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, rs.Err()
}
func eligible(j Job, rows []Row, now float64) bool {
	for _, r := range rows {
		if r.Item == j.Item && (r.Status == "running" || r.Status == "uncertain") {
			return false
		}
		if r.Key == j.Key && (r.Next > now || r.Status == "succeeded" && r.Fingerprint == j.Fingerprint) {
			return false
		}
	}
	return true
}
func (s *Store) start(j Job, log string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	payload, err := json.Marshal(j)
	if err != nil {
		return "", err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	now := seconds()
	_, err = tx.Exec(`INSERT INTO jobs(key,scope,agent,item,fingerprint,payload,status,started,run_id,log) VALUES(?,?,?,?,?,?,'running',?,?,?)
 ON CONFLICT(key) DO UPDATE SET scope=excluded.scope,fingerprint=excluded.fingerprint,payload=excluded.payload,status='running',started=excluded.started,run_id=excluded.run_id,log=excluded.log,error=NULL,finished=NULL,next_at=0`, j.Key, j.Scope, j.Agent, j.Item, j.Fingerprint, string(payload), now, id, log)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO runs(id,job_key,started,log) VALUES(?,?,?,?)`, id, j.Key, now, log)
	}
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}
func (s *Store) finish(key, id string, rc, base, cap int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var failures int
	var log string
	if err = tx.QueryRow(`SELECT failures,log FROM jobs WHERE key=? AND run_id=?`, key, id).Scan(&failures, &log); err != nil {
		return fmt.Errorf("run ownership changed for %s: %w", key, err)
	}
	status := "succeeded"
	delay := 0.
	var message any
	if rc != 0 {
		status = "failed"
		failures++
		delay = math.Min(float64(cap), float64(base)*math.Pow(2, float64(min(failures-1, 20))))
		message = fmt.Sprintf("exit %d; see %s", rc, log)
	} else {
		failures = 0
	}
	now := seconds()
	_, err = tx.Exec(`UPDATE jobs SET status=?,failures=?,next_at=?,finished=?,error=? WHERE key=? AND run_id=?`, status, failures, now+delay, now, message, key, id)
	if err == nil {
		_, err = tx.Exec(`UPDATE runs SET finished=?,exit_code=? WHERE id=?`, now, rc, id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func lockFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func (s *Store) runLock(id string) (*os.File, error) {
	return lockFile(filepath.Join(filepath.Dir(s.path), "workers", hashString(id)+".lock"))
}
func (s *Store) alive(id string) (bool, error) {
	f, err := s.runLock(id)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, f.Close()
}
func (s *Store) recover() error {
	rows, err := s.rows()
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Status != "running" {
			continue
		}
		alive, err := s.alive(r.RunID)
		if err != nil {
			return err
		}
		if !alive {
			_, err = s.db.Exec(`UPDATE jobs SET status='uncertain',error='Worker disappeared; inspect its log before retrying' WHERE key=? AND run_id=? AND status='running'`, r.Key, r.RunID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Store) retry(key string) error {
	result, err := s.db.Exec(`UPDATE jobs SET status='failed',next_at=0 WHERE key=? AND status='uncertain'`, key)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%s is not an uncertain job", key)
	}
	return nil
}
func (s *Store) exitLogs(scope string) error {
	rows, err := s.db.Query(`SELECT r.job_key,r.log FROM runs r JOIN jobs j ON j.key=r.job_key WHERE j.scope=? AND r.exit_code!=0 ORDER BY r.started`, scope)
	if err != nil {
		return err
	}
	printed := false
	for rows.Next() {
		var key, path string
		if err = rows.Scan(&key, &path); err != nil {
			rows.Close()
			return err
		}
		if !printed {
			fmt.Fprintln(os.Stderr, "\nFailed run logs:")
			printed = true
		}
		fmt.Fprintf(os.Stderr, "%s\n%s\n", key, path)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	current, err := s.rows()
	if err != nil {
		return err
	}
	printed = false
	for _, r := range current {
		if r.Scope != scope || r.Status != "running" && r.Status != "uncertain" {
			continue
		}
		if !printed {
			fmt.Fprintln(os.Stderr, "\nRunning or unresolved run logs:")
			printed = true
		}
		fmt.Fprintf(os.Stderr, "%s (%s)\n%s\n", r.Key, r.Status, r.Log)
	}
	return nil
}

// Manual retry bypasses only the delay, never ownership, scope, or authorization.
func (s *Store) retryFailed(key, scope string) error {
	result, err := s.db.Exec(`UPDATE jobs SET next_at=0 WHERE key=? AND scope=? AND status='failed'`, key, scope)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%s is not a failed job in this scope; running, completed, and unknown outcomes cannot be retried here", key)
	}
	return nil
}

// history returns individual attempts, including retries, rather than just the
// latest state of each agent. Scope and repository prevent cross-issue mixing.
func (s *Store) history(issue Row) ([]Row, error) {
	rs, err := s.db.Query(`SELECT j.payload,r.id,r.started,r.finished,r.exit_code,r.log,j.status,coalesce(j.run_id,'') FROM runs r JOIN jobs j ON j.key=r.job_key WHERE j.scope=? AND j.item=? ORDER BY r.started DESC,r.id DESC LIMIT 200`, issue.Scope, issue.Item)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	result := []Row{}
	for rs.Next() {
		var row Row
		var payload, currentStatus, currentID string
		var finished sql.NullFloat64
		var code sql.NullInt64
		if err := rs.Scan(&payload, &row.RunID, &row.Started, &finished, &code, &row.Log, &currentStatus, &currentID); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(payload), &row.Job); err != nil {
			return nil, err
		}
		if row.Repo != issue.Repo {
			continue
		}
		row.Key = row.RunID
		row.Status = "uncertain"
		if code.Valid {
			if code.Int64 == 0 {
				row.Status = "succeeded"
			} else {
				row.Status = "failed"
				row.Error = fmt.Sprintf("exit %d", code.Int64)
			}
		} else if row.RunID == currentID {
			row.Status = currentStatus
		}
		if finished.Valid {
			row.Finished = finished.Float64
		}
		// Historical failures are outcomes, not promises of a scheduled retry.
		row.Activity = "history"
		result = append(result, row)
	}
	return result, rs.Err()
}

func databaseBusy(err error) bool {
	var coded interface{ Code() int }
	return errors.As(err, &coded) && (coded.Code()&255 == 5 || coded.Code()&255 == 6)
}

// Keep ownership of the worker while persisting its result. Retrying this
// operation must never rerun Fullsend or release the per-run lock prematurely.
func (s *Store) finishReliably(key, id string, rc, base, cap int) error {
	for {
		err := s.finish(key, id, rc, base, cap)
		if !databaseBusy(err) {
			return err
		}
		fmt.Fprintf(os.Stderr, "Agent exited with code %d. State database is busy; retrying result save in 1s. The agent will not run again.\n", rc)
		time.Sleep(time.Second)
	}
}

// The user confirmed the remote sandbox has stopped. Still reject stale
// confirmations and live local workers; confirmation never bypasses scope.
func (s *Store) retryConfirmedUnknown(key, scope, id string) error {
	lock, err := s.runLock(id)
	if err != nil {
		return fmt.Errorf("cannot retry while the previous worker is active: %w", err)
	}
	defer lock.Close()
	result, err := s.db.Exec(`UPDATE jobs SET status='failed',next_at=0 WHERE key=? AND scope=? AND run_id=? AND status='uncertain'`, key, scope, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%s changed since confirmation or is outside this scope; refresh and inspect the current run", key)
	}
	return nil
}
