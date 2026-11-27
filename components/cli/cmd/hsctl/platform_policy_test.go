package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/commands"
	"github.com/openshift-online/hypershell/components/cli/pkg/apierror"
)

type recordedRequest struct {
	method string
	path   string
	body   string
}

// policyServer serves canned responses keyed by "METHOD path" and records every
// request. HYPERSHELL_CONFIG points the CLI at it with a static token.
func policyServer(t *testing.T, responses map[string]func(w http.ResponseWriter)) (*[]recordedRequest, *sync.Mutex) {
	t.Helper()
	var (
		mu       sync.Mutex
		requests []recordedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, recordedRequest{method: r.Method, path: r.URL.Path, body: string(body)})
		mu.Unlock()
		respond, ok := responses[r.Method+" "+r.URL.Path]
		if !ok {
			http.Error(w, `{"code":"HYPERSHELL-404","reason":"not found"}`, http.StatusNotFound)
			return
		}
		respond(w)
	}))
	t.Cleanup(server.Close)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cfg, err := json.Marshal(map[string]string{"url": server.URL, "access_token": "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERSHELL_CONFIG", cfgPath)
	return &requests, &mu
}

func runHsctl(t *testing.T, argv ...string) (string, error) {
	t.Helper()
	commands.ResetFlags(root)
	var out bytes.Buffer
	root.SetArgs(argv)
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(""))
	t.Cleanup(func() {
		root.SetArgs(nil)
		root.SetOut(nil)
		root.SetErr(nil)
		root.SetIn(nil)
	})
	err := root.Execute()
	return out.String(), err
}

const policyWithStatus = `{
  "kind": "PlatformGatewayPolicy",
  "id": "p1",
  "name": "cpu-cap",
  "cedar": "forbid (principal, action, resource) when { context.cpuRequestMillis > 4000 };",
  "enforcement_mode": "audit",
  "effective_mode": "audit",
  "status": {
    "gateways": [
      {"gateway_id": "g1", "condition": "Applied", "would_reject_24h": 7},
      {"gateway_id": "g2", "condition": "Pending", "would_reject_24h": 0}
    ],
    "rollout": {"applied": 1, "pending": 1, "unsupported": 0},
    "findings_last_24h": {"would_reject": 7, "would_add_defaults": 3}
  }
}`

