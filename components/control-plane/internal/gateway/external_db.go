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
	"reflect"
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

func (r *externalDatabaseReconciler) Reconcile(ctx context.Context, _ dynamic.Interface, clientset kubernetes.Interface, tenantNamespace, gatewayID, rotateAnnotation string) error {
	if err := ReconcileExternalDatabaseResources(ctx, clientset, tenantNamespace, gatewayID, r.cfg); err != nil {
		return fmt.Errorf("reconcile external database resources in %s: %w", tenantNamespace, err)
	}
	if rotateAnnotation != "" {
		if err := RotateExternalDatabaseCredentials(ctx, clientset, tenantNamespace, gatewayID, r.cfg, rotateAnnotation); err != nil {
			return fmt.Errorf("rotate external database credentials in %s: %w", tenantNamespace, err)
		}
	}
	return nil
}

func (r *externalDatabaseReconciler) Delete(ctx context.Context, _ dynamic.Interface, clientset kubernetes.Interface, gatewayID string) {
	if gatewayID == "" {
		return
	}
	DeleteExternalDatabaseResources(ctx, clientset, r.cfg, gatewayID)
}

// externalAdminParams holds the admin connection parameters read from the
// connection Secret. It is only ever alive for the duration of one reconcile.
type externalAdminParams struct {
	host        string
	port        string
	user        string
	password    string
	dbname      string
	sslmode     string
	sslrootcert string // optional; enables verify-full when sslmode=verify-full
}

const externalSecretPrefix = "hypershell-managed-db-"

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

// validateExternalSecretName enforces the connection_secret reference rules
// (no slash, reserved prefix) before any read is attempted.
func validateExternalSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("connection_secret name is empty")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("connection_secret %q contains a namespace separator ('/'); use a plain name", name)
	}
	if !strings.HasPrefix(name, externalSecretPrefix) {
		return fmt.Errorf("connection_secret %q does not start with the required prefix %q", name, externalSecretPrefix)
	}
	return nil
}

// readExternalAdminSecret reads and validates the admin connection Secret.
// Returns a sentinel string "secret_invalid" as the second return value on
// validation or missing-key errors so callers can map it to the status
// vocabulary without a separate sentinel type.
func readExternalAdminSecret(ctx context.Context, clientset kubernetes.Interface, namespace, secretName string) (*externalAdminParams, string, error) {
	if err := validateExternalSecretName(secretName); err != nil {
		return nil, "secret_invalid", fmt.Errorf("connection_secret validation: %w", err)
	}

	secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, "secret_invalid", fmt.Errorf("connection_secret %q not found in namespace %q", secretName, namespace)
		}
		return nil, "secret_invalid", fmt.Errorf("read connection_secret %q: %w", secretName, err)
	}

	get := func(key string) string { return string(secret.Data[key]) }
	required := []string{"host", "port", "user", "password"}
	for _, k := range required {
		if get(k) == "" {
			return nil, "secret_invalid", fmt.Errorf("connection_secret %q is missing required key %q", secretName, k)
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
	if sslmode == "disable" {
		log.Printf("WARN external DB secret %s: sslmode=disable is insecure; use require or verify-full for production", secretName)
	}

	return &externalAdminParams{
		host:        get("host"),
		port:        get("port"),
		user:        get("user"),
		password:    get("password"),
		dbname:      dbname,
		sslmode:     sslmode,
		sslrootcert: get("sslrootcert"),
	}, "", nil
}

func (p *externalAdminParams) dsn() string {
	s := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s connect_timeout=10",
		p.host, p.port, p.user, p.password, p.dbname, p.sslmode)
	if p.sslrootcert != "" {
		s += " sslrootcert=" + p.sslrootcert
	}
	return s
}

