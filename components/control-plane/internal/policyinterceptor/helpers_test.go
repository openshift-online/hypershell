package policyinterceptor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	gi "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/gatewayinterceptorv1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
)

const (
	testGatewayID = "gw-test-1"
	testIssuer    = "openshell-gateway:" + testGatewayID
)

func testDefaults(t *testing.T) DefaultQuantities {
	t.Helper()
	d := DefaultQuantities{CPURequest: "500m", CPULimit: "2", MemoryRequest: "1Gi", MemoryLimit: "4Gi"}
	parsed, err := cedar.ParseDefaults(d.CPURequest, d.CPULimit, d.MemoryRequest, d.MemoryLimit)
	if err != nil {
		t.Fatalf("parse defaults: %v", err)
	}
	d.Parsed = parsed
	return d
}

type testPolicy struct {
	name, mode, message, doc, cedar string
}

func cpuOver(n string) string {
	return `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource) when { context.cpuRequestMillis > ` + n + ` };`
}

func memLimitOver(n string) string {
	return `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource) when { context.memoryLimitBytes > ` + n + ` };`
}

// buildBundle builds a real bundle through the control-plane bundle builder.
func buildBundle(t *testing.T, gatewayID string, unsupported bool, policies ...testPolicy) []byte {
	t.Helper()
	var in []*pb.PlatformGatewayPolicy
	for i, p := range policies {
		in = append(in, &pb.PlatformGatewayPolicy{
			Metadata:         &pb.ObjectReference{Id: "id-" + p.name + "-" + string(rune('a'+i))},
			Name:             p.name,
			Target:           &pb.PlatformGatewayPolicyTarget{All: true},
			EnforcementMode:  p.mode,
			UserMessage:      p.message,
			DocumentationUrl: p.doc,
			Cedar:            p.cedar,
		})
	}
	b, skipped, err := policybundle.Build(policybundle.Input{
		GatewayID:   gatewayID,
		GlobalMode:  policybundle.ModeAudit,
		Policies:    in,
		Defaults:    testDefaults(t).Parsed,
		Unsupported: unsupported,
	})
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	if len(skipped) > 0 {
		t.Fatalf("bundle skipped policies: %+v", skipped)
	}
	raw, err := b.Encode()
	if err != nil {
		t.Fatalf("encode bundle: %v", err)
	}
	return raw
}

func newTestServer(t *testing.T, bundles ...[]byte) *Server {
	t.Helper()
	store := NewStore(testGatewayID)
	for _, raw := range bundles {
		if _, err := store.Apply(raw); err != nil {
			t.Fatalf("apply bundle: %v", err)
		}
	}
	return &Server{
		GatewayID:   testGatewayID,
		Audience:    DefaultAudience,
		Defaults:    testDefaults(t),
		PoliciesURL: "https://console.example/gateways/gw-test-1/policies",
		Store:       store,
		Events:      NewEventBuffer(100, nil),
		Metrics:     NewMetrics(),
		Now:         func() time.Time { return time.Unix(1700000000, 0) },
	}
}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("struct: %v", err)
	}
	return s
}

func userPrincipal() map[string]string {
	return map[string]string{"kind": "user", "subject": "alice@example.com", "roles": "user,dev"}
}

func validateEval(t *testing.T, method string, op map[string]any) *gi.InterceptorEvaluation {
	t.Helper()
	service := ServiceOpenShell
	if method == "UpdateSandbox" {
		service = ServiceSandboxUpdates
	}
	return &gi.InterceptorEvaluation{
		InterceptorName: InterceptorName,
		BindingId:       bindingID(service, method),
		Service:         service,
		Method:          method,
		Principal:       userPrincipal(),
		Phase:           &gi.InterceptorEvaluation_Validate{Validate: &gi.ValidateEvaluation{ProposedOperation: mustStruct(t, op)}},
	}
}

func modifyEval(t *testing.T, op map[string]any) *gi.InterceptorEvaluation {
	t.Helper()
	return &gi.InterceptorEvaluation{
		InterceptorName: InterceptorName,
		BindingId:       "create-sandbox",
		Service:         ServiceOpenShell,
		Method:          "CreateSandbox",
		Principal:       userPrincipal(),
		Phase:           &gi.InterceptorEvaluation_ModifyOperation{ModifyOperation: &gi.ModifyOperationEvaluation{ProposedOperation: mustStruct(t, op)}},
	}
}

// createOp returns a CreateSandboxRequest in protobuf JSON with the given
// template resources (nil for none).
func createOp(name string, requests, limits map[string]any) map[string]any {
	tmpl := map[string]any{"image": "quay.io/example/agent:1"}
	if requests != nil || limits != nil {
		res := map[string]any{}
		if requests != nil {
			res["requests"] = requests
		}
		if limits != nil {
			res["limits"] = limits
		}
		tmpl["resources"] = res
	}
	return map[string]any{
		"name":           name,
		"workspaceScope": map[string]any{"workspace": "default"},
		"spec":           map[string]any{"template": tmpl},
	}
}

// --- JWT and in-process gRPC over a unix socket ---

type jwtKit struct {
	priv    ed25519.PrivateKey
	keyFile string
}

func newJWTKit(t *testing.T) *jwtKit {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "public.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return &jwtKit{priv: priv, keyFile: path}
}

type tokenOpts struct {
	typ, iss, aud, callerKind string
	exp                       time.Time
	key                       ed25519.PrivateKey
}

func (k *jwtKit) token(t *testing.T, mutate func(*tokenOpts)) string {
	t.Helper()
	o := tokenOpts{typ: extensionJWTTyp, iss: testIssuer, aud: DefaultAudience, callerKind: "gateway", exp: time.Now().Add(5 * time.Minute), key: k.priv}
	if mutate != nil {
		mutate(&o)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"iss": o.iss, "aud": o.aud, "sub": "gateway", "iat": time.Now().Unix(), "exp": o.exp.Unix(),
		"jti": "j1", "caller_kind": o.callerKind,
	})
	tok.Header["typ"] = o.typ
	signed, err := tok.SignedString(o.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// startGRPC serves srv over a temp unix socket with JWT verification and
// returns a client.
func startGRPC(t *testing.T, srv *Server, kit *jwtKit) gi.GatewayInterceptorClient {
	t.Helper()
	verifier, err := NewVerifier(kit.keyFile, testIssuer, DefaultAudience)
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	dir, err := os.MkdirTemp("", "pgp")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "i.sock")
	lis, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer(grpc.ChainUnaryInterceptor(verifier.UnaryServerInterceptor(nil)))
	gi.RegisterGatewayInterceptorServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		}))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return gi.NewGatewayInterceptorClient(conn)
}

func withToken(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

// metricValue scrapes the metrics handler and returns the value of the first
// sample whose name (with labels) starts with prefix.
func metricValue(t *testing.T, m *Metrics, prefix string) float64 {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.HasPrefix(line, "#") || !strings.HasPrefix(line, prefix) {
			continue
		}
		fields := strings.Fields(line)
		v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			t.Fatalf("parse metric line %q: %v", line, err)
		}
		return v
	}
	return 0
}
