//go:build integration

package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestControllerDatabaseLifecycle uses a real TLS server and a non-superuser
// account. Run it with scripts/test-controller-database-secret.sh.
func TestControllerDatabaseLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	ca, err := os.ReadFile(os.Getenv("TEST_DATABASE_CA"))
	if err != nil {
		t.Fatal("TEST_DATABASE_CA must name the fixture CA")
	}
	port := os.Getenv("TEST_DATABASE_PORT")
	if port == "" {
		t.Fatal("TEST_DATABASE_PORT must name the fixture port")
	}
	cfg := ExternalDBConfig{CredentialsNamespace: "hypershell", CredentialsSecretName: "gateway-database-admin"}
	admin := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cfg.CredentialsSecretName, Namespace: cfg.CredentialsNamespace},
		Data: map[string][]byte{"host": []byte("localhost"), "port": []byte(port), "user": []byte("provisioner"),
			"password": []byte("fixture-admin"), "sslrootcert": ca}}
	client := k8sfake.NewSimpleClientset(admin)
	params, err := readExternalAdminCredentials(ctx, client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	db, release, err := openAdminConn(ctx, params)
	if err != nil {
		t.Fatal("fixture admin cannot connect")
	}
	t.Cleanup(release)
	var superuser bool
	if err := db.QueryRowContext(ctx, "SELECT rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&superuser); err != nil || superuser {
		t.Fatalf("fixture must use a non-superuser: %v", err)
	}
	gatewayID := fmt.Sprintf("test%d", time.Now().UnixNano())
	name := externalGatewayDBName(gatewayID)
	tenantNS := "gateway-test"
	reconcile := func() error { return ReconcileExternalDatabaseResources(ctx, client, tenantNS, gatewayID, cfg) }
	cleanup := func() error { return (&externalDatabaseReconciler{cfg: cfg}).Delete(ctx, nil, client, gatewayID) }
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	})
	if err := reconcile(); err != nil {
		t.Fatal(err)
	}
	tenant, err := client.CoreV1().Secrets(tenantNS).Get(ctx, tenantGatewayDBSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	firstPassword := string(tenant.Data["password"])
	if string(tenant.Data["sslmode"]) != "verify-full" || string(tenant.Data["sslrootcert"]) != string(ca) {
		t.Fatal("tenant TLS configuration is incomplete")
	}
	if err := reconcile(); err != nil {
		t.Fatal(err)
	}
	tenant, err = client.CoreV1().Secrets(tenantNS).Get(ctx, tenantGatewayDBSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(tenant.Data["password"]) != firstPassword {
		t.Fatal("reconcile changed tenant password")
	}
	uri, err := url.Parse(string(tenant.Data["uri"]))
	if err != nil {
		t.Fatal("invalid tenant URI")
	}
	q := uri.Query()
	q.Set("sslrootcert", os.Getenv("TEST_DATABASE_CA"))
	uri.RawQuery = q.Encode()
	tenantDB, err := sql.Open("postgres", uri.String())
	if err != nil {
		t.Fatal("invalid tenant connection")
	}
	defer func() { _ = tenantDB.Close() }()
	var tls bool
	if err := tenantDB.QueryRowContext(ctx, "SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()").Scan(&tls); err != nil || !tls {
		t.Fatalf("tenant TLS connection failed: %v", err)
	}
	if _, err := tenantDB.ExecContext(ctx, "CREATE TABLE lifecycle_test (id integer PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}

	// Reject a trusted certificate for a different hostname.
	badHost := *params
	badHost.host = "127.0.0.1"
	if _, closeBad, err := openAdminConn(ctx, &badHost); err == nil {
		closeBad()
		t.Fatal("hostname mismatch was accepted")
	}
	badCA := *params
	badCA.sslrootcert = validTestCA(t)
	if _, closeBad, err := openAdminConn(ctx, &badCA); err == nil {
		closeBad()
		t.Fatal("untrusted CA was accepted")
	}
	badCAPath := filepath.Join(t.TempDir(), "untrusted.pem")
	if err := os.WriteFile(badCAPath, []byte(badCA.sslrootcert), 0600); err != nil {
		t.Fatal(err)
	}
	q.Set("sslrootcert", badCAPath)
	uri.RawQuery = q.Encode()
	rejected, err := sql.Open("postgres", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := rejected.PingContext(ctx); err == nil {
		t.Fatal("tenant accepted an untrusted CA")
	}
	_ = rejected.Close()

	q.Set("sslrootcert", os.Getenv("TEST_DATABASE_CA"))
	uri.Host = "127.0.0.1:" + port
	uri.RawQuery = q.Encode()
	wrongHost, err := sql.Open("postgres", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := wrongHost.PingContext(ctx); err == nil {
		t.Fatal("tenant accepted a hostname mismatch")
	}
	_ = wrongHost.Close()

	// The next operation must read the new password from the Secret.
	if _, err := db.ExecContext(ctx, "ALTER ROLE provisioner PASSWORD 'fixture-rotated'"); err != nil {
		t.Fatal("fixture password change failed")
	}
	if err := reconcile(); err == nil {
		t.Fatal("stale admin password was accepted")
	}
	admin.Data["password"] = []byte("fixture-rotated")
	if _, err := client.CoreV1().Secrets(cfg.CredentialsNamespace).Update(ctx, admin, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := reconcile(); err != nil {
		t.Fatal(err)
	}

	// A missing Secret must fail both operations and must allow a later retry.
	if err := client.CoreV1().Secrets(cfg.CredentialsNamespace).Delete(ctx, cfg.CredentialsSecretName, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := reconcile(); err == nil {
		t.Fatal("missing Secret provisioned a database")
	}
	if err := cleanup(); err == nil {
		t.Fatal("missing Secret hid failed cleanup")
	}
	if _, err := client.CoreV1().Secrets(cfg.CredentialsNamespace).Create(ctx, admin, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	// A role with an extra object cannot be dropped. The database drop succeeds;
	// role failure must be returned, then a retry must remove the remaining role.
	object := pgQuoteIdent(name + "_blocker")
	if _, err := db.ExecContext(ctx, "CREATE DATABASE "+object+" OWNER "+pgQuoteIdent(name)); err != nil {
		t.Fatal(err)
	}

	if err := cleanup(); err == nil || !strings.Contains(err.Error(), "DROP ROLE") {
		t.Fatalf("expected DROP ROLE error: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DROP DATABASE "+object); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup is not idempotent: %v", err)
	}
	var remains bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1) OR EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", name).Scan(&remains); err != nil || remains {
		t.Fatalf("database or role remains: %v", err)
	}
}
