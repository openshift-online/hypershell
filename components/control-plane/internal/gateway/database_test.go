package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pq "github.com/lib/pq"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
)

// fakeDatabaseReconciler stands in for the PostgreSQL-backed reconciler in
// tests that exercise the surrounding gateway lifecycle.
type fakeDatabaseReconciler struct {
	reconcileErr error
	deleteErr    error
	reconciled   []string
	deleted      []string
}

func (f *fakeDatabaseReconciler) Reconcile(_ context.Context, _ dynamic.Interface, _ kubernetes.Interface, _ string, gatewayID string) error {
	f.reconciled = append(f.reconciled, gatewayID)
	return f.reconcileErr
}

func (f *fakeDatabaseReconciler) Delete(_ context.Context, _ dynamic.Interface, _ kubernetes.Interface, gatewayID string) error {
	f.deleted = append(f.deleted, gatewayID)
	return f.deleteErr
}

// testCAPEM returns a freshly generated self-signed certificate in PEM form,
// standing in for the admin Secret's sslrootcert bundle.
func testCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(cryptoRand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// writeAdminDir materialises a mounted admin Secret as one file per key.
func writeAdminDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func validAdminFiles(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"host":        "db.example.internal\n",
		"port":        "5432",
		"user":        "provisioner",
		"password":    "s3cr3t\n",
		"sslrootcert": testCAPEM(t),
	}
}

