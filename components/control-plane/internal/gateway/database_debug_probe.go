package gateway

// TEMPORARY diagnostic instrumentation for the intermittent "connect to
// gateway database server: unreachable" Kind e2e flake under investigation
// on PR #354. Unlike an earlier version of this instrumentation, this does
// NOT open any extra connections: it wraps the REAL dial call the admin and
// tenant connections already make (via pq.Connector.Dialer), logging
// entry/exit timing and the raw (unredacted) net error for that one real
// attempt. No-op unless HYPERSHELL_DB_DEBUG_DIAL_LOG=1 (only set in the Kind
// e2e overlay). Delete this file, its two call sites in database.go, and the
// env var once the flake is root-caused.

import (
	"database/sql"
	"log"
	"net"
	"os"
	"time"

	"github.com/lib/pq"
)

var debugDialLoggingEnabled = os.Getenv("HYPERSHELL_DB_DEBUG_DIAL_LOG") == "1"

// debugLoggingDialer wraps the real net.Dial/net.DialTimeout call pq makes
// to open a connection. It never dials anything itself beyond what the
// caller's own attempt already does.
type debugLoggingDialer struct {
	label string
}

func (d debugLoggingDialer) Dial(network, address string) (net.Conn, error) {
	start := time.Now()
	conn, err := net.Dial(network, address)
	log.Printf("DEBUG_DIAL[%s] Dial(%s, %s) duration=%s err=%v", d.label, network, address, time.Since(start), err)
	return conn, err
}

func (d debugLoggingDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	start := time.Now()
	conn, err := net.DialTimeout(network, address, timeout)
	log.Printf("DEBUG_DIAL[%s] DialTimeout(%s, %s, timeout=%s) duration=%s err=%v", d.label, network, address, timeout, time.Since(start), err)
	return conn, err
}

// debugOpenPQ opens dsn as *sql.DB. When HYPERSHELL_DB_DEBUG_DIAL_LOG=1 it
// routes the real connection through debugLoggingDialer; otherwise it is
// exactly sql.Open("postgres", dsn).
func debugOpenPQ(dsn, label string) (*sql.DB, error) {
	if !debugDialLoggingEnabled {
		return sql.Open("postgres", dsn)
	}
	connector, err := pq.NewConnector(dsn)
	if err != nil {
		return nil, err
	}
	connector.Dialer(debugLoggingDialer{label: label})
	return sql.OpenDB(connector), nil
}

// debugLogRawError logs the raw (unredacted) error for label when dial
// logging is enabled; a no-op otherwise. Used at the points where the real
// error is normally discarded in favor of a category label.
func debugLogRawError(label string, err error) {
	if debugDialLoggingEnabled {
		log.Printf("DEBUG_DIAL[%s] raw error (unredacted): %v", label, err)
	}
}
