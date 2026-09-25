package gateway

import (
	"context"
	cryptoRand "crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	// register postgres driver and use typed error codes for error classification
	pq "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// DatabaseConfig locates the PostgreSQL admin credentials the control plane
// provisions gateway databases with. AdminCredentialsDir is the directory the
// admin Secret is mounted at inside the controller pod; each Secret key is one
// file. The controller refuses to start when the directory does not hold a
// valid credential set (see ValidateAdminCredentialsDir) and re-reads it for
// every database operation so a rotated admin password takes effect without a
// restart. See specs/platform/openshell-gateway-database.spec.md.
type DatabaseConfig struct {
	AdminCredentialsDir string
}

// Admin credential file names inside DatabaseConfig.AdminCredentialsDir. They
// are the keys of the mounted Secret.
const (
	adminCredentialHostFile        = "host"
	adminCredentialPortFile        = "port"
	adminCredentialUserFile        = "user"
	adminCredentialPasswordFile    = "password"
	adminCredentialDBNameFile      = "dbname"
	adminCredentialSSLModeFile     = "sslmode"
	adminCredentialSSLRootCertFile = "sslrootcert"
)

// requiredSSLMode is the only TLS mode the control plane accepts for the admin
// connection. The admin connection verifies the server certificate chain against
// the mounted CA bundle and the hostname against the certificate. The tenant
// connection uses a separate mode (tenantSSLMode) because the upstream Helm
// chart cannot mount a CA bundle into the gateway pod.
const requiredSSLMode = "verify-full"

// defaultAdminDBName is the maintenance database the admin connection opens
// when the Secret carries no dbname key.
const defaultAdminDBName = "postgres"

// adminCredentials is one read of the mounted admin Secret. It is only ever
// alive for the duration of one database operation.
type adminCredentials struct {
	host        string
	port        string
	user        string
	password    string
	dbname      string
	sslrootcert string // PEM CA bundle, verbatim
	// sslrootcertPath is the mounted file the driver reads the bundle from.
	sslrootcertPath string
}

// ValidateAdminCredentialsDir is the control plane's startup precondition for
// gateway database provisioning: it reads the mounted admin Secret once and
// returns an error describing the first problem found. It does not connect to
// the server; reachability and privileges are checked at reconcile time with
// retries so a database outage never keeps the controller from starting.
func ValidateAdminCredentialsDir(dir string) error {
	_, err := readAdminCredentials(dir)
	return err
}

// readAdminCredentials reads and validates the mounted admin Secret. Every
// required file must be present and non-empty, sslrootcert must parse as PEM
// certificates, port must be a TCP port and sslmode, when present, must be
// verify-full. Values other than the password and the CA bundle are trimmed of
// surrounding whitespace so a trailing newline in a Secret key is harmless.
func readAdminCredentials(dir string) (*adminCredentials, error) {
	if dir == "" {
		return nil, fmt.Errorf("gateway database admin credentials directory is not configured")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("gateway database admin credentials directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("gateway database admin credentials path %q is not a directory", dir)
	}

	read := func(name string, required bool) (string, error) {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			if os.IsNotExist(err) && !required {
				return "", nil
			}
			return "", fmt.Errorf("gateway database admin credential %q: %w", name, err)
		}
		return string(raw), nil
	}
	readTrimmed := func(name string, required bool) (string, error) {
		v, err := read(name, required)
		if err != nil {
			return "", err
		}
		v = strings.TrimSpace(v)
		if required && v == "" {
			return "", fmt.Errorf("gateway database admin credential %q is empty", name)
		}
		return v, nil
	}

	host, err := readTrimmed(adminCredentialHostFile, true)
	if err != nil {
		return nil, err
	}
	port, err := readTrimmed(adminCredentialPortFile, true)
	if err != nil {
		return nil, err
	}
	if n, convErr := strconv.Atoi(port); convErr != nil || n < 1 || n > 65535 {
		return nil, fmt.Errorf("gateway database admin credential %q must be a TCP port between 1 and 65535", adminCredentialPortFile)
	}
	user, err := readTrimmed(adminCredentialUserFile, true)
	if err != nil {
		return nil, err
	}
	password, err := read(adminCredentialPasswordFile, true)
	if err != nil {
		return nil, err
	}
	password = strings.TrimRight(password, "\r\n")
	if password == "" {
		return nil, fmt.Errorf("gateway database admin credential %q is empty", adminCredentialPasswordFile)
	}
	dbname, err := readTrimmed(adminCredentialDBNameFile, false)
	if err != nil {
		return nil, err
	}
	if dbname == "" {
		dbname = defaultAdminDBName
	}
	sslmode, err := readTrimmed(adminCredentialSSLModeFile, false)
	if err != nil {
		return nil, err
	}
	if sslmode != "" && sslmode != requiredSSLMode {
		return nil, fmt.Errorf("gateway database admin credential %q is %q; only %q is supported", adminCredentialSSLModeFile, sslmode, requiredSSLMode)
	}
	sslrootcert, err := read(adminCredentialSSLRootCertFile, true)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sslrootcert) == "" {
		return nil, fmt.Errorf("gateway database admin credential %q is empty", adminCredentialSSLRootCertFile)
	}
	if !x509.NewCertPool().AppendCertsFromPEM([]byte(sslrootcert)) {
		return nil, fmt.Errorf("gateway database admin credential %q does not contain a PEM certificate", adminCredentialSSLRootCertFile)
	}

	return &adminCredentials{
		host:            host,
		port:            port,
		user:            user,
		password:        password,
		dbname:          dbname,
		sslrootcert:     sslrootcert,
		sslrootcertPath: filepath.Join(dir, adminCredentialSSLRootCertFile),
	}, nil
}

