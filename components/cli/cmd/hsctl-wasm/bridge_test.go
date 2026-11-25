//go:build js && wasm

package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall/js"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

const testOrigin = "https://console.example.com"

func TestMain(m *testing.M) {
	// A dedicated worker has the page location; the Node.js test runner has none.
	js.Global().Set("location", js.ValueOf(map[string]any{"origin": testOrigin}))
	os.Exit(m.Run())
}

// recordedRequest is one request the module sent.
type recordedRequest struct {
	method string
	url    string
	header http.Header
}

// recorder is the http.RoundTripper of the tests. Go disables the fetch
// transport under Node.js, so the tests inject this one where the worker uses
// http.DefaultTransport.
type recorder struct {
	mu       sync.Mutex
	requests []recordedRequest
	respond  func(*http.Request) (*http.Response, error)
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.requests = append(r.requests, recordedRequest{method: req.Method, url: req.URL.String(), header: req.Header.Clone()})
	respond := r.respond
	r.mu.Unlock()
	if req.Body != nil {
		_ = req.Body.Close()
	}
	if respond == nil {
		return jsonResponse(req, http.StatusOK, `{"kind":"GatewayList","page":1,"size":0,"total":0,"items":[]}`), nil
	}
	return respond(req)
}

func (r *recorder) sent() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.requests...)
}

func jsonResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

// install makes a bridge with the recorder the global hsctlRun, as main does.
func install(t *testing.T) *recorder {
	t.Helper()
	rec := &recorder{}
	fn := newBridge(rec).jsFunc()
	js.Global().Set("hsctlRun", fn)
	t.Cleanup(func() {
		js.Global().Delete("hsctlRun")
		fn.Release()
	})
	return rec
}

type runResult struct {
	stdout, stderr string
	exitCode       int
	mutated        bool
}

// call calls the global hsctlRun like the worker script does and waits for the
// promise.
func call(t *testing.T, args ...any) js.Value {
	t.Helper()
	return js.Global().Call("hsctlRun", args...)
}

func await(t *testing.T, promise js.Value) runResult {
	t.Helper()
	done := make(chan js.Value, 1)
	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		done <- args[0]
		return nil
	})
	defer then.Release()
	promise.Call("then", then)
	select {
	case v := <-done:
		for _, key := range []string{"stdout", "stderr"} {
			if v.Get(key).Type() != js.TypeString {
				t.Fatalf("result %s is %s, want a string", key, v.Get(key).Type())
			}
		}
		if v.Get("exitCode").Type() != js.TypeNumber || v.Get("mutated").Type() != js.TypeBoolean {
			t.Fatalf("result exitCode/mutated have types %s/%s, want number/boolean", v.Get("exitCode").Type(), v.Get("mutated").Type())
		}
		return runResult{
			stdout:   v.Get("stdout").String(),
			stderr:   v.Get("stderr").String(),
			exitCode: v.Get("exitCode").Int(),
			mutated:  v.Get("mutated").Bool(),
		}
	case <-time.After(30 * time.Second):
		t.Fatal("hsctlRun did not resolve")
		return runResult{}
	}
}

func run(t *testing.T, argv ...string) runResult {
	t.Helper()
	values := make([]any, len(argv))
	for i, arg := range argv {
		values[i] = arg
	}
	return await(t, call(t, js.ValueOf(values)))
}

// Verifies: CLI-WASM-02, CLI-WASM-06
func TestBridgeRejectsInvalidArgv(t *testing.T) {
	rec := install(t)
	for name, args := range map[string][]any{
		"no argument":       {},
		"string":            {"list gateways"},
		"number":            {42},
		"object":            {map[string]any{"0": "list"}},
		"non-string item":   {[]any{"list", 7}},
		"two arrays":        {[]any{"list"}, []any{"gateways"}},
		"null item":         {[]any{"list", nil}},
		"undefined (empty)": {js.Undefined()},
	} {
		t.Run(name, func(t *testing.T) {
			res := await(t, call(t, args...))
			if res.exitCode != 2 {
				t.Errorf("exit code = %d, want 2", res.exitCode)
			}
			if !strings.Contains(res.stderr, "array of strings") {
				t.Errorf("stderr = %q, want a usage message", res.stderr)
			}
			if res.stdout != "" || res.mutated {
				t.Errorf("stdout = %q, mutated = %t, want empty and false", res.stdout, res.mutated)
			}
		})
	}
	if sent := rec.sent(); len(sent) != 0 {
		t.Errorf("sent %d requests for invalid input, want none", len(sent))
	}
	if js.Global().Get("hsctlRun").Type() != js.TypeFunction {
		t.Error("hsctlRun is not a global function")
	}
}

