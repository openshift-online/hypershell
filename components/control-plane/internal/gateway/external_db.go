package gateway

import (
	"context"
	cryptoRand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"

	// register postgres driver and use typed error codes for status mapping
	pq "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type externalDatabaseReconciler struct {
	cfg ExternalDBConfig
}

// Reconcile provisions the gateway's external role and database. The
// rotateAnnotation parameter is deliberately ignored: external mode does not
// implement credential rotation, so the hypershell.redhat.io/rotate-db-credentials
// annotation is inert for external-backed gateways. See
// openshell-gateway-database-external.spec.md § Requirement: No Credential
// Rotation (External Mode).
func (r *externalDatabaseReconciler) Reconcile(ctx context.Context, _ dynamic.Interface, clientset kubernetes.Interface, tenantNamespace, gatewayID, _ string) error {
	if err := ReconcileExternalDatabaseResources(ctx, clientset, tenantNamespace, gatewayID, r.cfg); err != nil {
		return fmt.Errorf("reconcile external database resources in %s: %w", tenantNamespace, err)
	}
	return nil
}

// Delete drops the gateway's external database and role. Cleanup is
// unconditional, single-shot and best-effort: the gateway is already removed
// from the API server, there is no tombstone and no retry queue, so no later
// event re-delivers this work.
//
// It therefore always returns nil. A failure is logged at ERROR naming the
// gateway and ManagedDatabase IDs (never credentials) so the orphaned role and
// database are discoverable; operators reclaim them with the runbook in
// openshell-gateway-database-external.spec.md § Operator Runbook. Returning an
// error instead would strand gateway finalization on a retry that never
// succeeds.
func (r *externalDatabaseReconciler) Delete(ctx context.Context, _ dynamic.Interface, clientset kubernetes.Interface, gatewayID string) error {
	if gatewayID == "" || r.cfg.CredentialsNamespace == "" {
		return nil
	}
	if err := DeleteExternalDatabaseResources(ctx, clientset, r.cfg, gatewayID); err != nil {
		log.Printf("ERROR gateway %s: external database cleanup failed on ManagedDatabase %s; role and database %q may remain on the external server and require manual removal (see the external database spec's operator runbook): %v",
			gatewayID, r.cfg.ManagedDatabaseID, externalGatewayDBName(gatewayID), err)
	}
	return nil
}

// externalAdminParams holds the admin connection parameters read from the
// credentials Secret. It is only ever alive for the duration of one reconcile.
type externalAdminParams struct {
	host        string
	port        string
	user        string
	password    string
	dbname      string
	sslmode     string
	sslrootcert string // optional inline PEM CA bundle; enables certificate verification
}

// externalCredentialsNamespacePrefix is the reserved prefix for the namespace
// holding an external server's admin credentials. ManagedDatabase.connection_secret
// names that NAMESPACE, not a Secret: the credentials are provisioned out-of-band,
// normally before HyperShell is installed, so they must not depend on the control
// plane instance namespace existing.
//
// The prefix is a security boundary, not a convention. Combined with the fixed
// Secret name below it bounds what the control plane can be made to read to a
// single deliberately-named Secret inside deliberately-created namespaces,
// preventing an API-level reference from pointing this reconciler at an unrelated
// Secret such as hypershell-db-app. The same values are enforced by the API server
// (plugins/managedDatabases/service.go). See naming-multitenancy.spec.md §6.2.
const externalCredentialsNamespacePrefix = "hypershell-managed-db-"

// externalCredentialsSecretName is the fixed name of the only Secret this
// reconciler reads inside a hypershell-managed-db-<name> namespace.
const externalCredentialsSecretName = "hypershell-managed-db-credentials"

// dns1123LabelMaxLength is the Kubernetes limit for a namespace name.
const dns1123LabelMaxLength = 63

// dns1123LabelPattern matches a valid DNS-1123 label (a valid namespace name).
var dns1123LabelPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Closed-vocabulary status strings for the external DB probe and reconciler.
// Exported so callers (e.g. the ManagedDatabase reconciler) share the same
// vocabulary without re-declaring literals.
const (
	ExternalDBStatusReady                 = "Ready"
	ExternalDBStatusUnreachable           = "Failed: unreachable"
	ExternalDBStatusTLSFailed             = "Failed: tls_failed"
	ExternalDBStatusAuthFailed            = "Failed: auth_failed"
	ExternalDBStatusInsufficientPrivilege = "Failed: insufficient_privilege"
	ExternalDBStatusSecretInvalid         = "Failed: secret_invalid"
)