// openAdminConn opens a short-lived PostgreSQL admin connection. Callers must
// close it. Credentials must not appear in error messages.
func openAdminConn(ctx context.Context, params *externalAdminParams) (*sql.DB, error) {
	db, err := sql.Open("postgres", params.dsn())
	if err != nil {
		return nil, fmt.Errorf("open admin connection: driver init failed")
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// mapConnErrorToStatus maps a raw connection error to a closed-vocabulary
// ManagedDatabase status string. The raw error is never returned.
func mapConnErrorToStatus(err error) string {
	if err == nil {
		return ExternalDBStatusReady
	}
	msg := err.Error()
	lower := strings.ToLower(msg)

	// Network-level failures
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
	// TLS failures
	if strings.Contains(lower, "tls") ||
		strings.Contains(lower, "certificate") ||
		strings.Contains(lower, "x509") ||
		strings.Contains(lower, "ssl") {
		return ExternalDBStatusTLSFailed
	}
	// Auth failures - prefer typed SQLSTATE check over string matching.
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "28P01", "28000": // invalid_password, invalid_authorization_specification
			return ExternalDBStatusAuthFailed
		}
	}
	if strings.Contains(lower, "password authentication failed") ||
		strings.Contains(lower, "28p01") ||
		strings.Contains(lower, "28000") {
		return ExternalDBStatusAuthFailed
	}
	// Privilege (this is checked post-connect, not here)
	return ExternalDBStatusUnreachable
}

// ProbeExternalServer opens a short-lived admin connection, verifies the
// admin role's CREATEDB and CREATEROLE attributes, and returns a
// closed-vocabulary status string.
func ProbeExternalServer(ctx context.Context, clientset kubernetes.Interface, cfg ExternalDBConfig) string {
	params, _, err := readExternalAdminSecret(ctx, clientset, cfg.Namespace, cfg.SecretName)
	if err != nil {
		log.Printf("INFO external DB probe %s: %s", cfg.SecretName, ExternalDBStatusSecretInvalid)
		return ExternalDBStatusSecretInvalid
	}

	db, err := openAdminConn(ctx, params)
	if err != nil {
		status := mapConnErrorToStatus(err)
		log.Printf("INFO external DB probe %s: %s (connection error redacted)", cfg.SecretName, status)
		return status
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARN external DB probe %s: close connection: %v", cfg.SecretName, cerr)
		}
	}()

	var rolcreatedb, rolcreaterole bool
	row := db.QueryRowContext(ctx, "SELECT rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname = current_user")
	if err := row.Scan(&rolcreatedb, &rolcreaterole); err != nil {
		log.Printf("INFO external DB probe %s: %s (query error redacted)", cfg.SecretName, ExternalDBStatusUnreachable)
		return ExternalDBStatusUnreachable
	}

	if !rolcreatedb || !rolcreaterole {
		log.Printf("INFO external DB probe %s: %s (rolcreatedb=%v rolcreaterole=%v)",
			cfg.SecretName, ExternalDBStatusInsufficientPrivilege, rolcreatedb, rolcreaterole)
		return ExternalDBStatusInsufficientPrivilege
	}

	log.Printf("INFO external DB probe %s: %s", cfg.SecretName, ExternalDBStatusReady)
	return ExternalDBStatusReady
}

// externalGatewayDBName returns the PostgreSQL role/database name for a
// gateway.  Both use the same name following the CNPG pattern.
func externalGatewayDBName(gatewayID string) string {
	return "gw_" + strings.ToLower(gatewayID)
}

