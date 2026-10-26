package rbac

import (
	"context"
	"reflect"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/metadata"
)

func grpcBearer(t *testing.T, header string, claims jwt.MapClaims) context.Context {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-key"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", header+signed))
}

// On gRPC the framework's auth interceptor stores only the username, so realm
// roles must come from the verified authorization metadata. Without them the
// role sync revoked a caller's platform:admin binding on every gRPC call.
func TestExtractJWTRolesFromGRPCMetadata(t *testing.T) {
	claims := jwt.MapClaims{
		"preferred_username": "admin",
		"realm_access":       map[string]interface{}{"roles": []interface{}{"platform:admin", "gateway:creator"}},
	}
	want := []string{"platform:admin", "gateway:creator"}
	for _, header := range []string{"Bearer ", "bearer ", ""} {
		if got := extractJWTRolesFromContext(grpcBearer(t, header, claims)); !reflect.DeepEqual(got, want) {
			t.Fatalf("header %q: roles = %v, want %v", header, got, want)
		}
	}
	if got := extractJWTRolesFromContext(context.Background()); got != nil {
		t.Fatalf("no metadata: roles = %v, want none", got)
	}
	bad := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer not-a-jwt"))
	if got := extractJWTRolesFromContext(bad); got != nil {
		t.Fatalf("malformed token: roles = %v, want none", got)
	}
}