// validateCredentialsNamespace enforces the connection_secret reference rules
// (no slash, reserved prefix, valid DNS-1123 label) before any read is
// attempted. This runs again here, after API-server validation, so a row
// written before the rule existed cannot cause a read.
func validateCredentialsNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("connection_secret namespace is empty")
	}
	if strings.Contains(namespace, "/") {
		return fmt.Errorf("connection_secret %q contains a '/'; it must be a bare namespace name", namespace)
	}
	if !strings.HasPrefix(namespace, externalCredentialsNamespacePrefix) {
		return fmt.Errorf("connection_secret namespace %q does not start with the required prefix %q", namespace, externalCredentialsNamespacePrefix)
	}
	if len(namespace) > dns1123LabelMaxLength {
		return fmt.Errorf("connection_secret namespace %q exceeds the %d-character namespace name limit", namespace, dns1123LabelMaxLength)
	}
	if !dns1123LabelPattern.MatchString(namespace) {
		return fmt.Errorf("connection_secret namespace %q is not a valid DNS-1123 label", namespace)
	}
	return nil
}

// readExternalAdminSecret validates the credentials namespace and reads the
// fixed-name admin Secret from it. No other Secret in that namespace is read.
func readExternalAdminSecret(ctx context.Context, clientset kubernetes.Interface, credentialsNamespace string) (*externalAdminParams, error) {
	if err := validateCredentialsNamespace(credentialsNamespace); err != nil {
		return nil, fmt.Errorf("connection_secret validation: %w", err)
	}

	secret, err := clientset.CoreV1().Secrets(credentialsNamespace).Get(ctx, externalCredentialsSecretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("secret %q not found in namespace %q", externalCredentialsSecretName, credentialsNamespace)
		}
		return nil, fmt.Errorf("read Secret %q in namespace %q: %w", externalCredentialsSecretName, credentialsNamespace, err)
	}

	get := func(key string) string { return string(secret.Data[key]) }
	required := []string{"host", "port", "user", "password"}
	for _, k := range required {
		if get(k) == "" {
			return nil, fmt.Errorf("secret %q in namespace %q is missing required key %q", externalCredentialsSecretName, credentialsNamespace, k)
		}
	}

	dbname := get("dbname")
	if dbname == "" {
		dbname = "postgres"
	}
	sslmode := get("sslmode")
	if sslmode == "" {
		sslmode = "require"
	}
	switch sslmode {
	case "disable":
		log.Printf("WARN external DB credentials in namespace %s: sslmode=disable is insecure; use verify-full for production", credentialsNamespace)
	case "require", "allow", "prefer":
		log.Printf("WARN external DB credentials in namespace %s: sslmode=%s encrypts the connection but does not verify the server certificate; use verify-full with sslrootcert for production external servers", credentialsNamespace, sslmode)
	}

	return &externalAdminParams{
		host:        get("host"),
		port:        get("port"),
		user:        get("user"),
		password:    get("password"),
		dbname:      dbname,
		sslmode:     sslmode,
		sslrootcert: get("sslrootcert"),
	}, nil
}

// dsn builds the admin connection string. sslrootcert is supplied by the
// operator as inline PEM, but lib/pq only accepts a file-system path for that
// parameter, so the PEM is materialised into a private temp file whose lifetime
// is bounded by the returned cleanup func. cleanup is always non-nil.
func (p *externalAdminParams) dsn() (string, func(), error) {
	noop := func() {}

	// Use URL format so special characters in credentials are safely percent-encoded
	// instead of breaking the space-delimited keyword DSN.
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.user, p.password),
		Host:   net.JoinHostPort(p.host, p.port),
		Path:   "/" + p.dbname,
	}
	q := url.Values{
		"sslmode":         {p.sslmode},
		"connect_timeout": {"10"},
	}

	cleanup := noop
	if p.sslrootcert != "" {
		f, err := os.CreateTemp("", "hypershell-external-db-ca-*.pem")
		if err != nil {
			return "", noop, fmt.Errorf("materialise CA bundle: %w", err)
		}
		path := f.Name()
		cleanup = func() {
			if rerr := os.Remove(path); rerr != nil && !os.IsNotExist(rerr) {
				log.Printf("WARN external DB: remove temporary CA bundle: %v", rerr)
			}
		}
		if err := f.Chmod(0o600); err != nil {
			_ = f.Close()
			cleanup()
			return "", noop, fmt.Errorf("secure CA bundle: %w", err)
		}
		if _, err := f.WriteString(p.sslrootcert); err != nil {
			_ = f.Close()
			cleanup()
			return "", noop, fmt.Errorf("write CA bundle: %w", err)
		}
		if err := f.Close(); err != nil {
			cleanup()
			return "", noop, fmt.Errorf("close CA bundle: %w", err)
		}
		q.Set("sslrootcert", path)
	}

	u.RawQuery = q.Encode()
	return u.String(), cleanup, nil
}