// dsn builds the admin connection string. The CA bundle is passed to the driver
// as the mounted file path, so no credential material is copied anywhere.
func (c *adminCredentials) dsn() string {
	// URL format percent-encodes special characters in credentials instead of
	// letting them break the space-delimited keyword DSN.
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.user, c.password),
		Host:   net.JoinHostPort(c.host, c.port),
		Path:   "/" + c.dbname,
	}
	q := url.Values{
		"sslmode":         {requiredSSLMode},
		"sslrootcert":     {c.sslrootcertPath},
		"connect_timeout": {"10"},
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// openAdminConn opens a short-lived PostgreSQL admin connection. The returned
// release func closes the connection; callers must always defer it. It is
// non-nil on every return path. PingContext errors are returned unwrapped so
// callers can classify them with connErrorCategory.
func openAdminConn(ctx context.Context, creds *adminCredentials) (*sql.DB, func(), error) {
	noop := func() {}

	db, err := sql.Open("postgres", creds.dsn())
	if err != nil {
		return nil, noop, fmt.Errorf("open admin connection: %w", err)
	}
	db.SetMaxOpenConns(1)

	release := func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARN gateway database: close admin connection: %v", cerr)
		}
	}

	if err := db.PingContext(ctx); err != nil {
		release()
		return nil, noop, err
	}
	return db, release, nil
}

// Connection error categories. They label a failed admin connection in logs
// and errors without repeating the driver's message, which can embed the DSN.
const (
	connErrorUnreachable = "unreachable"
	connErrorTLSFailed   = "tls_failed"
	connErrorAuthFailed  = "auth_failed"
)

// connErrorCategory maps a raw connection error to one of the categories above.
// The raw error is never returned. It is a label only; no control flow depends
// on it, so a misclassification has low impact.
func connErrorCategory(err error) string {
	lower := strings.ToLower(err.Error())

	var netErr net.Error
	if errors.As(err, &netErr) {
		return connErrorUnreachable
	}
	if strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "i/o timeout") ||
		strings.Contains(lower, "network") {
		return connErrorUnreachable
	}
	// Typed SQLSTATE check runs before TLS string matching so an auth rejection
	// whose message mentions SSL is still classified as auth_failed.
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "28P01", "28000": // invalid_password, invalid_authorization_specification
			return connErrorAuthFailed
		}
	}
	if strings.Contains(lower, "tls") ||
		strings.Contains(lower, "certificate") ||
		strings.Contains(lower, "x509") ||
		strings.Contains(lower, "ssl") {
		return connErrorTLSFailed
	}
	if strings.Contains(lower, "password authentication failed") ||
		strings.Contains(lower, "28p01") ||
		strings.Contains(lower, "28000") {
		return connErrorAuthFailed
	}
	return connErrorUnreachable
}

