package gateway

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"

	pq "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// --- validateCredentialsNamespace ---
//
// connection_secret names the NAMESPACE holding the fixed-name credentials
// Secret, so the value must be a bare, prefixed, valid DNS-1123 label.

func TestValidateCredentialsNamespace(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "namespace slash", input: "hypershell-managed-db-foo/bar", wantErr: true},
		{name: "missing prefix", input: "my-namespace", wantErr: true},
		{name: "control plane namespace rejected", input: "hypershell", wantErr: true},
		{name: "uppercase is not a DNS-1123 label", input: "hypershell-managed-db-Prod", wantErr: true},
		{name: "dots are not allowed in a namespace name", input: "hypershell-managed-db-a.b", wantErr: true},
		{name: "trailing dash", input: "hypershell-managed-db-", wantErr: true},
		{name: "over 63 characters", input: "hypershell-managed-db-" + strings.Repeat("a", 64), wantErr: true},
		{name: "valid", input: "hypershell-managed-db-prod", wantErr: false},
		{name: "valid with numbers", input: "hypershell-managed-db-123abc", wantErr: false},
		{name: "exactly 63 characters", input: "hypershell-managed-db-" + strings.Repeat("a", 63-len("hypershell-managed-db-")), wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCredentialsNamespace(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for input %q: %v", tc.input, err)
			}
		})
	}
}

// --- pgQuoteIdent ---

func TestPgQuoteIdent(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`simple`, `"simple"`},
		{`with"quote`, `"with""quote"`},
		{`gw_abc123`, `"gw_abc123"`},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := pgQuoteIdent(tc.input)
			if got != tc.want {
				t.Errorf("pgQuoteIdent(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- pgQuoteLiteral ---

func TestPgQuoteLiteral(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`abc`, `abc`},
		{`it's`, `it''s`},
		{`''`, `''''`},
		{`abcdef0123456789`, `abcdef0123456789`},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := pgQuoteLiteral(tc.input)
			if got != tc.want {
				t.Errorf("pgQuoteLiteral(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- mapConnErrorToStatus ---

func TestMapConnErrorToStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ExternalDBStatusReady},
		{name: "connection refused", err: errors.New("connection refused"), want: ExternalDBStatusUnreachable},
		{name: "no such host", err: errors.New("dial tcp: no such host"), want: ExternalDBStatusUnreachable},
		{name: "network error", err: errors.New("network unreachable"), want: ExternalDBStatusUnreachable},
		{name: "tls error", err: errors.New("tls: certificate signed by unknown authority"), want: ExternalDBStatusTLSFailed},
		{name: "x509 error", err: errors.New("x509: certificate has expired"), want: ExternalDBStatusTLSFailed},
		{name: "ssl error", err: errors.New("ssl SYSCALL error"), want: ExternalDBStatusTLSFailed},
		{name: "pq auth 28P01", err: &pq.Error{Code: "28P01"}, want: ExternalDBStatusAuthFailed},
		{name: "pq auth 28000", err: &pq.Error{Code: "28000"}, want: ExternalDBStatusAuthFailed},
		// pq.Error with SSL in message must still be auth_failed (typed check beats string match)
		{name: "pq auth 28P01 with ssl message", err: &pq.Error{Code: "28P01", Message: "SSL connection required"}, want: ExternalDBStatusAuthFailed},
		{name: "password authentication failed", err: errors.New("password authentication failed for user foo"), want: ExternalDBStatusAuthFailed},
		{name: "net.Error timeout", err: &fakeNetError{timeout: true}, want: ExternalDBStatusUnreachable},
		{name: "unknown", err: errors.New("some unexpected error"), want: ExternalDBStatusUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapConnErrorToStatus(tc.err)
			if got != tc.want {
				t.Errorf("mapConnErrorToStatus(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

type fakeNetError struct{ timeout bool }

func (e *fakeNetError) Error() string   { return "fake net error" }
func (e *fakeNetError) Timeout() bool   { return e.timeout }
func (e *fakeNetError) Temporary() bool { return false }

var _ net.Error = (*fakeNetError)(nil)

// --- readExternalAdminSecret ---

func TestReadExternalAdminSecret(t *testing.T) {
	ctx := context.Background()
	credentialsNS := "hypershell-managed-db-kind"

	newSecret := func(data map[string][]byte) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: externalCredentialsSecretName, Namespace: credentialsNS},
			Data:       data,
		}
	}

	t.Run("secret not found", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		if _, err := readExternalAdminSecret(ctx, client, credentialsNS); err == nil {
			t.Fatal("expected error for missing secret, got nil")
		}
	})

	t.Run("invalid namespace reference is rejected before any read", func(t *testing.T) {
		// A Secret with the fixed name exists in a namespace that does NOT carry
		// the reserved prefix. Validation must reject the reference rather than
		// read it.
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: externalCredentialsSecretName, Namespace: "kube-system"},
			Data: map[string][]byte{
				"host": []byte("h"), "port": []byte("5432"),
				"user": []byte("u"), "password": []byte("p"),
			},
		}
		client := k8sfake.NewSimpleClientset(secret)
		if _, err := readExternalAdminSecret(ctx, client, "kube-system"); err == nil {
			t.Fatal("expected error for unprefixed namespace, got nil")
		}
	})

	t.Run("only the fixed-name Secret is read", func(t *testing.T) {
		// A differently-named Secret in an otherwise valid namespace must not be
		// picked up: the reachable set is exactly one name.
		other := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "some-other-secret", Namespace: credentialsNS},
			Data: map[string][]byte{
				"host": []byte("h"), "port": []byte("5432"),
				"user": []byte("u"), "password": []byte("p"),
			},
		}
		client := k8sfake.NewSimpleClientset(other)
		if _, err := readExternalAdminSecret(ctx, client, credentialsNS); err == nil {
			t.Fatal("expected error when only a differently-named Secret exists, got nil")
		}
	})

	t.Run("secret missing required key", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset(newSecret(map[string][]byte{
			"host": []byte("db.example.com"),
			"port": []byte("5432"),
			// user and password missing
		}))
		if _, err := readExternalAdminSecret(ctx, client, credentialsNS); err == nil {
			t.Fatal("expected error for missing key, got nil")
		}
	})

	t.Run("valid secret defaults dbname and sslmode", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset(newSecret(map[string][]byte{
			"host":     []byte("db.example.com"),
			"port":     []byte("5432"),
			"user":     []byte("admin"),
			"password": []byte("s3cr3t"),
		}))
		params, err := readExternalAdminSecret(ctx, client, credentialsNS)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if params.dbname != "postgres" {
			t.Errorf("dbname = %q, want %q", params.dbname, "postgres")
		}
		if params.sslmode != "require" {
			t.Errorf("sslmode = %q, want %q", params.sslmode, "require")
		}
	})

	t.Run("valid secret with explicit dbname and sslmode", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset(newSecret(map[string][]byte{
			"host":     []byte("db.example.com"),
			"port":     []byte("5432"),
			"user":     []byte("admin"),
			"password": []byte("s3cr3t"),
			"dbname":   []byte("mydb"),
			"sslmode":  []byte("disable"),
		}))
		params, err := readExternalAdminSecret(ctx, client, credentialsNS)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if params.dbname != "mydb" {
			t.Errorf("dbname = %q, want %q", params.dbname, "mydb")
		}
		if params.sslmode != "disable" {
			t.Errorf("sslmode = %q, want %q", params.sslmode, "disable")
		}
	})

	t.Run("inline PEM sslrootcert is carried through", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset(newSecret(map[string][]byte{
			"host":        []byte("db.example.com"),
			"port":        []byte("5432"),
			"user":        []byte("admin"),
			"password":    []byte("s3cr3t"),
			"sslmode":     []byte("verify-full"),
			"sslrootcert": []byte(testCAPEM),
		}))
		params, err := readExternalAdminSecret(ctx, client, credentialsNS)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if params.sslrootcert != testCAPEM {
			t.Errorf("sslrootcert = %q, want the inline PEM", params.sslrootcert)
		}
	})
}