// ReconcileExternalDatabaseResources provisions a dedicated role and database
// on the external server for gatewayID, then writes the tenant credentials
// Secret. It is idempotent: re-running against an already-provisioned gateway
// makes no destructive change.
func ReconcileExternalDatabaseResources(
	ctx context.Context,
	clientset kubernetes.Interface,
	tenantNamespace string,
	gatewayID string,
	cfg ExternalDBConfig,
) error {
	params, _, err := readExternalAdminSecret(ctx, clientset, cfg.Namespace, cfg.SecretName)
	if err != nil {
		return fmt.Errorf("read external admin secret: %w", err)
	}

	pgName := externalGatewayDBName(gatewayID)
	const gwSecretName = "openshell-gateway-db-credentials"

	// Determine password: reuse from existing tenant Secret (create-or-skip),
	// or generate a new one.
	password := ""
	existingSecret, secretErr := clientset.CoreV1().Secrets(tenantNamespace).Get(ctx, gwSecretName, metav1.GetOptions{})
	if secretErr != nil && !k8serrors.IsNotFound(secretErr) {
		return fmt.Errorf("get gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, secretErr)
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
	db, err := openAdminConn(ctx, params)
	if err != nil {
		return fmt.Errorf("connect to external server: connection failed (credentials redacted)")
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARN external DB provision: close connection: %v", cerr)
		}
	}()

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
		// Secret was absent but role exists: sync the new password to PostgreSQL.
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf("ALTER ROLE %s PASSWORD '%s'", pgQuoteIdent(pgName), pgQuoteLiteral(password)),
		); err != nil {
			return fmt.Errorf("ALTER ROLE password for gateway %s: DDL execution failed (credentials redacted)", gatewayID)
		}
		log.Printf("INFO updated external DB role password for gateway %s (Secret was absent)", gatewayID)
	}

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
	dbURI := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		pgName, url.QueryEscape(password), params.host, params.port, pgName, params.sslmode)
	if params.sslrootcert != "" {
		dbURI += "&sslrootcert=" + url.QueryEscape(params.sslrootcert)
	}

	desiredData := map[string][]byte{
		"host":     []byte(params.host),
		"port":     []byte(params.port),
		"dbname":   []byte(pgName),
		"user":     []byte(pgName),
		"password": []byte(password),
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
				Name:      gwSecretName,
				Namespace: tenantNamespace,
				Labels:    desiredLabels,
			},
			Type: corev1.SecretTypeOpaque,
			Data: desiredData,
		}
		if _, err := clientset.CoreV1().Secrets(tenantNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, err)
		}
		log.Printf("INFO created external gateway credentials secret %s in %s", gwSecretName, tenantNamespace)
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
				return fmt.Errorf("update gateway credentials secret %s/%s: %w", tenantNamespace, gwSecretName, err)
			}
		}
	}

	log.Printf("INFO external DB provisioning complete for gateway %s in namespace %s", gatewayID, tenantNamespace)
	return nil
}

// DeleteExternalDatabaseResources terminates active connections, drops the
// gateway's database and role on the external server. Idempotent: absent
// objects are treated as success.
func DeleteExternalDatabaseResources(
	ctx context.Context,
	clientset kubernetes.Interface,
	cfg ExternalDBConfig,
	gatewayID string,
) {
	if cfg.SecretName == "" || gatewayID == "" {
		return
	}

	params, _, err := readExternalAdminSecret(ctx, clientset, cfg.Namespace, cfg.SecretName)
	if err != nil {
		log.Printf("WARN external DB cleanup for gateway %s: cannot read admin secret (resources may require manual cleanup): %v", gatewayID, err)
		return
	}

	db, err := openAdminConn(ctx, params)
	if err != nil {
		log.Printf("WARN external DB cleanup for gateway %s: cannot connect to server (resources may require manual cleanup)", gatewayID)
		return
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARN external DB cleanup for gateway %s: close connection: %v", gatewayID, cerr)
		}
	}()

	pgName := externalGatewayDBName(gatewayID)

	// Terminate active backends so DROP DATABASE is not blocked.
	if _, err := db.ExecContext(ctx,
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
		pgName,
	); err != nil {
		log.Printf("WARN external DB cleanup for gateway %s: terminate backends failed (proceeding): %v", gatewayID, err)
	}

	// Drop database (guarded by existence check; cannot be in a transaction).
	var dbExists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", pgName).Scan(&dbExists); err == nil && dbExists {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %s", pgQuoteIdent(pgName))); err != nil {
			log.Printf("WARN external DB cleanup for gateway %s: DROP DATABASE failed (attempting role drop): %v", gatewayID, err)
		} else {
			log.Printf("INFO dropped external database %s for gateway %s", pgName, gatewayID)
		}
	}

	// Drop role.
	var roleExists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", pgName).Scan(&roleExists); err == nil && roleExists {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP ROLE %s", pgQuoteIdent(pgName))); err != nil {
			log.Printf("WARN external DB cleanup for gateway %s: DROP ROLE failed: %v", gatewayID, err)
			return
		}
		log.Printf("INFO dropped external role %s for gateway %s", pgName, gatewayID)
	}
}