// openAdminConn opens a short-lived PostgreSQL admin connection. The returned
// release func closes the connection and removes any temporary CA bundle;
// callers must always defer it. It is non-nil on every return path.
// PingContext errors are returned unwrapped so callers can errors.As them for
// typed classification (*pq.Error / net.Error) via mapConnErrorToStatus.
func openAdminConn(ctx context.Context, params *externalAdminParams) (*sql.DB, func(), error) {
	noop := func() {}

	dsn, cleanupCA, err := params.dsn()
	if err != nil {
		return nil, noop, err
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		cleanupCA()
		return nil, noop, fmt.Errorf("open admin connection: %w", err)
	}
	db.SetMaxOpenConns(1)

	release := func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARN external DB: close admin connection: %v", cerr)
		}
		// The CA file must outlive the connection: lib/pq reads it when a
		// pooled connection is (re)established, not only at Open.
		cleanupCA()
	}

	if err := db.PingContext(ctx); err != nil {
		release()
		return nil, noop, err
	}
	return db, release, nil
}

// mapConnErrorToStatus maps a raw connection error to a closed-vocabulary
// ManagedDatabase status string. The raw error is never returned.
func mapConnErrorToStatus(err error) string {
	if err == nil {
		return ExternalDBStatusReady
	}
	msg := err.Error()
	lower := strings.ToLower(msg)

	// Network-level failures: typed check first, then best-effort string matching
	// for errors the driver returns as unwrapped plain strings.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return ExternalDBStatusUnreachable
	}
	if strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "i/o timeout") ||
		strings.Contains(lower, "network") {
		return ExternalDBStatusUnreachable
	}
	// Auth failures: typed SQLSTATE check runs before TLS string matching so that
	// a pq.Error with code 28P01/28000 is always classified auth_failed even when
	// its message incidentally contains "ssl" (e.g. SSL-wrapped auth rejections).
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "28P01", "28000": // invalid_password, invalid_authorization_specification
			return ExternalDBStatusAuthFailed
		}
	}
	// TLS/auth best-effort string matching: only reached for unwrapped driver errors
	// where the typed checks above did not match. Result is a status label only
	// (no control-flow branch on it), so a misclassification has low impact.
	if strings.Contains(lower, "tls") ||
		strings.Contains(lower, "certificate") ||
		strings.Contains(lower, "x509") ||
		strings.Contains(lower, "ssl") {
		return ExternalDBStatusTLSFailed
	}
	if strings.Contains(lower, "password authentication failed") ||
		strings.Contains(lower, "28p01") ||
		strings.Contains(lower, "28000") {
		return ExternalDBStatusAuthFailed
	}
	// Unclassified errors: deliberately collapsed to unreachable. This is a
	// status label only; no control-flow branch depends on the value, so a
	// misclassification has low impact.
	return ExternalDBStatusUnreachable
}

// ProbeExternalServer opens a short-lived admin connection, verifies the
// admin role's CREATEDB and CREATEROLE attributes, and returns a
// closed-vocabulary status string. It is side-effect-free on the server.
func ProbeExternalServer(ctx context.Context, clientset kubernetes.Interface, cfg ExternalDBConfig) string {
	params, err := readExternalAdminSecret(ctx, clientset, cfg.CredentialsNamespace)
	if err != nil {
		log.Printf("INFO external DB probe (namespace %s): %s: %v", cfg.CredentialsNamespace, ExternalDBStatusSecretInvalid, err)
		return ExternalDBStatusSecretInvalid
	}

	db, release, err := openAdminConn(ctx, params)
	if err != nil {
		status := mapConnErrorToStatus(err)
		log.Printf("INFO external DB probe (namespace %s): %s (connection error redacted)", cfg.CredentialsNamespace, status)
		return status
	}
	defer release()

	var rolsuper, rolcreatedb, rolcreaterole bool
	row := db.QueryRowContext(ctx, "SELECT rolsuper, rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname = current_user")
	if err := row.Scan(&rolsuper, &rolcreatedb, &rolcreaterole); err != nil {
		log.Printf("INFO external DB probe (namespace %s): %s (query error redacted)", cfg.CredentialsNamespace, ExternalDBStatusUnreachable)
		return ExternalDBStatusUnreachable
	}

	// Superusers have implicit CREATE DATABASE / CREATE ROLE privileges regardless
	// of rolcreatedb/rolcreaterole, so short-circuit on rolsuper.
	if !rolsuper && (!rolcreatedb || !rolcreaterole) {
		log.Printf("INFO external DB probe (namespace %s): %s (rolsuper=%v rolcreatedb=%v rolcreaterole=%v)",
			cfg.CredentialsNamespace, ExternalDBStatusInsufficientPrivilege, rolsuper, rolcreatedb, rolcreaterole)
		return ExternalDBStatusInsufficientPrivilege
	}

	log.Printf("INFO external DB probe (namespace %s): %s", cfg.CredentialsNamespace, ExternalDBStatusReady)
	return ExternalDBStatusReady
}