// gatewayDBName returns the PostgreSQL role and database name for a gateway.
// Role and database share the name.
func gatewayDBName(gatewayID string) string {
	return "gw_" + strings.ToLower(gatewayID)
}

// tenantGatewayDBSecretName is the tenant-namespace Secret the gateway workload
// consumes. The gateway is deployed through the upstream OpenShell Helm chart
// (server.externalDbSecret), which reads only this Secret's uri key and injects
// it as OPENSHELL_DB_URL; it has no mechanism to mount an extra CA file into the
// gateway pod for the database connection (unlike its OIDC and Vault CA
// support). The tenant connection therefore uses sslmode=require: encrypted,
// but not certificate-verified. The admin connection above is unaffected by
// this and stays verify-full.
const tenantGatewayDBSecretName = "openshell-gateway-db-credentials"

// tenantSSLMode is the TLS mode written into the tenant Secret and the uri the
// gateway workload consumes. See tenantGatewayDBSecretName for why this differs
// from requiredSSLMode.
const tenantSSLMode = "require"

// tenantSecretData builds the per-gateway credentials Secret. The gateway
// receives only its own role and its database; admin credentials never reach a
// tenant namespace.
func tenantSecretData(creds *adminCredentials, pgName, password string) map[string][]byte {
	q := url.Values{
		"sslmode": {tenantSSLMode},
	}
	uri := url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(pgName, password),
		Host:     net.JoinHostPort(creds.host, creds.port),
		Path:     "/" + pgName,
		RawQuery: q.Encode(),
	}
	return map[string][]byte{
		"host":     []byte(creds.host),
		"port":     []byte(creds.port),
		"dbname":   []byte(pgName),
		"user":     []byte(pgName),
		"password": []byte(password),
		"sslmode":  []byte(tenantSSLMode),
		"uri":      []byte(uri.String()),
	}
}

