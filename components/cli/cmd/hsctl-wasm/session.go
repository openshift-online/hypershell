//go:build js && wasm

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
)

// errSessionExpired is the command error when the BFF asks for a new sign-in
// (CLI-WASM-03). The terminal shows it with a sign-in link (WEB-TERM-10).
var errSessionExpired = errors.New("the console session expired: sign in to the console again, then rerun the command")

// sessionMarker stands in for the access token. The generated connection
// builder refuses to build a connection without one, and the browser module has
// none: the console session cookie authenticates its requests. The marker is not
// a credential and is never sent: noToken supplies the token and sessionClient
// removes the Authorization header.
const sessionMarker = "console-session"

// callState is what one hsctlRun call records about its requests.
type callState struct {
	mutated atomic.Bool
}

// sessionStore is the in-memory configuration of the browser module: the page
// origin and no saved login. It never reads or writes a configuration file.
type sessionStore struct {
	origin string
}

func (s sessionStore) Load() (*config.Config, error) {
	if s.origin == "" {
		return nil, errors.New("can't find the console origin: the browser terminal must run in a worker started by the console page")
	}
	return &config.Config{URL: s.origin, AccessToken: sessionMarker}, nil
}

func (sessionStore) Save(*config.Config) error {
	return errors.New("the browser terminal has no saved configuration; it uses the console sign-in")
}

// noToken is the token provider of the browser module: it holds no token.
type noToken struct{}

func (noToken) GetToken() (string, error) { return "", nil }

// sessionClient sends API requests to the page origin with the console session
// cookie and without an Authorization header. It never retries a request.
type sessionClient struct {
	origin string
	client *http.Client
	call   *callState
}

func newSessionClient(origin string, transport http.RoundTripper, call *callState) connection.HTTPClient {
	return &sessionClient{origin: origin, client: &http.Client{Transport: transport}, call: call}
}

func (c *sessionClient) Do(req *http.Request) (*http.Response, error) {
	if !sameOrigin(req.URL, c.origin) {
		return nil, fmt.Errorf("the browser terminal sends requests only to the console origin %s, not to %s", c.origin, req.URL.Redacted())
	}
	if err := checkAPIPath(req.URL); err != nil {
		return nil, err
	}
	req.Header.Del("Authorization")
	// Read by the Go js/wasm fetch transport: send the session cookie, and only
	// to the page origin.
	req.Header.Set("js.fetch:credentials", "same-origin")
	req.Header.Set("js.fetch:mode", "same-origin")

	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		c.call.mutated.Store(true)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	expired, err := reauthRequired(resp)
	if err != nil {
		return nil, err
	}
	if expired {
		return nil, errSessionExpired
	}
	return resp, nil
}

// apiPrefixes are the only paths the browser module may request: the HyperShell
// API behind the console's same-origin proxy (CLI-WASM-03).
var apiPrefixes = []string{"/api/hypershell/v1", "/api/hypershell/ext"}

// checkAPIPath refuses a request outside the API paths. Commands join IDs into
// paths as given, and the browser resolves "." and ".." segments, including
// percent-encoded ones, before it sends a request; an ID such as
// "../../../../auth/logout" would otherwise reach another console endpoint with
// the session cookie.
func checkAPIPath(u *url.URL) error {
	escaped, decoded := u.EscapedPath(), u.Path
	for _, path := range []string{escaped, decoded} {
		if hasDotSegment(path) {
			return fmt.Errorf("the browser terminal refuses the request path %q: an ID or name must not contain "+
				"'.' or '..' path segments", escaped)
		}
	}
	for _, prefix := range apiPrefixes {
		if (escaped == prefix || strings.HasPrefix(escaped, prefix+"/")) &&
			(decoded == prefix || strings.HasPrefix(decoded, prefix+"/")) {
			return nil
		}
	}
	return fmt.Errorf("the browser terminal sends requests only to %s/ and %s/, not to %q",
		apiPrefixes[0], apiPrefixes[1], escaped)
}

// hasDotSegment reports whether a path has a "." or ".." segment, also after
// percent-decoding (repeatedly, for double encoding) and with a backslash read
// as a separator, as browsers do for http and https URLs.
func hasDotSegment(path string) bool {
	for i := 0; i < 4; i++ {
		for _, segment := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
			if segment == "." || segment == ".." {
				return true
			}
		}
		unescaped, err := url.PathUnescape(path)
		if err != nil || unescaped == path {
			return false
		}
		path = unescaped
	}
	return true
}

func sameOrigin(u *url.URL, origin string) bool {
	o, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, o.Scheme) && strings.EqualFold(u.Host, o.Host)
}

// maxReauthBody bounds how much of a 401 body is read to recognize the BFF
// re-authentication answer.
const maxReauthBody = 64 << 10

// reauthRequired reports whether the response is the BFF re-authentication
// answer: HTTP 401 with WWW-Authenticate: Bearer error="invalid_token" or a JSON
// body with "error": "reauth_required". It closes the body when it returns true
// and otherwise leaves the whole response readable.
func reauthRequired(resp *http.Response) (bool, error) {
	if resp.StatusCode != http.StatusUnauthorized {
		return false, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReauthBody))
	if err != nil {
		_ = resp.Body.Close()
		return false, fmt.Errorf("can't read the API response: %w", err)
	}
	var body struct {
		Error string `json:"error"`
	}
	expired := (json.Unmarshal(data, &body) == nil && body.Error == "reauth_required") ||
		strings.Contains(resp.Header.Get("WWW-Authenticate"), `error="invalid_token"`)
	if expired {
		if err := resp.Body.Close(); err != nil {
			return false, fmt.Errorf("can't close the API response: %w", err)
		}
		return true, nil
	}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(data), resp.Body), resp.Body}
	return false, nil
}