// Verifies: CLI-WASM-02.01, CLI-WASM-06
func TestFlagsDoNotLeakBetweenCalls(t *testing.T) {
	rec := install(t)
	if res := run(t, "list", "gateways", "--search", "name like 'prod%'"); res.exitCode != 0 {
		t.Fatalf("first call failed: %+v", res)
	}
	if res := run(t, "list", "gateways"); res.exitCode != 0 {
		t.Fatalf("second call failed: %+v", res)
	}
	sent := rec.sent()
	if len(sent) != 2 {
		t.Fatalf("sent %d requests, want 2", len(sent))
	}
	if !strings.Contains(sent[0].url, "search=") {
		t.Errorf("first request %s carries no search filter", sent[0].url)
	}
	if strings.Contains(sent[1].url, "search") {
		t.Errorf("second request %s carries the search filter of the first call", sent[1].url)
	}
}

// Verifies: CLI-WASM-02, CLI-WASM-06
func TestCallWhileBusyResolvesAtOnce(t *testing.T) {
	rec := install(t)
	release := make(chan struct{})
	rec.respond = func(req *http.Request) (*http.Response, error) {
		<-release
		return jsonResponse(req, http.StatusOK, `{"kind":"GatewayList","page":1,"size":0,"total":0,"items":[]}`), nil
	}

	first := call(t, js.ValueOf([]any{"list", "gateways"}))
	busy := await(t, call(t, js.ValueOf([]any{"delete", "gateway", "3f2a", "--yes"})))
	if busy.exitCode != 1 || !strings.Contains(busy.stderr, "still running") {
		t.Errorf("busy call = %+v, want exit code 1 and a busy message", busy)
	}
	if busy.mutated {
		t.Error("busy call reports mutated")
	}
	close(release)
	if res := await(t, first); res.exitCode != 0 {
		t.Fatalf("first call = %+v, want success", res)
	}
	sent := rec.sent()
	if len(sent) != 1 || sent[0].method != http.MethodGet {
		t.Errorf("sent %+v, want only the GET of the first call", sent)
	}

	// The next call after the first finished runs again.
	rec.respond = nil
	if res := run(t, "list", "gateways"); res.exitCode != 0 {
		t.Errorf("call after the busy one = %+v, want success", res)
	}
}

// Verifies: CLI-WASM-02, CLI-WASM-06
func TestMutatedReportsStateChangingRequests(t *testing.T) {
	rec := install(t)
	res := run(t, "list", "gateways")
	if res.exitCode != 0 || res.mutated {
		t.Errorf("list = %+v, want success with mutated false", res)
	}

	rec.respond = func(req *http.Request) (*http.Response, error) {
		return jsonResponse(req, http.StatusNoContent, ""), nil
	}
	res = run(t, "delete", "gateway", "3f2a", "--yes")
	if res.exitCode != 0 || !res.mutated {
		t.Errorf("delete --yes = %+v, want success with mutated true", res)
	}
	if res.stdout != "Gateway 3f2a deleted.\n" {
		t.Errorf("stdout = %q, want the native output", res.stdout)
	}
	sent := rec.sent()
	if last := sent[len(sent)-1]; last.method != http.MethodDelete || last.url != testOrigin+"/api/hypershell/v1/gateways/3f2a" {
		t.Errorf("last request = %s %s, want DELETE of the gateway", last.method, last.url)
	}

	// A failed state-changing request was still sent.
	rec.respond = func(req *http.Request) (*http.Response, error) {
		return jsonResponse(req, http.StatusConflict, `{"reason":"busy"}`), nil
	}
	res = run(t, "delete", "gateway", "3f2a", "--yes")
	if res.exitCode != 1 || !res.mutated {
		t.Errorf("failed delete = %+v, want exit code 1 with mutated true", res)
	}
}

