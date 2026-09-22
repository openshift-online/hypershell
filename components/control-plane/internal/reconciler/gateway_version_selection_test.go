package reconciler

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// releaseServer serves a fixed GetGatewayRelease response (or error) so the
// image-selection code path can be exercised over a real gRPC connection,
// mirroring the ManagedDatabase fakes in gateway_delete_db_notfound_test.go.
type releaseServer struct {
	pb.UnimplementedGatewayReleaseServiceServer
	release *pb.GatewayRelease
	err     error
}

func (s releaseServer) GetGatewayRelease(context.Context, *pb.GetGatewayReleaseRequest) (*pb.GetGatewayReleaseResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &pb.GetGatewayReleaseResponse{GatewayRelease: s.release}, nil
}

// dialReleaseServer stands up an in-process gRPC GatewayReleaseService and
// returns a client connection to it.
func dialReleaseServer(t *testing.T, srv releaseServer) *grpc.ClientConn {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test: %v", err)
	}
	server := grpc.NewServer()
	pb.RegisterGatewayReleaseServiceServer(server, srv)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial gateway release service: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func strptr(s string) *string { return &s }

// A gateway that references a release resolves to that release's image, and the
// resolved image takes precedence over any direct image on the gateway.
func TestSelectGatewayImage_ReleasePrecedesDirectImage(t *testing.T) {
	t.Setenv("GATEWAY_IMAGE", "registry.redhat.io/openshell/gateway:platform-default")
	conn := dialReleaseServer(t, releaseServer{
		release: &pb.GatewayRelease{
			Metadata: &pb.ObjectReference{Id: "r1"},
			Image:    "registry.redhat.io/openshell/gateway:v2",
		},
	})
	r := &GatewayReconciler{grpcConn: conn}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gw := &pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "g1"},
		Name:      "g1",
		ReleaseId: "r1",
		Image:     strptr("registry.redhat.io/openshell/gateway:v1"),
	}
	img, err := r.selectGatewayImage(ctx, gw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img != "registry.redhat.io/openshell/gateway:v2" {
		t.Fatalf("release image must take precedence, got %q", img)
	}
}