// RotateExternalDatabaseCredentials generates a new password, applies it to
// the PostgreSQL role, updates the tenant Secret, and records the trigger
// value on the Secret annotation so applyConfigHashAnnotation triggers a
// workload restart.
func RotateExternalDatabaseCredentials(
	ctx context.Context,
	clientset kubernetes.Interface,
	tenantNamespace string,
	gatewayID string,
	cfg ExternalDBConfig,
	triggerValue string,
) error {
	// Idempotency guard: skip if the Secret already records this trigger value,
	// mirroring rotateCNPGDatabaseCredentials. Without this check every reconcile
	// where rotateAnnotation != "" would re-rotate and roll the gateway pod.
	const gwSecretName = "openshell-gateway-db-credentials"
	existingSecret, err := clientset.CoreV1().Secrets(tenantNamespace).Get(ctx, gwSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get gateway credentials secret for rotation %s/%s: %w", tenantNamespace, gwSecretName, err)
	}
	if existingSecret.Annotations["hypershell.redhat.io/last-db-rotation"] == triggerValue {
		log.Printf("DEBUG external DB credentials in %s already rotated at %s, skipping", tenantNamespace, triggerValue)
		return nil
	}

	params, _, err := readExternalAdminSecret(ctx, clientset, cfg.Namespace, cfg.SecretName)
	if err != nil {
		return fmt.Errorf("read external admin secret for rotation: %w", err)
	}

	passwordBytes := make([]byte, 32)
	if _, err := cryptoRand.Read(passwordBytes); err != nil {
		return fmt.Errorf("generate rotation password: %w", err)
	}
	newPassword := hex.EncodeToString(passwordBytes)
	pgName := externalGatewayDBName(gatewayID)

	db, err := openAdminConn(ctx, params)
	if err != nil {
		return fmt.Errorf("connect to external server for rotation: connection failed (credentials redacted)")
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARN external DB rotation for gateway %s: close connection: %v", gatewayID, cerr)
		}
	}()

	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("ALTER ROLE %s PASSWORD '%s'", pgQuoteIdent(pgName), pgQuoteLiteral(newPassword)),
	); err != nil {
		return fmt.Errorf("ALTER ROLE for rotation of gateway %s: DDL execution failed (credentials redacted)", gatewayID)
	}
	log.Printf("INFO rotated external DB password for gateway %s", gatewayID)

	dbURI := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		pgName, url.QueryEscape(newPassword), params.host, params.port, pgName, params.sslmode)
	if params.sslrootcert != "" {
		dbURI += "&sslrootcert=" + url.QueryEscape(params.sslrootcert)
	}

	updated := existingSecret.DeepCopy()
	if updated.Data == nil {
		updated.Data = map[string][]byte{}
	}
	updated.Data["password"] = []byte(newPassword)
	updated.Data["uri"] = []byte(dbURI)
	if updated.Annotations == nil {
		updated.Annotations = map[string]string{}
	}
	updated.Annotations["hypershell.redhat.io/last-db-rotation"] = triggerValue

	if _, err := clientset.CoreV1().Secrets(tenantNamespace).Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update gateway credentials secret after rotation %s/%s: %w", tenantNamespace, gwSecretName, err)
	}
	log.Printf("INFO updated gateway credentials secret after external rotation for gateway %s", gatewayID)
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
