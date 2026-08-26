package serviceaccount

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
)

func TestExpiration(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	got, err := Expiration("", "30d", now)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-09-20T12:00:00Z"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, test := range []struct {
		name      string
		expiresAt string
		expiresIn string
	}{
		{name: "mutually exclusive", expiresAt: now.Format(time.RFC3339), expiresIn: "1h"},
		{name: "zero", expiresIn: "0h"},
		{name: "too many days", expiresIn: "366d"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Expiration(test.expiresAt, test.expiresIn, now); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestWriteStructuredCreatesPrivateFileWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.json")
	body := []byte(`{"credential":{"client_secret":"secret-value"}}`)
	if err := WriteStructured(io.Discard, path, body); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("file mode is %o, want 600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("secret-value")) || !bytes.Contains(data, []byte("workspace_membership_note")) {
		t.Fatalf("credential output is incomplete: %s", data)
	}
	if err := WriteStructured(io.Discard, path, body); err == nil {
		t.Fatal("expected existing output file to be rejected")
	}
}

func TestReserveOutputRejectsExistingTargetWithoutRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"credential":{"client_secret":"secret-value"}}`))
	}))
	defer server.Close()

	conn, err := connection.NewConnection().Config(&config.Config{
		URL:         server.URL,
		AccessToken: "management-token",
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Mirror the command flow: the output target is reserved before any request
	// is made, so an existing --output-file must abort before the POST.
	file, err := ReserveOutput(path)
	if err == nil {
		ReleaseOutput(file)
		if _, _, reqErr := Request(conn, http.MethodPost, "/service_accounts", nil, nil, http.StatusCreated); reqErr != nil {
			t.Fatalf("unexpected request error: %v", reqErr)
		}
		t.Fatal("expected reservation of an existing output file to fail")
	}
	if requests != 0 {
		t.Fatalf("expected zero HTTP requests, got %d", requests)
	}

	// The pre-existing file must be left untouched by the failed reservation.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Fatalf("existing output file was modified: %s", data)
	}
}

func TestReleaseOutputRemovesEmptyReservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.json")
	file, err := ReserveOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	if file == nil {
		t.Fatal("expected a reserved file handle")
	}
	ReleaseOutput(file)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected reservation to be removed, stat err = %v", err)
	}
	// A retry must succeed once the empty reservation is gone.
	retry, err := ReserveOutput(path)
	if err != nil {
		t.Fatalf("retry reservation failed: %v", err)
	}
	if err := WriteReserved(io.Discard, retry, []byte(`{"credential":{"client_secret":"secret-value"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRequestDoesNotExposeUnstructuredResponseBody(t *testing.T) {
	const secret = "must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"client_secret":"` + secret + `"}`))
	}))
	defer server.Close()

	conn, err := connection.NewConnection().Config(&config.Config{
		URL:         server.URL,
		AccessToken: "management-token",
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, _, err = Request(conn, http.MethodGet, "/failure", nil, nil, http.StatusOK)
	if err == nil {
		t.Fatal("expected request to fail")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposed response body: %v", err)
	}
}