// The primary database-backed path: a gateway that references a release and
// carries no direct image resolves to the release image.
func TestSelectGatewayImage_ReleaseOnly(t *testing.T) {
	t.Setenv("GATEWAY_IMAGE", "registry.redhat.io/openshell/gateway:platform-default")
	conn := dialReleaseServer(t, releaseServer{
		release: &pb.GatewayRelease{
			Metadata: &pb.ObjectReference{Id: "r1"},
			Image:    "registry.redhat.io/openshell/gateway:v2",
		},
	})
	r := &GatewayReconciler{grpcConn: conn}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	img, err := r.selectGatewayImage(ctx, &pb.Gateway{Name: "g1", ReleaseId: "r1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img != "registry.redhat.io/openshell/gateway:v2" {
		t.Fatalf("expected release image, got %q", img)
	}
}

// With no release_id the direct image is used.
func TestSelectGatewayImage_DirectImageFallback(t *testing.T) {
	t.Setenv("GATEWAY_IMAGE", "registry.redhat.io/openshell/gateway:platform-default")
	r := &GatewayReconciler{} // no grpcConn needed: release path not taken

	img, err := r.selectGatewayImage(context.Background(), &pb.Gateway{
		Name:  "g1",
		Image: strptr("registry.redhat.io/openshell/gateway:v1"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img != "registry.redhat.io/openshell/gateway:v1" {
		t.Fatalf("expected direct image, got %q", img)
	}
}

// Gateways without a release or direct image use GATEWAY_IMAGE in Helm values.
func TestSelectGatewayImage_PlatformDefault(t *testing.T) {
	const defaultImage = "quay.io/opendatahub/odh-openshell-gateway:v0.0.109-rhaiv.0@sha256:a80b79e514826e8d57ea137749cf18a6e7f3d92e26bfefe005f3a9c4a55b8bdd"
	t.Setenv("GATEWAY_IMAGE", defaultImage)

	for _, tc := range []struct {
		name  string
		image *string
	}{
		{name: "absent image"},
		{name: "empty image", image: strptr("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &GatewayReconciler{}
			img, err := r.selectGatewayImage(context.Background(), &pb.Gateway{Name: "g1", Image: tc.image})
			if err != nil {
				t.Fatalf("select default image: %v", err)
			}
			if img != defaultImage {
				t.Fatalf("expected platform image %q, got %q", defaultImage, img)
			}

			builder := helm.ValuesBuilder{
				Gateway:   helm.GatewayConfig{Image: img},
				Namespace: "openshell-test",
			}
			values, err := builder.Build()
			if err != nil {
				t.Fatalf("build Helm values: %v", err)
			}
			image, ok := values["image"].(map[string]interface{})
			if !ok {
				t.Fatal("Helm values must include the gateway image override")
			}
			if image["repository"] != "quay.io/opendatahub/odh-openshell-gateway" || image["tag"] != "v0.0.109-rhaiv.0" {
				t.Fatalf("unexpected Helm image values: %v", image)
			}
		})
	}
}

func TestSelectGatewayImage_MissingPlatformDefaultFails(t *testing.T) {
	t.Setenv("GATEWAY_IMAGE", "")
	r := &GatewayReconciler{}

	img, err := r.selectGatewayImage(context.Background(), &pb.Gateway{Name: "g1"})
	if err == nil || !strings.Contains(err.Error(), "GATEWAY_IMAGE") {
		t.Fatalf("expected a GATEWAY_IMAGE configuration error, got image %q, error %v", img, err)
	}
	if img != "" {
		t.Fatalf("expected no image on error, got %q", img)
	}
}

// --- Supervisor image selection ---

// A gateway with an explicit supervisor_image uses that image.
func TestSelectSupervisorImage_DirectImage(t *testing.T) {
	t.Setenv("GATEWAY_SUPERVISOR_IMAGE", "registry.redhat.io/openshell/supervisor:platform-default")
	img := selectSupervisorImage(&pb.Gateway{
		Name:            "g1",
		SupervisorImage: strptr("registry.redhat.io/openshell/supervisor:v1"),
	})
	if img != "registry.redhat.io/openshell/supervisor:v1" {
		t.Fatalf("expected direct supervisor image, got %q", img)
	}
}

// Without a direct supervisor_image the platform default is used.
func TestSelectSupervisorImage_PlatformDefault(t *testing.T) {
	const defaultImage = "registry.redhat.io/openshell/supervisor:platform-default"
	t.Setenv("GATEWAY_SUPERVISOR_IMAGE", defaultImage)

	for _, tc := range []struct {
		name            string
		supervisorImage *string
	}{
		{name: "absent supervisor_image"},
		{name: "empty supervisor_image", supervisorImage: strptr("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := selectSupervisorImage(&pb.Gateway{Name: "g1", SupervisorImage: tc.supervisorImage})
			if img != defaultImage {
				t.Fatalf("expected platform default %q, got %q", defaultImage, img)
			}
		})
	}
}

// When neither a direct supervisor_image nor GATEWAY_SUPERVISOR_IMAGE is set,
// selectSupervisorImage returns empty so the reconciler can fail fast.
func TestSelectSupervisorImage_MissingPlatformDefaultReturnsEmpty(t *testing.T) {
	t.Setenv("GATEWAY_SUPERVISOR_IMAGE", "")
	img := selectSupervisorImage(&pb.Gateway{Name: "g1"})
	if img != "" {
		t.Fatalf("expected empty image when no default is configured, got %q", img)
	}
}

// A referenced release that does not exist fails the selection so the reconcile
// is retried; it must not silently fall back to a default or empty image.
func TestSelectGatewayImage_ReleaseNotFoundFails(t *testing.T) {
	t.Setenv("GATEWAY_IMAGE", "registry.redhat.io/openshell/gateway:platform-default")
	conn := dialReleaseServer(t, releaseServer{err: status.Error(codes.NotFound, "GatewayRelease with id='missing' not found")})
	r := &GatewayReconciler{grpcConn: conn}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	img, err := r.selectGatewayImage(ctx, &pb.Gateway{Name: "g1", ReleaseId: "missing"})
	if err == nil {
		t.Fatalf("expected error for missing release, got image %q", img)
	}
}

// A referenced release with an empty image fails the selection rather than
// deploying an empty image.
func TestSelectGatewayImage_ReleaseEmptyImageFails(t *testing.T) {
	t.Setenv("GATEWAY_IMAGE", "registry.redhat.io/openshell/gateway:platform-default")
	conn := dialReleaseServer(t, releaseServer{
		release: &pb.GatewayRelease{Metadata: &pb.ObjectReference{Id: "r1"}, Image: ""},
	})
	r := &GatewayReconciler{grpcConn: conn}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := r.selectGatewayImage(ctx, &pb.Gateway{Name: "g1", ReleaseId: "r1"}); err == nil {
		t.Fatal("expected error for release with empty image")
	}
}

// A transient lookup error fails the selection so the reconcile is retried.
func TestSelectGatewayImage_TransientLookupErrorFails(t *testing.T) {
	t.Setenv("GATEWAY_IMAGE", "registry.redhat.io/openshell/gateway:platform-default")
	conn := dialReleaseServer(t, releaseServer{err: status.Error(codes.Unavailable, "release service temporarily unavailable")})
	r := &GatewayReconciler{grpcConn: conn}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := r.selectGatewayImage(ctx, &pb.Gateway{Name: "g1", ReleaseId: "r1"}); err == nil {
		t.Fatal("expected error for transient release lookup failure")
	}
}

// An empty payload (nil GatewayRelease) is a configuration error, not a usable
// image.
func TestSelectGatewayImage_EmptyPayloadFails(t *testing.T) {
	t.Setenv("GATEWAY_IMAGE", "registry.redhat.io/openshell/gateway:platform-default")
	conn := dialReleaseServer(t, releaseServer{release: nil})
	r := &GatewayReconciler{grpcConn: conn}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := r.selectGatewayImage(ctx, &pb.Gateway{Name: "g1", ReleaseId: "r1"}); err == nil {
		t.Fatal("expected error for empty release payload")
	}
}