// externalGatewayDBName returns the PostgreSQL role/database name for a
// gateway. Role and database share the name.
func externalGatewayDBName(gatewayID string) string {
	return "gw_" + strings.ToLower(gatewayID)
}

// tenantGatewayDBSecretName is the tenant-namespace Secret the gateway workload
// consumes as --db-url.
const tenantGatewayDBSecretName = "openshell-gateway-db-credentials"

// ReconcileExternalDatabaseResources provisions a dedicated role and database
// on the external server for gatewayID, then writes the tenant credentials
// Secret. It is idempotent: re-running against an already-provisioned gateway
// makes no destructive change and does not regenerate the password.
func ReconcileExternalDatabaseResources(
	ctx context.Context,
	clientset kubernetes.Interface,
	tenantNamespace string,
	gatewayID string,
	cfg ExternalDBConfig,
) error {
	params, err := readExternalAdminSecret(ctx, clientset, cfg.CredentialsNamespace)
	if err != nil {
		return fmt.Errorf("read external admin credentials: %w", err)
	}

	pgName := externalGatewayDBName(gatewayID)

	// Determine password: reuse from existing tenant Secret (create-or-skip),
	// or generate a new one.
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

	// Open admin connection and issue idempotent DDL.
	db, release, err := openAdminConn(ctx, params)
	if err != nil {
		return fmt.Errorf("connect to external server (%s): connection failed (credentials redacted)", mapConnErrorToStatus(err))
	}
	defer release()

	// Role: create if absent; if present but we generated a new password
	// (because the Secret was missing), apply the new password to the role.
	var roleExists bool
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", pgName,
	).Scan(&roleExists); err != nil {
		return fmt.Errorf("check role existence for gateway %s: %w", gatewayID, err)
	}

	if !roleExists {
		// lib/pq cannot parameterize CREATE ROLE / ALTER ROLE, so the password is
		// interpolated into the statement text. On servers with log_statement=all
		// the password will appear in the server log; operators should restrict
		// log verbosity or use server-side log redaction accordingly.
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", pgQuoteIdent(pgName), pgQuoteLiteral(password)),
		); err != nil {
			return fmt.Errorf("CREATE ROLE for gateway %s: DDL execution failed (credentials redacted)", gatewayID)
		}
		log.Printf("INFO created external DB role %s for gateway %s", pgName, gatewayID)
	} else if freshPassword {
		// Secret was absent but the role exists: sync the new password to
		// PostgreSQL so the gateway can authenticate with the Secret we are about
		// to write. This is provisioning repair, NOT credential rotation --
		// external mode has no rotation (see the external database spec). Do not
		// route the rotate-db-credentials annotation here.
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf("ALTER ROLE %s PASSWORD '%s'", pgQuoteIdent(pgName), pgQuoteLiteral(password)),
		); err != nil {
			return fmt.Errorf("ALTER ROLE password for gateway %s: DDL execution failed (credentials redacted)", gatewayID)
		}
		log.Printf("INFO repaired external DB role password for gateway %s (tenant Secret was absent)", gatewayID)
	}
	// Out-of-band password drift (the tenant Secret exists but the server-side role
	// password was changed externally) is not reconciled: detecting it would require
	// a round-trip login on every reconcile, which is too expensive against a shared
	// server. Recover by deleting the tenant Secret, which makes the branch above
	// re-apply a fresh password on the next reconcile.

	// Database: create if absent.
	var dbExists bool
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", pgName,
	).Scan(&dbExists); err != nil {
		return fmt.Errorf("check database existence for gateway %s: %w", gatewayID, err)
	}
	if !dbExists {
		// CREATE DATABASE cannot run inside a transaction block and has no IF NOT EXISTS.
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf("CREATE DATABASE %s OWNER %s", pgQuoteIdent(pgName), pgQuoteIdent(pgName)),
		); err != nil {
			return fmt.Errorf("CREATE DATABASE for gateway %s: %w", gatewayID, err)
		}
		log.Printf("INFO created external DB database %s for gateway %s", pgName, gatewayID)
	}

	// Isolation: revoke PUBLIC connect, grant only the gateway role.
	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC", pgQuoteIdent(pgName)),
	); err != nil {
		return fmt.Errorf("REVOKE CONNECT for gateway %s: %w", gatewayID, err)
	}
	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", pgQuoteIdent(pgName), pgQuoteIdent(pgName)),
	); err != nil {
		return fmt.Errorf("GRANT CONNECT for gateway %s: %w", gatewayID, err)
	}

	// Write or refresh the tenant credentials Secret.
	//
	// Tenant TLS: cap at "require" when the admin connection uses "verify-full".
	// Verifying the server certificate from the gateway pod needs the CA bundle
	// mounted into that pod, which is the deferred CA-distribution follow-up in
	// the spec. Until then the tenant connection is encrypted but unverified.
	tenantSSLMode := params.sslmode
	if tenantSSLMode == "verify-full" || tenantSSLMode == "verify-ca" {
		tenantSSLMode = "require"
	}
	tenantQ := url.Values{"sslmode": {tenantSSLMode}}
	tenantBase := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s",
		url.QueryEscape(pgName), url.QueryEscape(password), params.host, params.port, pgName)
	dbURI := tenantBase + "?" + tenantQ.Encode()

	desiredData := map[string][]byte{
		"host":     []byte(params.host),
		"port":     []byte(params.port),
		"dbname":   []byte(pgName),
		"user":     []byte(pgName),
		"password": []byte(password),
		"sslmode":  []byte(tenantSSLMode),
		"uri":      []byte(dbURI),
	}
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
		log.Printf("INFO created external gateway credentials secret %s in %s", tenantGatewayDBSecretName, tenantNamespace)
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

	log.Printf("INFO external DB provisioning complete for gateway %s in namespace %s", gatewayID, tenantNamespace)
	return nil
}