// tenantProbeDSN builds the connection string the readiness probe uses to
// confirm the freshly provisioned gateway credentials work. It mirrors the
// credentials written into the tenant Secret (tenantSecretData): the gateway
// role, its database and sslmode=require. require encrypts but does not verify
// the server certificate, so no CA file is referenced - the same TLS posture the
// gateway workload connects with (see tenantGatewayDBSecretName).
func tenantProbeDSN(creds *adminCredentials, pgName, password string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(pgName, password),
		Host:   net.JoinHostPort(creds.host, creds.port),
		Path:   "/" + pgName,
	}
	q := url.Values{
		"sslmode":         {tenantSSLMode},
		"connect_timeout": {tenantProbeConnectTimeout},
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// tenantProbeMaxAttempts and tenantProbeBaseDelay bound the readiness probe's
// exponential backoff. The delay doubles from the base, so six attempts wait
// 200ms, 400ms, 800ms, 1.6s and 3.2s between tries (~6.2s of backoff). That
// absorbs the brief window right after CREATE DATABASE where the new role or
// database is not yet connectable.
//
// Backoff is not the only time cost: each attempt also spends up to
// tenantProbeConnectTimeout establishing the connection, which against a
// black-holed host (packets dropped rather than refused) is the full timeout.
// tenantProbeMaxWait caps the whole loop so a probe can never pin the reconcile
// worker for more than that regardless of per-attempt connect stalls; the loop
// derives a child context with this deadline (never extending the caller's).
const (
	tenantProbeMaxAttempts    = 6
	tenantProbeBaseDelay      = 200 * time.Millisecond
	tenantProbeConnectTimeout = "5" // seconds, lib/pq connect_timeout
	tenantProbeMaxWait        = 30 * time.Second
)

// tenantProbeDelay is the backoff before the (attempt+1)th probe try. attempt is
// zero-based, so attempt 0 is the first retry after the initial failure.
func tenantProbeDelay(attempt int) time.Duration {
	return tenantProbeBaseDelay << attempt
}

// verifyTenantConn confirms the provisioned gateway credentials actually work by
// connecting as the gateway role to its own database with the TLS posture the
// gateway workload uses (sslmode=require) and pinging it. DDL success alone does
// not prove the role can log in and connect: a wrong password, a missing CONNECT
// grant or a database not yet accepting connections all surface only on a real
// login. It retries with exponential backoff so a transient failure right after
// CREATE DATABASE does not fail provisioning, and returns a categorized error
// (never the raw driver error, which can embed the DSN) once attempts run out.
// The whole loop is bounded by tenantProbeMaxWait so a stalled connect can never
// pin the reconcile worker.
func verifyTenantConn(ctx context.Context, creds *adminCredentials, gatewayID, pgName, password string) error {
	ctx, cancel := context.WithTimeout(ctx, tenantProbeMaxWait)
	defer cancel()

	dsn := tenantProbeDSN(creds, pgName, password)
	var last error
	for attempt := 0; attempt < tenantProbeMaxAttempts; attempt++ {
		if attempt > 0 {
			delay := tenantProbeDelay(attempt - 1)
			log.Printf("WARN tenant database probe for gateway %s: %s, retrying in %s", gatewayID, connErrorCategory(last), delay)
			select {
			case <-ctx.Done():
				return fmt.Errorf("verify tenant connection for gateway %s: %w", gatewayID, ctx.Err())
			case <-time.After(delay):
			}
		}
		last = pingTenant(ctx, dsn)
		if last == nil {
			log.Printf("INFO verified tenant database connection for gateway %s", gatewayID)
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("verify tenant connection for gateway %s: %w", gatewayID, ctx.Err())
		}
	}
	return fmt.Errorf("verify tenant connection for gateway %s: %s (driver error redacted, %d attempts)", gatewayID, connErrorCategory(last), tenantProbeMaxAttempts)
}

// pingTenant opens a short-lived tenant connection and pings it, always closing
// the connection before returning. The ping error is returned unwrapped so
// verifyTenantConn can classify it with connErrorCategory.
func pingTenant(ctx context.Context, dsn string) error {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open tenant connection: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARN tenant database probe: close connection: %v", cerr)
		}
	}()
	return db.PingContext(ctx)
}

// databaseReconciler is the DatabaseReconciler for the mounted admin
// credentials. See db_reconciler.go for the interface contract.
type databaseReconciler struct {
	cfg DatabaseConfig
}

// gatewayDatabaseDDLMu serializes per-gateway CREATE/DROP DATABASE in this
// process. PostgreSQL copies a template directory for CREATE DATABASE; running
// two of those at once against template1 fails with SQLSTATE 55006 ("source
// database is being accessed by other users") and Kind e2e hits that when the
// seeded gateway and the suite gateway provision concurrently. template0 (see
// createGatewayDatabaseSQL) is the durable fix; the mutex keeps same-process
// workers from stampeding the server on top of that.
var gatewayDatabaseDDLMu sync.Mutex

// createGatewayDatabaseSQL copies template0, not the default template1.
// template1 accepts connections (autovacuum, leftover backends, another
// CREATE DATABASE), and any one of those fails provisioning. template0 has
// datallowconn=false, which is the documented way to create databases
// concurrently.
func createGatewayDatabaseSQL(pgName string) string {
	return fmt.Sprintf(
		"CREATE DATABASE %s OWNER %s TEMPLATE template0",
		pgQuoteIdent(pgName),
		pgQuoteIdent(pgName),
	)
}

func isDatabaseSourceBusy(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "55006" { // object_in_use
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "being accessed by other users")
}

