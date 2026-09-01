package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// BrowserPKCE performs an OAuth2 authorization code flow with PKCE.
// It starts a local callback server, opens a browser to the authorization URL,
// waits for the redirect, and exchanges the code for tokens.
func BrowserPKCE(issuerURL, clientID string, insecure bool) (TokenResponse, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return TokenResponse{}, err
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return TokenResponse{}, fmt.Errorf("generating state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return TokenResponse{}, fmt.Errorf("starting callback listener: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch in callback")
			return
		}
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			desc := r.URL.Query().Get("error_description")
			fmt.Fprintf(w, "<html><body><h2>Login failed</h2><p>%s: %s</p><p>You may close this window.</p></body></html>",
				html.EscapeString(errParam), html.EscapeString(desc))
			errCh <- fmt.Errorf("authorization error: %s - %s", errParam, desc)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- fmt.Errorf("missing code in callback")
			return
		}
		fmt.Fprintf(w, "<html><body><h2>Login successful</h2><p>You may close this window and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go srv.Serve(ln) //nolint:errcheck

	authURL := buildAuthURL(issuerURL, clientID, callbackURL, state, challenge)
	fmt.Fprintf(os.Stderr, "Opening browser for authentication...\n")
	fmt.Fprintf(os.Stderr, "If the browser does not open, visit:\n  %s\n\n", authURL)
	openBrowser(authURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	defer srv.Shutdown(ctx) //nolint:errcheck

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return TokenResponse{}, err
	case <-ctx.Done():
		return TokenResponse{}, fmt.Errorf("login timed out after 5 minutes")
	}

	resp, err := newHTTPClient(insecure).PostForm(tokenEndpoint(issuerURL), url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {callbackURL},
		"code_verifier": {verifier},
	})
	if err != nil {
		return TokenResponse{}, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	return parseTokenResponse(resp)
}

func generatePKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generating code verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func buildAuthURL(issuerURL, clientID, redirectURI, state, codeChallenge string) string {
	base := strings.TrimRight(issuerURL, "/") + "/protocol/openid-connect/auth"
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"scope":                 {"openid email profile"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return base + "?" + params.Encode()
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	_ = cmd.Start()
}
