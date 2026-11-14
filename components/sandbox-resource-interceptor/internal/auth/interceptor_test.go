package auth

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestUnaryInterceptor(t *testing.T) {
	verifier, keys := newTestVerifier(t)
	interceptor := UnaryInterceptor(verifier)
	handler := func(ctx context.Context, _ any) (any, error) {
		claims, ok := ClaimsFromContext(ctx)
		if !ok {
			t.Error("handler found no verified claims in context")
		}
		if claims.Issuer != "openshell-gateway:abc123" {
			t.Errorf("claims issuer = %q", claims.Issuer)
		}
		return "ok", nil
	}

	t.Run("valid credential", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			"authorization", "Bearer "+keys.mintToken(t, nil),
		))
		got, err := interceptor(ctx, nil, nil, handler)
		if err != nil {
			t.Fatalf("interceptor: %v", err)
		}
		if got != "ok" {
			t.Errorf("handler result = %v", got)
		}
	})

	t.Run("missing metadata", func(t *testing.T) {
		_, err := interceptor(context.Background(), nil, nil, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("code = %v, want Unauthenticated", status.Code(err))
		}
	})

	t.Run("missing authorization header", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{}))
		_, err := interceptor(ctx, nil, nil, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("code = %v, want Unauthenticated", status.Code(err))
		}
	})

	t.Run("duplicate authorization headers", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			"authorization", "Bearer one",
			"authorization", "Bearer two",
		))
		_, err := interceptor(ctx, nil, nil, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("code = %v, want Unauthenticated", status.Code(err))
		}
	})

	t.Run("wrong scheme", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			"authorization", "Basic dXNlcjpwYXNz",
		))
		_, err := interceptor(ctx, nil, nil, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("code = %v, want Unauthenticated", status.Code(err))
		}
	})

	t.Run("empty bearer", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			"authorization", "Bearer ",
		))
		_, err := interceptor(ctx, nil, nil, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("code = %v, want Unauthenticated", status.Code(err))
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			"authorization", "Bearer not-a-token",
		))
		_, err := interceptor(ctx, nil, nil, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("code = %v, want Unauthenticated", status.Code(err))
		}
	})
}
