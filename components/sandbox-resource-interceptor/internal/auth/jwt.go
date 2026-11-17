// Package auth verifies the gateway's extension bearer tokens.
//
// The gateway attaches a short-lived Ed25519 JWT to every call on the
// interceptor's Unix socket. The token is minted by the gateway's
// ExtensionJwtIssuer (openshell-extension-core jwt.rs): JOSE typ
// "openshell-ext+jwt", claims iss/sub "openshell-gateway:<gateway-id>", aud
// set to the interceptor's expected audience, caller_kind "gateway", and no
// sandbox_id. The sidecar verifies the signature directly against the
// gateway's public key mounted in the same pod; there is no JWKS fetch.
package auth

import (
	"crypto"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const (
	// ExtensionJWTType is the explicit JOSE typ header of every extension
	// bearer token. Requiring it keeps a sandbox bootstrap token signed by
	// the same key from being accepted here.
	ExtensionJWTType = "openshell-ext+jwt"

	// GatewayIssuerPrefix is the issuer and subject prefix of gateway-minted
	// extension tokens ("openshell-gateway:<gateway-id>").
	GatewayIssuerPrefix = "openshell-gateway:"

	// CallerKindGateway is the caller_kind of tokens minted for the gateway
	// process itself (as opposed to a sandbox supervisor).
	CallerKindGateway = "gateway"
)

// Claims is the extension JWT claim set minted by the gateway. The standard
// claims are validated by the parser; the extension-specific ones are checked
// by Verifier.Verify.
type Claims struct {
	jwt.RegisteredClaims
	// CallerKind is "gateway" or "supervisor" (snake_case on the wire).
	CallerKind string `json:"caller_kind"`
	// SandboxID is set for supervisor callers and must be absent for the
	// gateway.
	SandboxID *string `json:"sandbox_id,omitempty"`
}

// Verifier validates gateway extension tokens against one public key.
type Verifier struct {
	audience  string
	publicKey crypto.PublicKey
	leeway    time.Duration
}

// NewVerifier returns a Verifier that accepts only tokens for audience with
// the given Ed25519 public key. Leeway relaxes exp/iat/nbf checks to absorb
// clock skew inside the pod (the upstream session profile allows 30s).
func NewVerifier(audience string, publicKey crypto.PublicKey, leeway time.Duration) (*Verifier, error) {
	if audience == "" {
		return nil, errors.New("audience must not be empty")
	}
	if _, ok := publicKey.(ed25519.PublicKey); !ok {
		return nil, fmt.Errorf("public key must be Ed25519, got %T", publicKey)
	}
	if leeway < 0 {
		return nil, errors.New("leeway must not be negative")
	}
	return &Verifier{audience: audience, publicKey: publicKey, leeway: leeway}, nil
}

// LoadPublicKey reads an Ed25519 public key from a PKIX PEM file. The gateway
// mounts this from the same JWT Secret it signs with
// (gateway_jwt.public_key_path).
func LoadPublicKey(path string) (crypto.PublicKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading public key: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("public key file %s contains no PEM block", path)
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing public key: %w", err)
	}
	if _, ok := key.(ed25519.PublicKey); !ok {
		return nil, fmt.Errorf("public key must be Ed25519, got %T", key)
	}
	return key, nil
}

// Verify parses and validates a bearer token and returns its claims. Every
// failure is an authentication failure; the returned error is safe to return
// to the gateway as an Unauthenticated status.
func (v *Verifier) Verify(token string) (Claims, error) {
	// Claims validation runs manually below (with leeway) instead of in the
	// parser, because jwt/v4 parser validation has no leeway option.
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithoutClaimsValidation())
	parsed, err := parser.ParseWithClaims(token, &Claims{}, func(_ *jwt.Token) (any, error) {
		return v.publicKey, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("invalid token: %w", err)
	}
	if !parsed.Valid {
		return Claims{}, errors.New("invalid token")
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok {
		return Claims{}, errors.New("invalid token claims")
	}
	if typ, _ := parsed.Header["typ"].(string); typ != ExtensionJWTType {
		return Claims{}, fmt.Errorf("invalid token typ %q, want %q", typ, ExtensionJWTType)
	}
	if err := v.verifyTimes(claims); err != nil {
		return Claims{}, err
	}
	if !claims.VerifyAudience(v.audience, true) {
		return Claims{}, fmt.Errorf("token audience does not include %q", v.audience)
	}
	if err := v.verifyGatewayClaims(claims); err != nil {
		return Claims{}, err
	}
	return *claims, nil
}

// verifyTimes checks exp/iat/nbf with the configured leeway. The gateway and
// this sidecar share a pod, so the leeway only absorbs token-refresh races
// and minor scheduling jitter.
func (v *Verifier) verifyTimes(claims *Claims) error {
	now := time.Now()
	if claims.ExpiresAt == nil || claims.ExpiresAt.IsZero() {
		return errors.New("token has no expiry")
	}
	if now.After(claims.ExpiresAt.Add(v.leeway)) {
		return errors.New("token is expired")
	}
	if claims.IssuedAt == nil || claims.IssuedAt.IsZero() {
		return errors.New("token has no issued-at")
	}
	if claims.IssuedAt.After(now.Add(v.leeway)) {
		return errors.New("token issued in the future")
	}
	if claims.NotBefore != nil && now.Add(v.leeway).Before(claims.NotBefore.Time) {
		return errors.New("token is not yet valid")
	}
	return nil
}

// verifyGatewayClaims enforces the gateway-caller shape of extension tokens:
// a gateway identity issuer and subject, gateway caller kind, and no sandbox
// binding.
func (v *Verifier) verifyGatewayClaims(claims *Claims) error {
	if !hasGatewayIdentity(claims.Issuer) {
		return fmt.Errorf("issuer %q does not identify a gateway (%q prefix missing)", claims.Issuer, GatewayIssuerPrefix)
	}
	if claims.Subject != claims.Issuer {
		return fmt.Errorf("subject %q does not match issuer %q", claims.Subject, claims.Issuer)
	}
	if claims.CallerKind != CallerKindGateway {
		return fmt.Errorf("caller_kind %q, want %q", claims.CallerKind, CallerKindGateway)
	}
	if claims.SandboxID != nil {
		return errors.New("gateway extension tokens must not carry a sandbox_id")
	}
	if claims.ID == "" {
		return errors.New("token has no jti")
	}
	return nil
}

// hasGatewayIdentity reports whether issuer is "openshell-gateway:<id>" with a
// non-empty gateway id.
func hasGatewayIdentity(issuer string) bool {
	rest, ok := strings.CutPrefix(issuer, GatewayIssuerPrefix)
	return ok && rest != ""
}