// Verifies: CLI-WASM-02.02, CLI-WASM-02
func TestConfirmationNeedsYes(t *testing.T) {
	rec := install(t)
	res := run(t, "delete", "gateway", "3f2a")
	if res.exitCode == 0 {
		t.Errorf("exit code = 0, want non-zero")
	}
	if want := "Error: confirmation required but stdin is not a terminal, pass --yes to proceed\n"; res.stderr != want {
		t.Errorf("stderr = %q, want the native message %q", res.stderr, want)
	}
	if res.mutated {
		t.Error("mutated = true, want false")
	}
	if sent := rec.sent(); len(sent) != 0 {
		t.Errorf("sent %+v, want no request", sent)
	}

	// Commands that prompt on standard input read end of file and cancel, as a
	// native hsctl does with empty standard input.
	res = run(t, "delete", "agentRuntime", "2abc")
	if res.exitCode != 0 || !strings.Contains(res.stderr, "Cancelled.") {
		t.Errorf("prompting delete = %+v, want the native cancellation", res)
	}
	if sent := rec.sent(); len(sent) != 0 {
		t.Errorf("sent %+v, want no request", sent)
	}
}

// Verifies: CLI-WASM-03.01, CLI-WASM-03, CLI-WASM-06
func TestRequestsUseTheConsoleSession(t *testing.T) {
	// A saved native login must be neither read nor used, and no file written.
	dir := t.TempDir()
	saved := filepath.Join(dir, "config.json")
	const savedConfig = `{"access_token":"file-token","url":"https://api.example.com"}`
	if err := os.WriteFile(saved, []byte(savedConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERSHELL_CONFIG", saved)
	t.Setenv("HOME", dir)

	rec := install(t)
	if res := run(t, "list", "gateways"); res.exitCode != 0 {
		t.Fatalf("list gateways = %+v, want success", res)
	}
	if res := run(t, "list", "agentRuntimes"); res.exitCode != 0 {
		t.Fatalf("list agentRuntimes = %+v, want success", res)
	}
	sent := rec.sent()
	if len(sent) != 2 {
		t.Fatalf("sent %d requests, want 2", len(sent))
	}
	for i, prefix := range []string{
		testOrigin + "/api/hypershell/v1/gateways?",
		testOrigin + "/api/hypershell/ext/agent_runtimes?",
	} {
		req := sent[i]
		if !strings.HasPrefix(req.url, prefix) {
			t.Errorf("request %d = %s, want %s...", i, req.url, prefix)
		}
		if v, ok := req.header["Authorization"]; ok {
			t.Errorf("request %d carries Authorization %q", i, v)
		}
		if got := req.header.Get("js.fetch:credentials"); got != "same-origin" {
			t.Errorf("request %d js.fetch:credentials = %q, want same-origin", i, got)
		}
		if got := req.header.Get("js.fetch:mode"); got != "same-origin" {
			t.Errorf("request %d js.fetch:mode = %q, want same-origin", i, got)
		}
	}

	data, err := os.ReadFile(saved)
	if err != nil || string(data) != savedConfig {
		t.Errorf("config file changed: %q, %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Errorf("config directory has %d entries (%v), want only the saved file", len(entries), err)
	}
}

// Verifies: CLI-WASM-03
func TestExpiredSessionAsksToSignInAgain(t *testing.T) {
	rec := install(t)
	rec.respond = func(req *http.Request) (*http.Response, error) {
		resp := jsonResponse(req, http.StatusUnauthorized,
			`{"error":"reauth_required","login_url":"/auth/login","message":"Sign in again"}`)
		resp.Header.Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		return resp, nil
	}
	res := run(t, "list", "gateways")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "console session expired") || !strings.Contains(res.stderr, "sign in to the console again") {
		t.Errorf("stderr = %q, want the expired-session message", res.stderr)
	}

	// Another 401 keeps the native error text.
	rec.respond = func(req *http.Request) (*http.Response, error) {
		return jsonResponse(req, http.StatusUnauthorized, `{"reason":"denied"}`), nil
	}
	res = run(t, "list", "gateways")
	if res.exitCode != 1 || !strings.Contains(res.stderr, `API returned 401: {"reason":"denied"}`) {
		t.Errorf("plain 401 = %+v, want the native API error", res)
	}
}

// Verifies: CLI-WASM-03
func TestStateChangingRequestsAreNotRetried(t *testing.T) {
	rec := install(t)
	rec.respond = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	}
	res := run(t, "delete", "gateway", "3f2a", "--yes")
	if res.exitCode != 1 || !strings.Contains(res.stderr, "network down") {
		t.Errorf("delete = %+v, want the transport error", res)
	}
	if sent := rec.sent(); len(sent) != 1 {
		t.Errorf("sent %d requests, want exactly 1", len(sent))
	}
}

// Verifies: CLI-WASM-04.01, CLI-WASM-06
func TestLoginExplainsTheBrowserSession(t *testing.T) {
	rec := install(t)
	res := run(t, "login")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
	for _, want := range []string{"browser terminal uses the console sign-in", "hsctl login needs the installed hsctl", "/docs/cli/install-hsctl"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", res.stderr, want)
		}
	}
	if sent := rec.sent(); len(sent) != 0 {
		t.Errorf("sent %+v, want no request", sent)
	}
}

