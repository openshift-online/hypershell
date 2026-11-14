// Package e2e exercises the sidecar the way the gateway does: a real gRPC
// client over a real Unix socket, authenticated with a gateway-style
// extension JWT. It is still an in-process test (no pod, no gateway), but it
// covers the full serve path: socket creation, metadata auth, Describe
// negotiation, and Evaluate.
package e2e

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/auth"
	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/server"
	pb "github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/pkg/api/grpc"
)

func TestServeOverUnixSocket(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating keypair: %v", err)
	}
	verifier, err := auth.NewVerifier(server.ExpectedAudience, publicKey, 30*time.Second)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	socketPath := filepath.Join(t.TempDir(), "interceptors", "sandbox-resources.sock")
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatalf("creating socket directory: %v", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listening: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(auth.UnaryInterceptor(verifier)))
	pb.RegisterGatewayInterceptorServer(grpcServer, server.New("e2e-test"))
	stopped := make(chan struct{})
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			select {
			case <-stopped:
				// Stop() closed the listener; not a failure.
			default:
				t.Errorf("Serve: %v", err)
			}
		}
	}()
	t.Cleanup(func() {
		close(stopped)
		grpcServer.Stop()
	})

	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dialing socket: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()
	client := pb.NewGatewayInterceptorClient(conn)

	ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer "+mintToken(t, privateKey))

	t.Run("Describe", func(t *testing.T) {
		manifest, err := client.Describe(ctx, &pb.DescribeRequest{Gateway: &pb.PeerMetadata{
			ProtocolVersion:       &pb.ProtocolVersion{Major: 1, Minor: 0},
			ImplementationName:    "openshell-gateway",
			SupportedCapabilities: []string{"openshell.gateway-interceptor.contract"},
			RequiredCapabilities:  []string{"openshell.gateway-interceptor.contract"},
		}})
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}
		if manifest.GetName() != server.Name {
			t.Errorf("name = %q, want %q", manifest.GetName(), server.Name)
		}
		if manifest.GetExpectedAudience() != server.ExpectedAudience {
			t.Errorf("expected_audience = %q", manifest.GetExpectedAudience())
		}
	})

	t.Run("Evaluate allows", func(t *testing.T) {
		result, err := client.Evaluate(ctx, &pb.InterceptorEvaluation{
			InterceptorName: server.Name,
			BindingId:       server.CreateSandboxBindingID,
			Service:         "openshell.v1.OpenShell",
			Method:          "CreateSandbox",
			Phase: &pb.InterceptorEvaluation_ModifyOperation{
				ModifyOperation: &pb.ModifyOperationEvaluation{},
			},
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if !result.GetAllowed() {
			t.Error("allowed = false, want true")
		}
	})

	t.Run("unauthenticated without credential", func(t *testing.T) {
		_, err := client.Describe(t.Context(), &pb.DescribeRequest{})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("Describe code = %v, want Unauthenticated (err: %v)", status.Code(err), err)
		}
	})

	t.Run("unauthenticated with foreign key", func(t *testing.T) {
		_, otherPrivate, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generating foreign keypair: %v", err)
		}
		foreignCtx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer "+mintToken(t, otherPrivate))
		_, err = client.Describe(foreignCtx, &pb.DescribeRequest{})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("Describe code = %v, want Unauthenticated (err: %v)", status.Code(err), err)
		}
	})
}

// mintToken encodes a gateway extension JWT the way the gateway's
// ExtensionJwtIssuer does.
func mintToken(t *testing.T, key ed25519.PrivateKey) string {
	t.Helper()
	now := time.Now()
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "openshell-gateway:e2e",
			Subject:   "openshell-gateway:e2e",
			Audience:  jwt.ClaimStrings{server.ExpectedAudience},
			ID:        "e2e-token",
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
		CallerKind: auth.CallerKindGateway,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["typ"] = auth.ExtensionJWTType
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}