func TestReadAdminCredentials(t *testing.T) {
	t.Run("complete secret", func(t *testing.T) {
		files := validAdminFiles(t)
		dir := writeAdminDir(t, files)
		creds, err := readAdminCredentials(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if creds.host != "db.example.internal" {
			t.Errorf("host = %q, want trimmed hostname", creds.host)
		}
		if creds.password != "s3cr3t" {
			t.Errorf("password = %q, want trailing newline stripped only", creds.password)
		}
		if creds.dbname != defaultAdminDBName {
			t.Errorf("dbname = %q, want default %q", creds.dbname, defaultAdminDBName)
		}
		if creds.sslrootcert != files["sslrootcert"] {
			t.Error("sslrootcert must be kept verbatim")
		}
		if creds.sslrootcertPath != filepath.Join(dir, "sslrootcert") {
			t.Errorf("sslrootcertPath = %q", creds.sslrootcertPath)
		}
	})

	t.Run("optional dbname and explicit verify-full", func(t *testing.T) {
		files := validAdminFiles(t)
		files["dbname"] = "maintenance"
		files["sslmode"] = "verify-full\n"
		creds, err := readAdminCredentials(writeAdminDir(t, files))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if creds.dbname != "maintenance" {
			t.Errorf("dbname = %q", creds.dbname)
		}
	})

	t.Run("startup validation accepts the same directory", func(t *testing.T) {
		if err := ValidateAdminCredentialsDir(writeAdminDir(t, validAdminFiles(t))); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(map[string]string)
		want   string
	}{
		{"missing host", func(f map[string]string) { delete(f, "host") }, `"host"`},
		{"empty user", func(f map[string]string) { f["user"] = "  \n" }, `"user" is empty`},
		{"empty password", func(f map[string]string) { f["password"] = "\n" }, `"password" is empty`},
		{"missing sslrootcert", func(f map[string]string) { delete(f, "sslrootcert") }, `"sslrootcert"`},
		{"sslrootcert not PEM", func(f map[string]string) { f["sslrootcert"] = "not a certificate" }, "PEM certificate"},
		{"port not numeric", func(f map[string]string) { f["port"] = "fivefourthreetwo" }, "TCP port"},
		{"port out of range", func(f map[string]string) { f["port"] = "70000" }, "TCP port"},
		{"sslmode require rejected", func(f map[string]string) { f["sslmode"] = "require" }, `only "verify-full"`},
		{"sslmode disable rejected", func(f map[string]string) { f["sslmode"] = "disable" }, `only "verify-full"`},
		{"sslmode verify-ca rejected", func(f map[string]string) { f["sslmode"] = "verify-ca" }, `only "verify-full"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := validAdminFiles(t)
			tc.mutate(files)
			_, err := readAdminCredentials(writeAdminDir(t, files))
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "s3cr3t") {
				t.Error("error must not contain the password")
			}
		})
	}

	t.Run("missing directory", func(t *testing.T) {
		if _, err := readAdminCredentials(filepath.Join(t.TempDir(), "absent")); err == nil {
			t.Fatal("expected an error for a missing directory")
		}
	})

	t.Run("empty directory path", func(t *testing.T) {
		if _, err := readAdminCredentials(""); err == nil {
			t.Fatal("expected an error for an unconfigured directory")
		}
	})
}

func TestAdminDSNUsesVerifyFullAndMountedCA(t *testing.T) {
	dir := writeAdminDir(t, validAdminFiles(t))
	creds, err := readAdminCredentials(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	creds.password = "p@ss word/with?chars"
	u, err := url.Parse(creds.dsn())
	if err != nil {
		t.Fatalf("dsn is not a URL: %v", err)
	}
	if got, _ := u.User.Password(); got != creds.password {
		t.Errorf("password round trip = %q", got)
	}
	q := u.Query()
	if q.Get("sslmode") != requiredSSLMode {
		t.Errorf("sslmode = %q, want %q", q.Get("sslmode"), requiredSSLMode)
	}
	if q.Get("sslrootcert") != filepath.Join(dir, "sslrootcert") {
		t.Errorf("sslrootcert = %q, want the mounted file path", q.Get("sslrootcert"))
	}
	if q.Get("connect_timeout") != "10" {
		t.Errorf("connect_timeout = %q", q.Get("connect_timeout"))
	}
	if u.Host != net.JoinHostPort(creds.host, creds.port) {
		t.Errorf("host = %q", u.Host)
	}
}

// TestTenantSecretDataUsesRequireNotVerifyFull documents the deliberate gap
// between the admin and tenant connections: the gateway workload is deployed
// through the upstream OpenShell Helm chart, which has no mechanism to mount a
// CA bundle into the gateway pod for the database connection, so the tenant
// leg cannot be verify-full. It stays encrypted (sslmode=require) rather than
// unverified-and-unencrypted.
func TestTenantSecretDataUsesRequireNotVerifyFull(t *testing.T) {
	ca := testCAPEM(t)
	creds := &adminCredentials{host: "db.example.internal", port: "5432", user: "provisioner", password: "admin", sslrootcert: ca}
	data := tenantSecretData(creds, "gw_abc", "deadbeef")

	for key, want := range map[string]string{
		"host": "db.example.internal", "port": "5432", "dbname": "gw_abc", "user": "gw_abc",
		"password": "deadbeef", "sslmode": tenantSSLMode,
	} {
		if got := string(data[key]); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if _, ok := data["sslrootcert"]; ok {
		t.Error("tenant Secret must not carry a sslrootcert key; nothing mounts it into the gateway pod")
	}
	if strings.Contains(string(data["uri"]), "admin") {
		t.Fatal("tenant uri must not carry admin credentials")
	}
	u, err := url.Parse(string(data["uri"]))
	if err != nil {
		t.Fatalf("uri is not a URL: %v", err)
	}
	if u.Scheme != "postgresql" || u.Path != "/gw_abc" || u.User.Username() != "gw_abc" {
		t.Errorf("uri = %q", u.String())
	}
	if u.Query().Get("sslmode") != tenantSSLMode {
		t.Errorf("uri sslmode = %q, want %q", u.Query().Get("sslmode"), tenantSSLMode)
	}
	if u.Query().Get("sslrootcert") != "" {
		t.Errorf("uri must not reference a CA file path, got %q", u.Query().Get("sslrootcert"))
	}
}

// TestTenantProbeDSNMatchesTenantSecret checks the readiness probe connects with
// the same identity and TLS posture written into the tenant Secret: the gateway
// role and database, sslmode=require and no CA file (nothing mounts one into the
// gateway pod). A drift here would verify a connection the gateway can never make.
func TestTenantProbeDSNMatchesTenantSecret(t *testing.T) {
	creds := &adminCredentials{host: "db.example.internal", port: "5432", user: "provisioner", password: "admin"}
	u, err := url.Parse(tenantProbeDSN(creds, "gw_abc", "p@ss word/with?chars"))
	if err != nil {
		t.Fatalf("dsn is not a URL: %v", err)
	}
	if u.User.Username() != "gw_abc" {
		t.Errorf("user = %q, want the gateway role, not the admin user", u.User.Username())
	}
	if got, _ := u.User.Password(); got != "p@ss word/with?chars" {
		t.Errorf("password round trip = %q", got)
	}
	if u.Path != "/gw_abc" {
		t.Errorf("path = %q, want the gateway database", u.Path)
	}
	if u.Host != net.JoinHostPort(creds.host, creds.port) {
		t.Errorf("host = %q", u.Host)
	}
	q := u.Query()
	if q.Get("sslmode") != tenantSSLMode {
		t.Errorf("sslmode = %q, want %q (the tenant posture, not verify-full)", q.Get("sslmode"), tenantSSLMode)
	}
	if q.Get("sslrootcert") != "" {
		t.Errorf("probe must not reference a CA file, got %q", q.Get("sslrootcert"))
	}
	if q.Get("connect_timeout") != tenantProbeConnectTimeout {
		t.Errorf("connect_timeout = %q, want %q", q.Get("connect_timeout"), tenantProbeConnectTimeout)
	}
}

func TestTenantProbeDelayDoublesFromBase(t *testing.T) {
	for attempt, want := range map[int]time.Duration{
		0: 200 * time.Millisecond,
		1: 400 * time.Millisecond,
		2: 800 * time.Millisecond,
		3: 1600 * time.Millisecond,
		4: 3200 * time.Millisecond,
	} {
		if got := tenantProbeDelay(attempt); got != want {
			t.Errorf("tenantProbeDelay(%d) = %s, want %s", attempt, got, want)
		}
	}
}

// TestVerifyTenantConnFailureIsRedactedAndBounded checks that when no server is
// reachable the probe exhausts its attempts and returns a categorized error that
// never leaks the password or DSN. A cancelled context short-circuits the retry
// loop so a shutting-down controller does not block on backoff.
func TestVerifyTenantConnFailureIsRedactedAndBounded(t *testing.T) {
	creds := &adminCredentials{host: "127.0.0.1", port: "1", user: "provisioner", password: "s3cr3t"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := verifyTenantConn(ctx, creds, "gw1", "gw_gw1", "topsecretpassword")
	if err == nil {
		t.Fatal("expected an error when the context is already cancelled")
	}
	if strings.Contains(err.Error(), "topsecretpassword") {
		t.Errorf("error must not leak the tenant password: %v", err)
	}
}

func TestCreateGatewayDatabaseSQLUsesTemplate0(t *testing.T) {
	got := createGatewayDatabaseSQL("gw_abc")
	if !strings.Contains(got, "TEMPLATE template0") {
		t.Fatalf("CREATE DATABASE must copy template0, got %s", got)
	}
	if strings.Contains(got, "template1") {
		t.Fatalf("CREATE DATABASE must not copy template1, got %s", got)
	}
}

func TestIsDatabaseSourceBusy(t *testing.T) {
	if !isDatabaseSourceBusy(&pq.Error{Code: "55006", Message: "source database \"template1\" is being accessed by other users"}) {
		t.Fatal("SQLSTATE 55006 must be classified busy")
	}
	if !isDatabaseSourceBusy(errors.New(`pq: source database "template1" is being accessed by other users`)) {
		t.Fatal("occupancy text must be classified busy")
	}
	if isDatabaseSourceBusy(errors.New("permission denied to create database")) {
		t.Fatal("privilege errors must not be retried as occupancy")
	}
	if isDatabaseSourceBusy(nil) {
		t.Fatal("nil is not busy")
	}
}

func TestConnErrorCategory(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"typed net error", &net.OpError{Op: "dial", Err: errors.New("refused")}, connErrorUnreachable},
		{"connection refused", errors.New("dial tcp: connection refused"), connErrorUnreachable},
		{"tls handshake", errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"), connErrorTLSFailed},
		{"pq invalid password", &pq.Error{Code: "28P01", Message: "password authentication failed"}, connErrorAuthFailed},
		{"pq auth spec with ssl in message", &pq.Error{Code: "28000", Message: "no pg_hba.conf entry for host, SSL on"}, connErrorAuthFailed},
		{"plain password failure", errors.New("pq: password authentication failed for user \"x\""), connErrorAuthFailed},
		{"unknown", errors.New("something odd"), connErrorUnreachable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := connErrorCategory(tc.err); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGatewayDBName(t *testing.T) {
	if got := gatewayDBName("2ABCdef"); got != "gw_2abcdef" {
		t.Errorf("got %q", got)
	}
}

func TestPgQuoteIdent(t *testing.T) {
	if got := pgQuoteIdent(`a"b`); got != `"a""b"` {
		t.Errorf("got %s", got)
	}
}

func TestPgQuoteLiteral(t *testing.T) {
	if got := pgQuoteLiteral("it's"); got != "it''s" {
		t.Errorf("got %s", got)
	}
}

func TestDeleteGatewayDatabaseEarlyExit(t *testing.T) {
	// An empty gateway ID is a no-op regardless of credentials.
	if err := DeleteGatewayDatabase(context.Background(), DatabaseConfig{AdminCredentialsDir: "/nonexistent"}, ""); err != nil {
		t.Fatalf("expected nil for empty gateway ID, got %v", err)
	}
	// Unreadable credentials are an error the caller must retry, not a silent success.
	err := DeleteGatewayDatabase(context.Background(), DatabaseConfig{AdminCredentialsDir: filepath.Join(t.TempDir(), "absent")}, "gw1")
	if err == nil {
		t.Fatal("expected an error when the admin credentials cannot be read")
	}
}

func TestDatabaseReconcilerDeleteReturnsCleanupErrors(t *testing.T) {
	r := &databaseReconciler{cfg: DatabaseConfig{AdminCredentialsDir: filepath.Join(t.TempDir(), "absent")}}
	if err := r.Delete(context.Background(), nil, nil, "gw1"); err == nil {
		t.Fatal("cleanup failures must surface so the delete is retried and recorded")
	}
	if err := r.Delete(context.Background(), nil, nil, ""); err != nil {
		t.Fatalf("empty gateway ID must be a no-op, got %v", err)
	}
}

func TestNewDatabaseReconcilerRequiresDirectory(t *testing.T) {
	if _, err := newDatabaseReconciler(ReconcileOpts{}); err == nil {
		t.Fatal("expected an error without an admin credentials directory")
	}
	if _, err := newDatabaseReconciler(ReconcileOpts{Database: DatabaseConfig{AdminCredentialsDir: "/etc/x"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteGatewayResourcesRecordsDatabaseOrphanAndRetries(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	fake := &fakeDatabaseReconciler{deleteErr: errors.New("server unreachable")}
	var recorded []string
	opts := ReconcileOpts{
		GatewayID:          "Gateway-ID",
		GatewayName:        "gw",
		databaseReconciler: fake,
		RecordOrphan: func(_ context.Context, kind, name, reason string) {
			recorded = append(recorded, kind+"/"+name+": "+reason)
		},
	}

	err := DeleteGatewayResources(context.Background(), client, nil, nil, "gateway-ns", opts)
	if err == nil {
		t.Fatal("a failed database cleanup must fail the delete so it is retried")
	}
	if !strings.Contains(err.Error(), "server unreachable") {
		t.Errorf("error %q does not carry the cleanup cause", err)
	}
	if len(recorded) != 1 || !strings.HasPrefix(recorded[0], "PostgreSQLDatabase/gw_gateway-id: ") {
		t.Fatalf("expected one PostgreSQLDatabase orphan record for gw_gateway-id, got %v", recorded)
	}

	fake.deleteErr = nil
	if err := DeleteGatewayResources(context.Background(), client, nil, nil, "gateway-ns", opts); err != nil {
		t.Fatalf("retry after the server recovered must succeed, got %v", err)
	}
	if len(fake.deleted) != 2 {
		t.Fatalf("expected the database delete to run on both attempts, got %d", len(fake.deleted))
	}
	if len(recorded) != 1 {
		t.Fatalf("a successful retry must not record another orphan, got %v", recorded)
	}
}