func execCreateGatewayDatabase(ctx context.Context, db *sql.DB, gatewayID, pgName string) error {
	stmt := createGatewayDatabaseSQL(pgName)
	var last error
	for attempt := 0; attempt < 8; attempt++ {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			log.Printf("INFO created database %s for gateway %s", pgName, gatewayID)
			return nil
		} else {
			last = err
		}
		if !isDatabaseSourceBusy(last) {
			return fmt.Errorf("CREATE DATABASE for gateway %s: %w", gatewayID, last)
		}
		delay := time.Duration(attempt+1) * 200 * time.Millisecond
		log.Printf("WARN CREATE DATABASE for gateway %s: source database busy, retrying in %s: %v", gatewayID, delay, last)
		select {
		case <-ctx.Done():
			return fmt.Errorf("CREATE DATABASE for gateway %s: %w", gatewayID, ctx.Err())
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("CREATE DATABASE for gateway %s: %w", gatewayID, last)
}

// reconcileGatewayDatabaseDDL applies the idempotent provisioning DDL. It reports
// whether it established new credentials or a new database this pass (role
// created, password re-synced, or database created); the caller uses that to
// decide whether a tenant connection probe is warranted. Idempotent re-runs that
// change nothing report false.
func reconcileGatewayDatabaseDDL(ctx context.Context, db *sql.DB, adminUser, pgName, gatewayID, password string, freshPassword bool) (bool, error) {
	changed := false
	var roleExists bool
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", pgName,
	).Scan(&roleExists); err != nil {
		return false, fmt.Errorf("check role existence for gateway %s: %w", gatewayID, err)
	}
	if !roleExists {
		// lib/pq cannot parameterize CREATE ROLE / ALTER ROLE, so the password is
		// interpolated into the statement text. On servers with log_statement=all
		// it appears in the server log; operators should restrict log verbosity
		// or use server-side redaction accordingly.
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", pgQuoteIdent(pgName), pgQuoteLiteral(password)),
		); err != nil {
			return false, fmt.Errorf("CREATE ROLE for gateway %s: DDL execution failed (credentials redacted)", gatewayID)
		}
		changed = true
		log.Printf("INFO created database role %s for gateway %s", pgName, gatewayID)
	} else if freshPassword {
		// Provisioning repair, not credential rotation: the role exists but its
		// tenant Secret is gone, so the server-side password is re-synced to the
		// Secret about to be written.
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf("ALTER ROLE %s PASSWORD '%s'", pgQuoteIdent(pgName), pgQuoteLiteral(password)),
		); err != nil {
			return false, fmt.Errorf("ALTER ROLE password for gateway %s: DDL execution failed (credentials redacted)", gatewayID)
		}
		changed = true
		log.Printf("INFO repaired database role password for gateway %s (tenant Secret was absent)", gatewayID)
	}
	// Out-of-band password drift (tenant Secret present, server-side password
	// changed externally) is not reconciled or detected here: a steady-state
	// reconcile makes no credential change, so it reports changed=false and the
	// caller runs no tenant login. Recover by deleting the tenant Secret, which
	// makes the branch above re-apply a fresh password on the next reconcile.

	// A non-superuser admin needs membership in the gateway role to create a
	// database owned by it (PostgreSQL 16 and later do not grant SET ROLE on
	// creation). GRANT is idempotent and harmless for a superuser admin.
	if !strings.EqualFold(adminUser, pgName) {
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf("GRANT %s TO %s", pgQuoteIdent(pgName), pgQuoteIdent(adminUser)),
		); err != nil {
			return false, fmt.Errorf("GRANT gateway role to admin for gateway %s: %w", gatewayID, err)
		}
	}

	var dbExists bool
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", pgName,
	).Scan(&dbExists); err != nil {
		return false, fmt.Errorf("check database existence for gateway %s: %w", gatewayID, err)
	}
	if !dbExists {
		// CREATE DATABASE cannot run inside a transaction block and has no IF NOT EXISTS.
		if err := execCreateGatewayDatabase(ctx, db, gatewayID, pgName); err != nil {
			return false, err
		}
		changed = true
	}

	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC", pgQuoteIdent(pgName)),
	); err != nil {
		return false, fmt.Errorf("REVOKE CONNECT for gateway %s: %w", gatewayID, err)
	}
	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", pgQuoteIdent(pgName), pgQuoteIdent(pgName)),
	); err != nil {
		return false, fmt.Errorf("GRANT CONNECT for gateway %s: %w", gatewayID, err)
	}
	return changed, nil
}