// --- dsn: URL format, connect_timeout, inline-PEM CA materialisation ---

const testCAPEM = "-----BEGIN CERTIFICATE-----\nZmFrZQ==\n-----END CERTIFICATE-----\n"

func TestDSNConnectTimeout(t *testing.T) {
	p := &externalAdminParams{
		host: "db.example.com", port: "5432",
		user: "admin", password: "pass",
		dbname: "postgres", sslmode: "require",
	}
	dsn, cleanup, err := p.dsn()
	if err != nil {
		t.Fatalf("dsn() unexpected error: %v", err)
	}
	defer cleanup()
	if !strings.Contains(dsn, "connect_timeout=10") {
		t.Errorf("dsn does not contain connect_timeout=10: %s", dsn)
	}
}

func TestDSNURLFormat(t *testing.T) {
	t.Run("special chars in password are percent-encoded", func(t *testing.T) {
		p := &externalAdminParams{host: "h", port: "5432", user: "u", password: "p@ss w0rd!", dbname: "db", sslmode: "require"}
		dsn, cleanup, err := p.dsn()
		if err != nil {
			t.Fatalf("dsn() unexpected error: %v", err)
		}
		defer cleanup()
		if strings.Contains(dsn, "p@ss w0rd!") {
			t.Errorf("password not encoded in dsn: %s", dsn)
		}
		if !strings.Contains(dsn, "p%40ss+w0rd%21") && !strings.Contains(dsn, "p%40ss%20w0rd%21") {
			t.Errorf("expected percent-encoded password in dsn: %s", dsn)
		}
	})
}