// hsctl get platform-gateway-policy (the spec's hyphenated name is an alias of
// the camelCase command, like the sibling extension commands) calls the
// extension get-by-id path and renders the read-only status: per-gateway
// rollout condition and the 24-hour finding counts. list calls the extension
// collection path.
// Verifies: CLI-EXT-06.01
func TestPlatformGatewayPolicyGetRendersStatus(t *testing.T) {
	requests, mu := policyServer(t, map[string]func(http.ResponseWriter){
		"GET /api/hypershell/ext/platform_gateway_policies/p1": func(w http.ResponseWriter) {
			_, _ = io.WriteString(w, policyWithStatus)
		},
	})

	out, err := runHsctl(t, "get", "platform-gateway-policy", "p1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var got struct {
		Status struct {
			Gateways []struct {
				GatewayID      string `json:"gateway_id"`
				Condition      string `json:"condition"`
				WouldReject24h int    `json:"would_reject_24h"`
			} `json:"gateways"`
			Rollout struct {
				Applied int `json:"applied"`
				Pending int `json:"pending"`
			} `json:"rollout"`
			Findings struct {
				WouldReject      int `json:"would_reject"`
				WouldAddDefaults int `json:"would_add_defaults"`
			} `json:"findings_last_24h"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("get output is not the policy JSON: %v\n%s", err, out)
	}
	if len(got.Status.Gateways) != 2 || got.Status.Gateways[0].Condition != "Applied" || got.Status.Gateways[0].WouldReject24h != 7 {
		t.Errorf("per-gateway rollout status missing from output: %+v", got.Status.Gateways)
	}
	if got.Status.Rollout.Applied != 1 || got.Status.Rollout.Pending != 1 {
		t.Errorf("rollout counts missing from output: %+v", got.Status.Rollout)
	}
	if got.Status.Findings.WouldReject != 7 || got.Status.Findings.WouldAddDefaults != 3 {
		t.Errorf("24-hour finding counts missing from output: %+v", got.Status.Findings)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) != 1 || (*requests)[0].path != "/api/hypershell/ext/platform_gateway_policies/p1" {
		t.Errorf("requests = %+v, want one GET of the extension get-by-id path", *requests)
	}
}

// The policy toggle and a gateway's effective policies are read through the
// same registrar, from the paths the API serves.
// Verifies: CLI-EXT-06.01
func TestPlatformPolicyReadCommandsCallTheirPaths(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		path string
	}{
		{[]string{"list", "platform-gateway-policies", "--output", "json"}, "/api/hypershell/ext/platform_gateway_policies"},
		{[]string{"get", "platform-policy-settings"}, "/api/hypershell/ext/platform_policy_settings"},
		{[]string{"get", "effective-policies", "g1"}, "/api/hypershell/v1/gateways/g1/effective_policies"},
	} {
		t.Run(strings.Join(tc.argv[:2], " "), func(t *testing.T) {
			requests, mu := policyServer(t, map[string]func(http.ResponseWriter){
				"GET " + tc.path: func(w http.ResponseWriter) {
					_, _ = io.WriteString(w, `{"kind":"List","page":1,"size":0,"total":0,"items":[]}`)
				},
			})
			if _, err := runHsctl(t, tc.argv...); err != nil {
				t.Fatalf("%v: %v", tc.argv, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(*requests) == 0 || (*requests)[0].method != http.MethodGet || (*requests)[0].path != tc.path {
				t.Errorf("requests = %+v, want GET %s", *requests, tc.path)
			}
		})
	}
}

// A policy create, update, delete, or toggle change by a caller without
// platform-policy:admin or platform:admin gets the API 403, which the command
// surfaces as a denied operation rather than a retryable error.
// Verifies: CLI-EXT-06.02
func TestPlatformPolicyWritesSurface403AsDenied(t *testing.T) {
	cedarFile := filepath.Join(t.TempDir(), "cpu-cap.cedar")
	if err := os.WriteFile(cedarFile, []byte("forbid (principal, action, resource);"), 0o600); err != nil {
		t.Fatal(err)
	}
	forbidden := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"kind":"Error","code":"HYPERSHELL-403","reason":"platform-policy:admin required"}`)
	}

	for _, tc := range []struct {
		name   string
		argv   []string
		method string
		path   string
	}{
		{"create", []string{"create", "platform-gateway-policy", "--name", "cpu-cap", "--cedar-file", cedarFile, "--gateway-id", "g1"},
			http.MethodPost, "/api/hypershell/ext/platform_gateway_policies"},
		{"update", []string{"update", "platform-gateway-policy", "p1", "--enforcement-mode", "enforce"},
			http.MethodPatch, "/api/hypershell/ext/platform_gateway_policies/p1"},
		{"delete", []string{"delete", "platform-gateway-policy", "p1", "--yes"},
			http.MethodDelete, "/api/hypershell/ext/platform_gateway_policies/p1"},
		{"toggle", []string{"update", "platform-policy-settings", "--mode", "enforce"},
			http.MethodPatch, "/api/hypershell/ext/platform_policy_settings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests, mu := policyServer(t, map[string]func(http.ResponseWriter){
				tc.method + " " + tc.path: forbidden,
			})

			_, err := runHsctl(t, tc.argv...)
			if err == nil {
				t.Fatalf("%v succeeded, want a denied error", tc.argv)
			}
			if !apierror.IsDenied(err) {
				t.Fatalf("error %q (%T) is not a denied error", err, err)
			}
			msg := err.Error()
			for _, want := range []string{"permission denied", "platform-policy:admin", "retrying will not succeed"} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}

			mu.Lock()
			defer mu.Unlock()
			if len(*requests) != 1 || (*requests)[0].method != tc.method || (*requests)[0].path != tc.path {
				t.Errorf("requests = %+v, want exactly one %s %s (no retry)", *requests, tc.method, tc.path)
			}
		})
	}
}

// create and update send the documented body: create always carries name and
// cedar plus the target; update sends only the fields that were set.
func TestPlatformGatewayPolicyWriteBodies(t *testing.T) {
	cedarFile := filepath.Join(t.TempDir(), "cpu-cap.cedar")
	if err := os.WriteFile(cedarFile, []byte("forbid (principal, action, resource);"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok := func(status int) func(http.ResponseWriter) {
		return func(w http.ResponseWriter) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"id":"p1"}`)
		}
	}
	requests, mu := policyServer(t, map[string]func(http.ResponseWriter){
		"POST /api/hypershell/ext/platform_gateway_policies":     ok(http.StatusCreated),
		"PATCH /api/hypershell/ext/platform_gateway_policies/p2": ok(http.StatusOK),
	})

	if _, err := runHsctl(t, "create", "platformGatewayPolicy", "--name", "cpu-cap", "--cedar-file", cedarFile,
		"--user-message", "CPU capped", "--gateway-id", "g1,g2", "--exclude", "g3"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := runHsctl(t, "update", "platformGatewayPolicy", "p2", "--description", "new"); err != nil {
		t.Fatalf("update: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) != 2 {
		t.Fatalf("requests = %+v, want 2", *requests)
	}
	var created map[string]interface{}
	if err := json.Unmarshal([]byte((*requests)[0].body), &created); err != nil {
		t.Fatal(err)
	}
	target, _ := created["target"].(map[string]interface{})
	if created["name"] != "cpu-cap" || created["cedar"] != "forbid (principal, action, resource);" ||
		created["user_message"] != "CPU capped" || target["all"] != false ||
		len(target["gateway_ids"].([]interface{})) != 2 || len(target["exclude"].([]interface{})) != 1 {
		t.Errorf("create body = %s", (*requests)[0].body)
	}
	if got := (*requests)[1].body; got != `{"description":"new"}` {
		t.Errorf("update body = %s, want only the description", got)
	}
}