// Verifies: CLI-WASM-04, CLI-WASM-06
func TestLocalOnlyCommandsAndFlags(t *testing.T) {
	rec := install(t)
	for _, tc := range []struct {
		argv   []string
		signIn bool
	}{
		{[]string{"logout"}, true},
		{[]string{"whoami"}, true},
		{[]string{"login", "--url", "https://api.example.com", "--token-file", "/dev/stdin"}, true},
		{[]string{"config"}, false},
		{[]string{"completion", "bash"}, false},
		{[]string{"completion"}, false},
		{[]string{"tui"}, false},
		{[]string{"apply", "-f", "gateway.yaml"}, false},
		{[]string{"apply", "-k", "overlays/dev"}, false},
		{[]string{"secret-source", "upload", "key", "--value-file", "key.pem"}, false},
		{[]string{"secret-source", "upload", "key", "--value-stdin"}, false},
		{[]string{"create", "gateway", "--body", "gateway.json"}, false},
		{[]string{"update", "gateway", "3f2a", "--body", "gateway.json"}, false},
		{[]string{"create", "platformGatewayPolicy", "--name", "p", "--cedar-file", "-"}, false},
		{[]string{"create", "serviceAccount", "--gateway-id", "g1", "--name", "ci", "--output-file", "bundle.json"}, false},
	} {
		t.Run(strings.Join(tc.argv, " "), func(t *testing.T) {
			res := run(t, tc.argv...)
			if res.exitCode != 1 {
				t.Errorf("exit code = %d, want 1 (stderr %q)", res.exitCode, res.stderr)
			}
			if !strings.Contains(res.stderr, "needs the installed hsctl") || !strings.Contains(res.stderr, "/docs/cli/install-hsctl") {
				t.Errorf("stderr = %q, want the installed-hsctl message", res.stderr)
			}
			if got := strings.Contains(res.stderr, "console sign-in"); got != tc.signIn {
				t.Errorf("stderr = %q, mentions the console sign-in: %t, want %t", res.stderr, got, tc.signIn)
			}
			if res.mutated {
				t.Error("mutated = true, want false")
			}
		})
	}
	if sent := rec.sent(); len(sent) != 0 {
		t.Errorf("local-only commands sent %+v, want no request", sent)
	}
}

// Verifies: CLI-WASM-04, CLI-WASM-02
func TestHelpVersionAndErrorsMatchNative(t *testing.T) {
	rec := install(t)
	res := run(t, "help")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "Available Commands:") || !strings.Contains(res.stdout, "tui") {
		t.Errorf("help = %+v, want the command list", res)
	}
	res = run(t)
	if res.exitCode != 0 || !strings.Contains(res.stdout, "Command line tool for the HyperShell API server.") {
		t.Errorf("no arguments = %+v, want the root help", res)
	}
	res = run(t, "login", "--help")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "--token-file") {
		t.Errorf("login --help = %+v, want the native help", res)
	}
	res = run(t, "version")
	if res.exitCode != 0 || strings.TrimSpace(res.stdout) == "" {
		t.Errorf("version = %+v, want the version", res)
	}
	res = run(t, "version", "extra")
	if want := "Error: unknown command \"extra\" for \"hsctl version\"\n"; res.exitCode != 1 || res.stderr != want {
		t.Errorf("version extra = %+v, want exit code 1 and %q", res, want)
	}
	res = run(t, "get", "gateway")
	if want := "Error: accepts 1 arg(s), received 0\n"; res.exitCode != 1 || res.stderr != want {
		t.Errorf("get gateway = %+v, want exit code 1 and %q", res, want)
	}
	if sent := rec.sent(); len(sent) != 0 {
		t.Errorf("sent %+v, want no request", sent)
	}

	rec.respond = func(req *http.Request) (*http.Response, error) {
		return jsonResponse(req, http.StatusNotFound, `{"reason":"not found"}`), nil
	}
	res = run(t, "get", "gateway", "3f2a")
	if res.exitCode != 1 || !strings.HasPrefix(res.stderr, "Error: ") || !strings.Contains(res.stderr, "404") {
		t.Errorf("get missing gateway = %+v, want the native API error", res)
	}
}

