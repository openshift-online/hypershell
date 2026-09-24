package gateway

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestReconcileGatewayDatabaseWhileTemplate1Occupied is the Kind e2e
// DatabaseReady flake: CREATE DATABASE copies template1 by default, so a
// concurrent session on template1 (another gateway's CREATE DATABASE, autovacuum,
// or a leftover backend) fails provisioning with "source database is being
// accessed by other users" for as long as that session lasts. The production
// path must succeed anyway.
func TestReconcileGatewayDatabaseWhileTemplate1Occupied(t *testing.T) {
	cli, ok := containerCLI()
	if !ok {
		t.Skip("docker/podman is not available")
	}
	root := findRepoRoot(t)
	tlsDir := t.TempDir()
	runCmd(t, exec.Command("bash", filepath.Join(root, "scripts/gen-postgres-tls.sh"),
		tlsDir, "localhost", "127.0.0.1"))
	if err := os.Chmod(filepath.Join(tlsDir, "tls.key"), 0o600); err != nil {
		t.Fatalf("chmod tls.key: %v", err)
	}

	cid := startTLSPostgres(t, cli, tlsDir)
	hostPort := postgresHostPort(t, cli, cid)
	waitPostgresReady(t, cli, cid)

	hold := exec.Command(cli, "exec", cid, "psql", "-U", "postgres", "-d", "template1", "-c", "SELECT pg_sleep(60)")
	if err := hold.Start(); err != nil {
		t.Fatalf("occupy template1: %v", err)
	}
	t.Cleanup(func() { _ = hold.Process.Kill(); _, _ = hold.Process.Wait() })
	time.Sleep(500 * time.Millisecond)

	ns := "openshell-e2e-db"
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
	cfg := DatabaseConfig{AdminCredentialsDir: writeAdminDir(t, map[string]string{
		"host":        "localhost",
		"port":        hostPort,
		"user":        "postgres",
		"password":    "test",
		"dbname":      "postgres",
		"sslmode":     "verify-full",
		"sslrootcert": mustRead(t, filepath.Join(tlsDir, "ca.crt")),
	})}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ReconcileGatewayDatabase(ctx, client, ns, "2abCdefGhijkLmNoPqrStuVwxyz", cfg); err != nil {
		t.Fatalf("provision while template1 is occupied: %v", err)
	}
	secret, err := client.CoreV1().Secrets(ns).Get(ctx, tenantGatewayDBSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("tenant secret: %v", err)
	}
	wantDB := gatewayDBName("2abCdefGhijkLmNoPqrStuVwxyz")
	if got := string(secret.Data["dbname"]); got != wantDB {
		t.Fatalf("tenant dbname = %q, want %q", got, wantDB)
	}
}

func containerCLI() (string, bool) {
	for _, name := range []string{"docker", "podman"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if exec.Command(path, "info").Run() == nil {
			return path, true
		}
	}
	return "", false
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getcwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts/gen-postgres-tls.sh")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find repository root (scripts/gen-postgres-tls.sh)")
		}
		dir = parent
	}
}

func startTLSPostgres(t *testing.T, cli, tlsDir string) string {
	t.Helper()
	cmd := exec.Command(cli, "run", "-d", "--rm",
		"-e", "POSTGRES_PASSWORD=test",
		"-e", "POSTGRES_HOST_AUTH_METHOD=scram-sha-256",
		"-v", tlsDir+":/tls:ro",
		"-p", "127.0.0.1::5432",
		"postgres:15",
		"sh", "-c",
		"cp /tls/tls.key /tmp/server.key && chown postgres:postgres /tmp/server.key && chmod 600 /tmp/server.key && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tls/tls.crt -c ssl_key_file=/tmp/server.key",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("start postgres: %v\n%s", err, out)
	}
	cid := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		_ = exec.Command(cli, "rm", "-f", cid).Run()
	})
	return cid
}

func postgresHostPort(t *testing.T, cli, cid string) string {
	t.Helper()
	out, err := exec.Command(cli, "port", cid, "5432").CombinedOutput()
	if err != nil {
		t.Fatalf("port: %v\n%s", err, out)
	}
	_, port, err := net.SplitHostPort(strings.TrimSpace(string(bytes.ReplaceAll(out, []byte("0.0.0.0"), []byte("127.0.0.1")))))
	if err != nil {
		t.Fatalf("parse port %q: %v", out, err)
	}
	return port
}

func waitPostgresReady(t *testing.T, cli, cid string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		out, err := exec.Command(cli, "exec", cid, "pg_isready", "-U", "postgres").CombinedOutput()
		last = string(out)
		if err == nil {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("postgres not ready: %s", last)
}

func runCmd(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", strings.Join(cmd.Args, " "), err, out)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