func TestDSNSslrootcert(t *testing.T) {
	t.Run("no sslrootcert omitted from dsn", func(t *testing.T) {
		p := &externalAdminParams{host: "h", port: "5432", user: "u", password: "p", dbname: "db", sslmode: "require"}
		dsn, cleanup, err := p.dsn()
		if err != nil {
			t.Fatalf("dsn() unexpected error: %v", err)
		}
		defer cleanup()
		if strings.Contains(dsn, "sslrootcert") {
			t.Errorf("unexpected sslrootcert in dsn: %s", dsn)
		}
	})

	// The operator supplies inline PEM, but lib/pq only accepts a path, so dsn()
	// must materialise the PEM into a real file and point the DSN at it.
	t.Run("inline PEM is materialised to a private file", func(t *testing.T) {
		p := &externalAdminParams{host: "h", port: "5432", user: "u", password: "p", dbname: "db", sslmode: "verify-full", sslrootcert: testCAPEM}
		dsn, cleanup, err := p.dsn()
		if err != nil {
			t.Fatalf("dsn() unexpected error: %v", err)
		}

		parsed, perr := url.Parse(dsn)
		if perr != nil {
			t.Fatalf("dsn is not a valid URL: %v", perr)
		}
		path := parsed.Query().Get("sslrootcert")
		if path == "" {
			t.Fatal("dsn missing sslrootcert parameter")
		}
		if path == testCAPEM {
			t.Fatal("dsn passed inline PEM verbatim; lib/pq needs a file path")
		}

		info, serr := os.Stat(path)
		if serr != nil {
			t.Fatalf("CA bundle not written to %s: %v", path, serr)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("CA bundle permissions = %o, want 600", perm)
		}
		content, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatalf("read CA bundle: %v", rerr)
		}
		if string(content) != testCAPEM {
			t.Errorf("CA bundle content = %q, want the inline PEM", string(content))
		}

		cleanup()
		if _, serr := os.Stat(path); !os.IsNotExist(serr) {
			t.Errorf("cleanup did not remove the CA bundle at %s", path)
		}
	})

	t.Run("cleanup is safe to call when no CA file was written", func(t *testing.T) {
		p := &externalAdminParams{host: "h", port: "5432", user: "u", password: "p", dbname: "db", sslmode: "require"}
		_, cleanup, err := p.dsn()
		if err != nil {
			t.Fatalf("dsn() unexpected error: %v", err)
		}
		cleanup()
		cleanup()
	})
}

// --- DeleteExternalDatabaseResources early-exit paths ---

func TestDeleteExternalDatabaseResourcesEarlyExit(t *testing.T) {
	ctx := context.Background()
	cfg := ExternalDBConfig{CredentialsNamespace: "hypershell-managed-db-kind", ManagedDatabaseID: "md-1"}

	t.Run("empty gatewayID returns nil without connecting", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		if err := DeleteExternalDatabaseResources(ctx, client, cfg, ""); err != nil {
			t.Errorf("expected nil for empty gatewayID, got %v", err)
		}
	})

	t.Run("empty credentials namespace returns nil without connecting", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		empty := ExternalDBConfig{CredentialsNamespace: "", ManagedDatabaseID: "md-1"}
		if err := DeleteExternalDatabaseResources(ctx, client, empty, "gw-abc123"); err != nil {
			t.Errorf("expected nil for empty credentials namespace, got %v", err)
		}
	})

	t.Run("missing secret returns error", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		if err := DeleteExternalDatabaseResources(ctx, client, cfg, "gw-abc123"); err == nil {
			t.Error("expected error when the credentials Secret is missing, got nil")
		}
	})
}

// --- single-shot cleanup semantics ---

// Cleanup is unconditional and single-shot: there is no tombstone and no retry
// queue, so the reconciler must swallow the failure (after logging it) rather
// than returning an error that would strand gateway finalization on a retry
// that can never be re-delivered.
func TestExternalDatabaseReconcilerDeleteNeverReturnsError(t *testing.T) {
	ctx := context.Background()
	client := k8sfake.NewSimpleClientset() // no credentials Secret -> cleanup fails
	r := &externalDatabaseReconciler{cfg: ExternalDBConfig{
		CredentialsNamespace: "hypershell-managed-db-kind",
		ManagedDatabaseID:    "md-1",
	}}

	if err := r.Delete(ctx, nil, client, "gw-abc123"); err != nil {
		t.Fatalf("Delete() = %v, want nil (cleanup is best-effort and single-shot)", err)
	}
}

func TestExternalDatabaseReconcilerDeleteNoopsWithoutIdentifiers(t *testing.T) {
	ctx := context.Background()
	client := k8sfake.NewSimpleClientset()

	r := &externalDatabaseReconciler{cfg: ExternalDBConfig{CredentialsNamespace: "hypershell-managed-db-kind"}}
	if err := r.Delete(ctx, nil, client, ""); err != nil {
		t.Errorf("Delete() with empty gatewayID = %v, want nil", err)
	}

	r = &externalDatabaseReconciler{cfg: ExternalDBConfig{}}
	if err := r.Delete(ctx, nil, client, "gw-abc123"); err != nil {
		t.Errorf("Delete() with empty config = %v, want nil", err)
	}
}

// --- externalGatewayDBName ---

func TestExternalGatewayDBName(t *testing.T) {
	if got := externalGatewayDBName("2J5K7M9PqrsTvwxyz"); got != "gw_2j5k7m9pqrstvwxyz" {
		t.Errorf("externalGatewayDBName() = %q, want lowercased gw_ name", got)
	}
}
