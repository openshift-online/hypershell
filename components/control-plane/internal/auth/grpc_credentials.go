package auth

import (
	"context"
	"fmt"
)

// GRPCCredentials implements grpc/credentials.PerRPCCredentials by attaching
// a Bearer token from a TokenProvider to every outgoing gRPC call.
type GRPCCredentials struct {
	provider *TokenProvider
}

// NewGRPCCredentials returns a PerRPCCredentials implementation backed by the
// given TokenProvider.
func NewGRPCCredentials(provider *TokenProvider) *GRPCCredentials {
	return &GRPCCredentials{provider: provider}
}

// GetRequestMetadata returns authorization metadata containing a Bearer token.
func (c *GRPCCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	token, err := c.provider.Token()
	if err != nil {
		return nil, fmt.Errorf("get OIDC token: %w", err)
	}

	if token == "" {
		return nil, nil
	}

	return map[string]string{
		"authorization": "Bearer " + token,
	}, nil
}

// RequireTransportSecurity returns false because development environments dial
// an in-cluster api-server address in plaintext (the hop never leaves the
// cluster). External addresses are dialed with TLS; the transport is selected
// from the dial address by the grpctransport package.
func (c *GRPCCredentials) RequireTransportSecurity() bool {
	return false
}