// Verifies: CLI-WASM-01.02, CLI-WASM-06.01
func TestBrowserTreeHasTUIOnlyAsLocalOnlyStub(t *testing.T) {
	install(t)
	tui, _, err := sharedRoot().Find([]string{"tui"})
	if err != nil || tui.Name() != "tui" {
		t.Fatalf("tui is not in the browser tree: %v", err)
	}
	tui.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "help" {
			t.Errorf("the browser tui has the flag --%s; it must be the local-only stub, not the native command", f.Name)
		}
	})
}

// IDs are joined into request paths as given, and the browser resolves dot
// segments before it sends a request, so a traversal ID must never leave the
// API paths with the session cookie.
// Verifies: CLI-WASM-03
func TestTraversalIDsSendNoRequest(t *testing.T) {
	rec := install(t)
	for _, argv := range [][]string{
		{"get", "gateway", "../../../../auth/logout"},
		{"get", "gateway", ".."},
		{"get", "gateway", "."},
		{"get", "gateway", "%2e%2e/%2e%2e/%2e%2e/%2e%2e/auth/logout"},
		{"get", "gateway", "%2E%2E%2F%2E%2E%2Ftelemetry"},
		{"get", "gateway", ".%2e/.%2e/telemetry"},
		{"get", "gateway", "%252e%252e/%252e%252e/auth/logout"},
		{"get", "gateway", `..\..\..\..\auth\logout`},
		{"get", "agentRuntime", "../../../telemetry"},
		{"delete", "gateway", "../../../../auth/logout", "--yes"},
	} {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			res := run(t, argv...)
			if res.exitCode != 1 {
				t.Errorf("exit code = %d, want 1 (stderr %q)", res.exitCode, res.stderr)
			}
			if !strings.Contains(res.stderr, "browser terminal refuses the request path") {
				t.Errorf("stderr = %q, want the refused-path message", res.stderr)
			}
			if res.mutated {
				t.Error("mutated = true for a refused request")
			}
		})
	}
	if sent := rec.sent(); len(sent) != 0 {
		t.Errorf("traversal IDs sent %+v, want no request", sent)
	}
}

// Verifies: CLI-WASM-03
func TestSessionClientRefusesPathsOutsideTheAPI(t *testing.T) {
	rec := &recorder{}
	client := newSessionClient(testOrigin, rec, &callState{})
	for _, path := range []string{
		"/auth/logout",
		"/telemetry",
		"/",
		"/api/hypershell/v1beta/gateways",
		"/api/hypershell/extensions",
		"/api/hypershell",
	} {
		req, err := http.NewRequest(http.MethodGet, testOrigin+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Do(req); err == nil || !strings.Contains(err.Error(), "only to /api/hypershell/v1/ and /api/hypershell/ext/") {
			t.Errorf("%s: err = %v, want the API-path refusal", path, err)
		}
	}
	for _, path := range []string{"/api/hypershell/v1", "/api/hypershell/v1/gateways", "/api/hypershell/ext/agent_runtimes/a1"} {
		req, err := http.NewRequest(http.MethodGet, testOrigin+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("%s: err = %v, want the request sent", path, err)
			continue
		}
		_ = resp.Body.Close()
	}
	if sent := rec.sent(); len(sent) != 3 {
		t.Errorf("sent %d requests, want only the 3 API requests", len(sent))
	}
}
