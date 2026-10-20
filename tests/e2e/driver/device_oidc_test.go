package driver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateGatewayDeviceAuthorizationUsesPKCE(t *testing.T) {
	var challenge string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"device_authorization_endpoint": "http://" + r.Host + "/device",
				"token_endpoint":                "http://" + r.Host + "/token",
			})
		case "/device":
			_ = r.ParseForm()
			challenge = r.Form.Get("code_challenge")
			if r.Form.Get("code_challenge_method") != "S256" {
				t.Error("device request did not use PKCE S256")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code": "device", "user_code": "user", "verification_uri": "https://verify.example", "interval": 0,
			})
		case "/token":
			_ = r.ParseForm()
			digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if got := base64.RawURLEncoding.EncodeToString(digest[:]); got != challenge {
				t.Error("token request verifier does not match device request challenge")
			}
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := validateGatewayDeviceAuthorization(context.Background(), server.Client(), server.URL, "gateway-client"); err != nil {
		t.Fatalf("validate device authorization: %v", err)
	}
}
