package policyinterceptor

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Gateway extension token contract (OpenShell extensibility overview,
// "Authenticating Extensions"): typ openshell-ext+jwt, alg EdDSA pinned,
// iss openshell-gateway:<gateway_id>, exact aud, caller_kind gateway.
const (
	extensionJWTTyp   = "openshell-ext+jwt"
	callerKindGateway = "gateway"
	// clockLeeway tolerates small skew; the gateway and the sidecar share a pod
	// and therefore a node clock.
	clockLeeway = 30 * time.Second
)

// extensionClaims is the OpenShell extension claim set.
type extensionClaims struct {
	Issuer     string           `json:"iss"`
	Audience   jwt.ClaimStrings `json:"aud"`
	Subject    string           `json:"sub"`
	IssuedAt   *jwt.NumericDate `json:"iat,omitempty"`
	ExpiresAt  *jwt.NumericDate `json:"exp,omitempty"`
	NotBefore  *jwt.NumericDate `json:"nbf,omitempty"`
	ID         string           `json:"jti"`
	CallerKind string           `json:"caller_kind"`
	SandboxID  string           `json:"sandbox_id,omitempty"`
}

// Valid is a no-op so golang-jwt does not apply its own (leeway-free) time
// checks; Verifier.Verify validates every claim explicitly.
func (extensionClaims) Valid() error { return nil }

// Verifier verifies the gateway's extension JWT on every interceptor call
// (PLAT-PGP-09). The public key file is re-read when it changes, so a rotated
// gateway signing key is picked up without a restart.
type Verifier struct {
	keyFile  string
	issuer   string
	audience string
	now      func() time.Time

	mu      sync.Mutex
	key     ed25519.PublicKey
	modTime time.Time
	size    int64
}

// NewVerifier loads the gateway public key and returns a verifier. A missing or
// invalid key is a startup error.
func NewVerifier(keyFile, issuer, audience string) (*Verifier, error) {
	if issuer == "" || audience == "" {
		return nil, errors.New("JWT issuer and audience are required")
	}
	v := &Verifier{keyFile: keyFile, issuer: issuer, audience: audience, now: time.Now}
	if _, err := v.publicKey(); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *Verifier) publicKey() (ed25519.PublicKey, error) {
	info, err := os.Stat(v.keyFile)
	if err != nil {
		return nil, fmt.Errorf("stat gateway JWT public key: %w", err)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key != nil && info.ModTime().Equal(v.modTime) && info.Size() == v.size {
		return v.key, nil
	}
	raw, err := os.ReadFile(v.keyFile)
	if err != nil {
		return nil, fmt.Errorf("read gateway JWT public key: %w", err)
	}
	parsed, err := jwt.ParseEdPublicKeyFromPEM(raw)
	if err != nil {
		return nil, fmt.Errorf("parse gateway JWT public key: %w", err)
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("gateway JWT public key is not an Ed25519 key")
	}
	v.key, v.modTime, v.size = key, info.ModTime(), info.Size()
	return key, nil
}

// Verify checks one bearer token and returns its claims.
func (v *Verifier) Verify(token string) (*extensionClaims, error) {
	key, err := v.publicKey()
	if err != nil {
		return nil, err
	}
	claims := &extensionClaims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	parsed, err := parser.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if typ, _ := t.Header["typ"].(string); typ != extensionJWTTyp {
			return nil, fmt.Errorf("token typ %q is not %q", typ, extensionJWTTyp)
		}
		return key, nil
	})
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	if !parsed.Valid {
		return nil, errors.New("token is not valid")
	}
	now := v.now()
	if claims.ExpiresAt == nil {
		return nil, errors.New("token has no exp claim")
	}
	if now.After(claims.ExpiresAt.Add(clockLeeway)) {
		return nil, errors.New("token is expired")
	}
	if claims.NotBefore != nil && now.Add(clockLeeway).Before(claims.NotBefore.Time) {
		return nil, errors.New("token is not yet valid")
	}
	if claims.IssuedAt != nil && now.Add(clockLeeway).Before(claims.IssuedAt.Time) {
		return nil, errors.New("token is issued in the future")
	}
	if claims.Issuer != v.issuer {
		return nil, fmt.Errorf("token issuer %q is not the expected gateway issuer", claims.Issuer)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != v.audience {
		return nil, errors.New("token audience does not exactly match the interceptor audience")
	}
	if claims.CallerKind != callerKindGateway {
		return nil, fmt.Errorf("token caller_kind %q is not %q", claims.CallerKind, callerKindGateway)
	}
	return claims, nil
}

// UnaryServerInterceptor rejects every call whose bearer token does not verify
// with codes.Unauthenticated (PLAT-PGP-09.02). The detailed reason goes only to
// onReject (logs and metrics); the returned status is generic, so a probing
// client learns nothing about the expected issuer or audience.
func (v *Verifier) UnaryServerInterceptor(onReject func(method string, err error)) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		token, err := bearerToken(ctx)
		if err == nil {
			_, err = v.Verify(token)
		}
		if err != nil {
			if onReject != nil {
				onReject(info.FullMethod, err)
			}
			return nil, status.Error(codes.Unauthenticated, "gateway extension token did not verify")
		}
		return handler(ctx, req)
	}
}

func bearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no request metadata")
	}
	values := md.Get("authorization")
	if len(values) != 1 {
		return "", errors.New("expected exactly one authorization header")
	}
	scheme, token, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", errors.New("authorization header is not a bearer token")
	}
	return strings.TrimSpace(token), nil
}
