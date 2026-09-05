package gateway

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	pq "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// --- validateExternalSecretName ---

func TestValidateExternalSecretName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "namespace slash", input: "hypershell-managed-db-foo/bar", wantErr: true},
		{name: "missing prefix", input: "my-secret", wantErr: true},
		{name: "valid", input: "hypershell-managed-db-prod", wantErr: false},
		{name: "valid with numbers", input: "hypershell-managed-db-123abc", wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExternalSecretName(tc.input)
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
	ns := "hypershell-system"
	secretName := "hypershell-managed-db-kind"

	t.Run("secret not found", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		_, sentinel, err := readExternalAdminSecret(ctx, client, ns, secretName)
		if err == nil {
			t.Fatal("expected error for missing secret, got nil")
		}
		if sentinel != "secret_invalid" {
			t.Errorf("sentinel = %q, want %q", sentinel, "secret_invalid")
		}
	})

	t.Run("invalid secret name (no prefix)", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		_, sentinel, err := readExternalAdminSecret(ctx, client, ns, "my-secret")
		if err == nil {
			t.Fatal("expected error for invalid secret name, got nil")
		}
		if sentinel != "secret_invalid" {
			t.Errorf("sentinel = %q, want %q", sentinel, "secret_invalid")
		}
	})

	t.Run("secret missing required key", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
			Data: map[string][]byte{
				"host": []byte("db.example.com"),
				"port": []byte("5432"),
				// user and password missing
			},
		}
		client := k8sfake.NewSimpleClientset(secret)
		_, sentinel, err := readExternalAdminSecret(ctx, client, ns, secretName)
		if err == nil {
			t.Fatal("expected error for missing key, got nil")
		}
		if sentinel != "secret_invalid" {
			t.Errorf("sentinel = %q, want %q", sentinel, "secret_invalid")
		}
	})

	t.Run("valid secret defaults dbname and sslmode", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
			Data: map[string][]byte{
				"host":     []byte("db.example.com"),
				"port":     []byte("5432"),
				"user":     []byte("admin"),
				"password": []byte("s3cr3t"),
			},
		}
		client := k8sfake.NewSimpleClientset(secret)
		params, sentinel, err := readExternalAdminSecret(ctx, client, ns, secretName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sentinel != "" {
			t.Errorf("sentinel = %q, want empty", sentinel)
		}
		if params.dbname != "postgres" {
			t.Errorf("dbname = %q, want %q", params.dbname, "postgres")
		}
		if params.sslmode != "require" {
			t.Errorf("sslmode = %q, want %q", params.sslmode, "require")
		}
	})

	t.Run("valid secret with explicit dbname and sslmode", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
			Data: map[string][]byte{
				"host":     []byte("db.example.com"),
				"port":     []byte("5432"),
				"user":     []byte("admin"),
				"password": []byte("s3cr3t"),
				"dbname":   []byte("mydb"),
				"sslmode":  []byte("disable"),
			},
		}
		client := k8sfake.NewSimpleClientset(secret)
		params, _, err := readExternalAdminSecret(ctx, client, ns, secretName)
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
}

// --- dsn uses URL format, includes connect_timeout and optional sslrootcert ---

func TestDSNConnectTimeout(t *testing.T) {
	p := &externalAdminParams{
		host: "db.example.com", port: "5432",
		user: "admin", password: "pass",
		dbname: "postgres", sslmode: "require",
	}
	dsn := p.dsn()
	if !strings.Contains(dsn, "connect_timeout=10") {
		t.Errorf("dsn does not contain connect_timeout=10: %s", dsn)
	}
}

func TestDSNURLFormat(t *testing.T) {
	t.Run("special chars in password are percent-encoded", func(t *testing.T) {
		p := &externalAdminParams{host: "h", port: "5432", user: "u", password: "p@ss w0rd!", dbname: "db", sslmode: "require"}
		dsn := p.dsn()
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
		if strings.Contains(p.dsn(), "sslrootcert") {
			t.Errorf("unexpected sslrootcert in dsn: %s", p.dsn())
		}
	})
	t.Run("sslrootcert file path included when set", func(t *testing.T) {
		p := &externalAdminParams{host: "h", port: "5432", user: "u", password: "p", dbname: "db", sslmode: "verify-full", sslrootcert: "/etc/ssl/ca.pem"}
		if !strings.Contains(p.dsn(), "sslrootcert") {
			t.Errorf("dsn missing sslrootcert: %s", p.dsn())
		}
	})
}

// --- DeleteExternalDatabaseResources early-exit paths ---

func TestDeleteExternalDatabaseResourcesEarlyExit(t *testing.T) {
	ctx := context.Background()

	t.Run("empty gatewayID returns nil without connecting", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		cfg := ExternalDBConfig{SecretName: "hypershell-managed-db-kind", Namespace: "hypershell-system"}
		err := DeleteExternalDatabaseResources(ctx, client, cfg, "")
		if err != nil {
			t.Errorf("expected nil for empty gatewayID, got %v", err)
		}
	})

	t.Run("empty SecretName returns nil without connecting", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		cfg := ExternalDBConfig{SecretName: "", Namespace: "hypershell-system"}
		err := DeleteExternalDatabaseResources(ctx, client, cfg, "gw-abc123")
		if err != nil {
			t.Errorf("expected nil for empty SecretName, got %v", err)
		}
	})

	t.Run("missing secret returns error", func(t *testing.T) {
		client := k8sfake.NewSimpleClientset()
		cfg := ExternalDBConfig{SecretName: "hypershell-managed-db-kind", Namespace: "hypershell-system"}
		err := DeleteExternalDatabaseResources(ctx, client, cfg, "gw-abc123")
		if err == nil {
			t.Error("expected error when secret is missing, got nil")
		}
	})
}