// DeleteExternalDatabaseResources terminates active connections and drops the
// gateway's database and role on the external server. Idempotent: absent
// objects are treated as success.
//
// A non-nil error means the objects may still exist on the server. Deletion is
// single-shot (see externalDatabaseReconciler.Delete), so the caller logs the
// failure rather than scheduling a retry.
func DeleteExternalDatabaseResources(
	ctx context.Context,
	clientset kubernetes.Interface,
	cfg ExternalDBConfig,
	gatewayID string,
) error {
	if cfg.CredentialsNamespace == "" || gatewayID == "" {
		return nil
	}

	params, err := readExternalAdminSecret(ctx, clientset, cfg.CredentialsNamespace)
	if err != nil {
		return fmt.Errorf("cannot read external admin credentials: %w", err)
	}

	db, release, err := openAdminConn(ctx, params)
	if err != nil {
		return fmt.Errorf("cannot connect to external server (%s): connection failed (credentials redacted)", mapConnErrorToStatus(err))
	}
	defer release()

	pgName := externalGatewayDBName(gatewayID)

	// Terminate active backends so DROP DATABASE is not blocked.
	// Terminating another role's backends needs superuser or pg_signal_backend
	// membership; a failure here is not fatal because DROP DATABASE ... WITH
	// (FORCE) below performs the same termination server-side.
	if _, err := db.ExecContext(ctx,
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
		pgName,
	); err != nil {
		log.Printf("WARN external DB cleanup for gateway %s: terminate backends failed (proceeding): %v", gatewayID, err)
	}

	// Drop the gateway database. This is a cluster-level operation and must
	// come before DROP ROLE because the role owns the database.
	var dbExists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", pgName).Scan(&dbExists); err == nil && dbExists {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", pgQuoteIdent(pgName))); err != nil {
			return fmt.Errorf("DROP DATABASE failed: %w", err)
		}
		log.Printf("INFO dropped external database %s for gateway %s", pgName, gatewayID)
	}

	// Drop role (safe once the database it owned is gone).
	var roleExists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", pgName).Scan(&roleExists); err == nil && roleExists {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP ROLE %s", pgQuoteIdent(pgName))); err != nil {
			return fmt.Errorf("DROP ROLE failed: %w", err)
		}
		log.Printf("INFO dropped external role %s for gateway %s", pgName, gatewayID)
	}
	return nil
}

// pgQuoteIdent quotes a PostgreSQL identifier to prevent SQL injection.
// Only safe for identifiers produced from internal gateway IDs.
func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// pgQuoteLiteral quotes a string literal for use in SQL by doubling single
// quotes. Safe here because every interpolated value is a hex-encoded password
// (character set [0-9a-f]); DO NOT reuse this function for arbitrary user input.
func pgQuoteLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
