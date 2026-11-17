package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const testAudience = "urn:openshell:extension:interceptor:sandbox-resources"

// testKeys holds an Ed25519 keypair standing in for the gateway's JWT Secret.
type testKeys struct {
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func newTestKeys(t *testing.T) testKeys {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating Ed25519 keypair: %v", err)
	}
	return testKeys{public: public, private: private}
}

func (k testKeys) publicPEM(t *testing.T) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(k.public)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

// mintToken encodes an extension JWT the way the gateway's ExtensionJwtIssuer
// does: EdDSA, typ "openshell-ext+jwt", snake_case caller_kind.
func (k testKeys) mintToken(t *testing.T, mutate func(header *jwt.Token, claims *Claims)) string {
	t.Helper()
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "openshell-gateway:abc123",
			Subject:   "openshell-gateway:abc123",
			Audience:  jwt.ClaimStrings{testAudience},
			ID:        "0e5f3a5a-6d0a-4a1a-9d8a-2f8d0f1f4e2a",
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
		CallerKind: CallerKindGateway,
	}
	header := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	header.Header["typ"] = ExtensionJWTType
	header.Header["kid"] = "gateway-jwt"
	if mutate != nil {
		mutate(header, claims)
	}
	token, err := header.SignedString(k.private)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return token
}

func newTestVerifier(t *testing.T) (*Verifier, testKeys) {
	t.Helper()
	keys := newTestKeys(t)
	verifier, err := NewVerifier(testAudience, keys.public, 30*time.Second)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return verifier, keys
}

func TestVerifyAcceptsGatewayToken(t *testing.T) {
	verifier, keys := newTestVerifier(t)
	claims, err := verifier.Verify(keys.mintToken(t, nil))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Issuer != "openshell-gateway:abc123" {
		t.Errorf("issuer = %q", claims.Issuer)
	}
	if claims.CallerKind != CallerKindGateway {
		t.Errorf("caller_kind = %q", claims.CallerKind)
	}
}

func TestVerifyRejections(t *testing.T) {
	verifier, keys := newTestVerifier(t)
	otherKeys := newTestKeys(t)
	expired := time.Now().Add(-2 * time.Hour)

	cases := []struct {
		name  string
		token string
	}{
		{
			name:  "wrong signature",
			token: otherKeys.mintToken(t, nil),
		},
		{
			name: "wrong audience",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				claims.Audience = jwt.ClaimStrings{"urn:openshell:extension:interceptor:other"}
			}),
		},
		{
			name: "sandbox bootstrap typ",
			token: keys.mintToken(t, func(header *jwt.Token, _ *Claims) {
				header.Header["typ"] = "openshell-sandbox-session+jwt"
			}),
		},
		{
			name: "missing typ",
			token: keys.mintToken(t, func(header *jwt.Token, _ *Claims) {
				delete(header.Header, "typ")
			}),
		},
		{
			name: "expired",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				claims.ExpiresAt = jwt.NewNumericDate(expired)
			}),
		},
		{
			name: "no expiry",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				claims.ExpiresAt = nil
			}),
		},
		{
			name: "supervisor caller kind",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				claims.CallerKind = "supervisor"
				sandboxID := "sandbox-1"
				claims.SandboxID = &sandboxID
			}),
		},
		{
			name: "sandbox id on gateway token",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				sandboxID := "sandbox-1"
				claims.SandboxID = &sandboxID
			}),
		},
		{
			name: "issuer without gateway prefix",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				claims.Issuer = "someone-else"
				claims.Subject = "someone-else"
			}),
		},
		{
			name: "empty gateway id",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				claims.Issuer = GatewayIssuerPrefix
				claims.Subject = GatewayIssuerPrefix
			}),
		},
		{
			name: "subject differs from issuer",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				claims.Subject = "openshell-gateway:other"
			}),
		},
		{
			name: "no jti",
			token: keys.mintToken(t, func(_ *jwt.Token, claims *Claims) {
				claims.ID = ""
			}),
		},
		{
			name:  "not a token",
			token: "garbage",
		},
		{
			name:  "empty",
			token: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifier.Verify(tc.token); err == nil {
				t.Fatalf("Verify accepted a token that must be rejected (%s)", tc.name)
			}
		})
	}
}

// The gateway serializes aud as a plain JSON string, not an array; both forms
// must parse.
func TestVerifyAcceptsStringAudience(t *testing.T) {
	verifier, keys := newTestVerifier(t)
	token := keys.mintToken(t, func(header *jwt.Token, claims *Claims) {
		header.Claims = stringAudienceClaims(t, claims)
	})
	if _, err := verifier.Verify(token); err != nil {
		t.Fatalf("Verify rejected a string-audience token: %v", err)
	}
}

// stringAudienceClaims re-encodes the claims with aud as a bare string by
// round-tripping through a map, matching the Rust issuer's serialization.
func stringAudienceClaims(t *testing.T, claims *Claims) jwt.MapClaims {
	t.Helper()
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshaling claims: %v", err)
	}
	var out jwt.MapClaims
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshaling claims: %v", err)
	}
	out["aud"] = testAudience
	return out
}

func TestNewVerifierRejectsBadInputs(t *testing.T) {
	if _, err := NewVerifier("", ed25519.PublicKey{}, 0); err == nil {
		t.Error("empty audience accepted")
	}
	if _, err := NewVerifier("aud", "not a key", 0); err == nil {
		t.Error("non-Ed25519 key accepted")
	}
	if _, err := NewVerifier("aud", ed25519.PublicKey{}, -time.Second); err == nil {
		t.Error("negative leeway accepted")
	}
}

func TestLoadPublicKey(t *testing.T) {
	keys := newTestKeys(t)

	t.Run("valid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "public.pem")
		if err := os.WriteFile(path, keys.publicPEM(t), 0o644); err != nil {
			t.Fatalf("writing key: %v", err)
		}
		loaded, err := LoadPublicKey(path)
		if err != nil {
			t.Fatalf("LoadPublicKey: %v", err)
		}
		if !loaded.(ed25519.PublicKey).Equal(keys.public) {
			t.Error("loaded key differs from the written key")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, err := LoadPublicKey(filepath.Join(t.TempDir(), "absent.pem")); err == nil {
			t.Error("missing file accepted")
		}
	})

	t.Run("not PEM", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "public.pem")
		if err := os.WriteFile(path, []byte("not pem"), 0o644); err != nil {
			t.Fatalf("writing key: %v", err)
		}
		if _, err := LoadPublicKey(path); err == nil {
			t.Error("non-PEM file accepted")
		}
	})

	t.Run("not Ed25519", func(t *testing.T) {
		// An RSA key parses as PKIX but must be rejected.
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generating RSA key: %v", err)
		}
		der, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
		if err != nil {
			t.Fatalf("marshaling RSA key: %v", err)
		}
		rsaPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
		path := filepath.Join(t.TempDir(), "public.pem")
		if err := os.WriteFile(path, rsaPEM, 0o644); err != nil {
			t.Fatalf("writing key: %v", err)
		}
		if _, err := LoadPublicKey(path); err == nil {
			t.Error("RSA key accepted")
		}
	})
}

func TestHasGatewayIdentity(t *testing.T) {
	cases := map[string]bool{
		"openshell-gateway:abc":    true,
		"openshell-gateway:":       false,
		"openshell-gateway":        false,
		"openshell-supervisor:abc": false,
		"":                         false,
	}
	for issuer, want := range cases {
		if got := hasGatewayIdentity(issuer); got != want {
			t.Errorf("hasGatewayIdentity(%q) = %v, want %v", issuer, got, want)
		}
	}
}
