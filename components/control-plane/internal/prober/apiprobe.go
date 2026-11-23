package prober

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// doAPIRead issues one GET against a production instance's read path (OBS-PRB-04,
// OBS-PRB-05) and returns the HTTP status. token is attached as a bearer
// credential when non-empty. A transport failure or timeout returns a non-nil
// error.
func doAPIRead(ctx context.Context, httpClient *http.Client, instanceURL, readPath, token string) (int, error) {
	target := strings.TrimRight(instanceURL, "/") + readPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, fmt.Errorf("build API read request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GET %s: %w", target, err)
	}
	// The body is irrelevant to classification; drain and close so the connection
	// can be reused.
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}

// probeAPIAvailability runs one API availability probe (OBS-PRB-04): one real
// database-backed read against a production instance. A 5xx status, a timeout, or
// a transport failure is a failed probe; any other response, including a 4xx such
// as 401, is a good probe. token is best-effort (an authentication outage must not
// pull down the availability signal, which is measured separately by OBS-PRB-05),
// so it is attached when available and omitted otherwise.
//
// When a token is present the read reaches the database read path, so a
// database-backed outage surfaces as a 5xx failed probe (OBS-PRB-04). When no
// token is available (the token exchange is down or unconfigured) the API answers
// 401 before the database read, so the probe degrades to confirming the API is
// reachable; this is the intended behaviour, since OBS-PRB-04 defines a 4xx as a
// good availability event and an authentication outage is measured separately by
// OBS-PRB-05.
func probeAPIAvailability(ctx context.Context, httpClient *http.Client, instanceURL, readPath, token string) Result {
	status, err := doAPIRead(ctx, httpClient, instanceURL, readPath, token)
	return classifyAvailability(status, err)
}

// classifyAvailability classifies an API availability read (OBS-PRB-04).
func classifyAvailability(status int, err error) Result {
	if err != nil {
		return ResultFailure // transport failure or timeout
	}
	if status >= 500 {
		return ResultFailure // 5xx
	}
	return ResultSuccess // any non-5xx, including 401
}

// probeAPIAuth runs one API authentication probe (OBS-PRB-05): obtain an access
// token from Keycloak with the prober identity, then one authenticated read. The
// probe is good only when both steps succeed and the authenticated read returns a
// 2xx. A token acquisition failure is a failed probe and the authenticated read is
// NOT attempted; a non-2xx read (including 401/403), a transport failure, or a
// timeout is a failed probe.
func probeAPIAuth(ctx context.Context, httpClient *http.Client, instanceURL, readPath string, exchange func(context.Context) (string, error)) Result {
	token, err := exchange(ctx)
	if err != nil || token == "" {
		// Token acquisition failed; do not call the authenticated read (OBS-PRB-05.02).
		return ResultFailure
	}
	status, err := doAPIRead(ctx, httpClient, instanceURL, readPath, token)
	return classifyAuth(status, err)
}

// classifyAuth classifies an authenticated API read (OBS-PRB-05).
func classifyAuth(status int, err error) Result {
	if err != nil {
		return ResultFailure // transport failure or timeout
	}
	if status >= 200 && status < 300 {
		return ResultSuccess
	}
	return ResultFailure // non-2xx, including 401/403
}