// Reconcile provisions the gateway's role and database and writes the tenant
// credentials Secret. HyperShell does not rotate per-gateway database
// credentials; see openshell-gateway-database.spec.md.
func (r *databaseReconciler) Reconcile(ctx context.Context, _ dynamic.Interface, clientset kubernetes.Interface, tenantNamespace, gatewayID string) error {
	if err := ReconcileGatewayDatabase(ctx, clientset, tenantNamespace, gatewayID, r.cfg); err != nil {
		return fmt.Errorf("reconcile gateway database for namespace %s: %w", tenantNamespace, err)
	}
	return nil
}

// Delete drops the gateway's database and role. A non-nil error means the
// objects may still exist on the server; the caller returns it to the delete
// retry queue and records an IncompleteFinalization Event so the leftover is
// operator-visible (gateway-deletion-finalization.spec.md).
func (r *databaseReconciler) Delete(ctx context.Context, _ dynamic.Interface, _ kubernetes.Interface, gatewayID string) error {
	if gatewayID == "" {
		return nil
	}
	if err := DeleteGatewayDatabase(ctx, r.cfg, gatewayID); err != nil {
		return fmt.Errorf("drop database and role %q: %w", gatewayDBName(gatewayID), err)
	}
	return nil
}

// ReconcileGatewayDatabase provisions a dedicated role and database for
// gatewayID on the admin server, then writes the tenant credentials Secret. It
// is idempotent: re-running against an already-provisioned gateway makes no
// destructive change and does not regenerate the password.
func ReconcileGatewayDatabase(
	ctx context.Context,
	clientset kubernetes.Interface,
	tenantNamespace string,
	gatewayID string,
	cfg DatabaseConfig,
) error {
	creds, err := readAdminCredentials(cfg.AdminCredentialsDir)
	if err != nil {
		return fmt.Errorf("read admin credentials: %w", err)
	}

	pgName := gatewayDBName(gatewayID)

	// Password: reuse the one in the existing tenant Secret, or generate a new one.
	password := ""
	existingSecret, secretErr := clientset.CoreV1().Secrets(tenantNamespace).Get(ctx, tenantGatewayDBSecretName, metav1.GetOptions{})
	if secretErr != nil && !k8serrors.IsNotFound(secretErr) {
		return fmt.Errorf("get gateway credentials secret %s/%s: %w", tenantNamespace, tenantGatewayDBSecretName, secretErr)
	}
	secretExists := secretErr == nil
	if secretExists {
		password = string(existingSecret.Data["password"])
	}

	freshPassword := password == ""
	if freshPassword {
		passwordBytes := make([]byte, 32)
		if _, err := cryptoRand.Read(passwordBytes); err != nil {
			return fmt.Errorf("generate database password: %w", err)
		}
		password = hex.EncodeToString(passwordBytes)
	}

	db, release, err := openAdminConn(ctx, creds)
	if err != nil {
		return fmt.Errorf("connect to gateway database server: %s (driver error redacted)", connErrorCategory(err))
	}
	defer release()

	var provisioned bool
	if err := func() error {
		gatewayDatabaseDDLMu.Lock()
		defer gatewayDatabaseDDLMu.Unlock()
		var ddlErr error
		provisioned, ddlErr = reconcileGatewayDatabaseDDL(ctx, db, creds.user, pgName, gatewayID, password, freshPassword)
		return ddlErr
	}(); err != nil {
		return err
	}

	// Verify the credentials actually connect before writing the tenant Secret,
	// so DatabaseReady is never reported on credentials that cannot log in.
	// Retries with backoff absorb the brief post-CREATE window. Only probe when
	// this pass established new credentials or a new database: a steady-state
	// reconcile of an already-provisioned gateway would otherwise turn every
	// pass into a tenant login that a transient database blip could fail.
	if provisioned {
		if err := verifyTenantConn(ctx, creds, gatewayID, pgName, password); err != nil {
			return err
		}
	}

	// Write or refresh the tenant credentials Secret. Refreshing also propagates a
	// rotated CA bundle; the gateway pod picks the new file up on its next restart.
	desiredData := tenantSecretData(creds, pgName, password)
	desiredLabels := map[string]string{
		"app.kubernetes.io/name":       "openshell",
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/managed-by": "hypershell-control-plane",
		"hypershell.redhat.io/managed": "true",
	}

	if !secretExists {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tenantGatewayDBSecretName,
				Namespace: tenantNamespace,
				Labels:    desiredLabels,
			},
			Type: corev1.SecretTypeOpaque,
			Data: desiredData,
		}
		if _, err := clientset.CoreV1().Secrets(tenantNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create gateway credentials secret %s/%s: %w", tenantNamespace, tenantGatewayDBSecretName, err)
		}
		log.Printf("INFO created gateway credentials secret %s in %s", tenantGatewayDBSecretName, tenantNamespace)
	} else {
		updated := existingSecret.DeepCopy()
		if updated.Labels == nil {
			updated.Labels = map[string]string{}
		}
		for k, v := range desiredLabels {
			updated.Labels[k] = v
		}
		updated.Data = desiredData
		if !reflect.DeepEqual(existingSecret.Data, updated.Data) || !reflect.DeepEqual(existingSecret.Labels, updated.Labels) {
			if _, err := clientset.CoreV1().Secrets(tenantNamespace).Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update gateway credentials secret %s/%s: %w", tenantNamespace, tenantGatewayDBSecretName, err)
			}
		}
	}

	log.Printf("INFO database provisioning complete for gateway %s in namespace %s", gatewayID, tenantNamespace)
	return nil
}

// DeleteGatewayDatabase terminates active connections and drops the gateway's
// database and role. Absent objects count as already removed. A failed
// existence query, connection or DROP returns an error: the objects may still
// exist and the caller must not report the cleanup as complete.
func DeleteGatewayDatabase(ctx context.Context, cfg DatabaseConfig, gatewayID string) error {
	if gatewayID == "" {
		return nil
	}

	creds, err := readAdminCredentials(cfg.AdminCredentialsDir)
	if err != nil {
		return fmt.Errorf("read admin credentials: %w", err)
	}

	db, release, err := openAdminConn(ctx, creds)
	if err != nil {
		return fmt.Errorf("connect to gateway database server: %s (driver error redacted)", connErrorCategory(err))
	}
	defer release()

	gatewayDatabaseDDLMu.Lock()
	defer gatewayDatabaseDDLMu.Unlock()
	return dropGatewayDatabase(ctx, db, gatewayID)
}

// dropGatewayDatabase runs the cleanup statements on an open admin connection.
func dropGatewayDatabase(ctx context.Context, db *sql.DB, gatewayID string) error {
	pgName := gatewayDBName(gatewayID)

	// Terminate active backends so DROP DATABASE is not blocked. Terminating
	// another role's backends needs superuser or pg_signal_backend membership; a
	// failure here is not fatal because DROP DATABASE ... WITH (FORCE) below
	// performs the same termination server-side.
	if _, err := db.ExecContext(ctx,
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
		pgName,
	); err != nil {
		log.Printf("WARN database cleanup for gateway %s: terminate backends failed (proceeding): %v", gatewayID, err)
	}

	// Drop the database first: the role owns it.
	var dbExists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", pgName).Scan(&dbExists); err != nil {
		return fmt.Errorf("check database existence before cleanup: %w", err)
	}
	if dbExists {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", pgQuoteIdent(pgName))); err != nil {
			return fmt.Errorf("DROP DATABASE failed: %w", err)
		}
		log.Printf("INFO dropped database %s for gateway %s", pgName, gatewayID)
	}

	var roleExists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", pgName).Scan(&roleExists); err != nil {
		return fmt.Errorf("check role existence before cleanup: %w", err)
	}
	if roleExists {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP ROLE %s", pgQuoteIdent(pgName))); err != nil {
			return fmt.Errorf("DROP ROLE failed: %w", err)
		}
		log.Printf("INFO dropped role %s for gateway %s", pgName, gatewayID)
	}
	return nil
}

// pgQuoteIdent quotes a PostgreSQL identifier to prevent SQL injection.
// Only safe for identifiers produced from internal gateway IDs and the admin
// user name read from the mounted Secret.
func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// pgQuoteLiteral quotes a string literal for use in SQL by doubling single
// quotes. Safe here because every interpolated value is a hex-encoded password
// (character set [0-9a-f]); DO NOT reuse this function for arbitrary user input.
func pgQuoteLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
