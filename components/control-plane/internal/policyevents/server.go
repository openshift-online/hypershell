// Package policyevents hosts the in-cluster ReportPolicyEvents endpoint the
// policy interceptor sidecars report to (CTRL-CORE-22, PLAT-PGP-15,
// SEC-RBAC-32). Each call is authenticated with the sidecar's projected,
// audience-bound service-account token (TokenReview); the token's namespace
// must be the reporting gateway's namespace on this cluster. Accepted events
// are forwarded to the api-server under the control plane's own registered
// identity, and the applied bundle revision is recorded for status write-back.
package policyevents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	eventspb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/policyevents/v1"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// Audience is the audience the sidecar's projected token is bound to. The
// chart projects the token with this audience; TokenReview checks it.
const Audience = "hypershell-policy-events"

// maxEventsPerReport bounds one report (the api-server accepts at most 500 per
// forward call).
const maxEventsPerReport = 500

// TokenReviewer authenticates a projected service-account token and returns
// the namespace of its service account.
type TokenReviewer interface {
	Review(ctx context.Context, token string) (namespace string, err error)
}

// GatewayResolver returns the namespace of a gateway on this cluster, or
// found=false when the gateway does not exist or belongs to another cluster.
type GatewayResolver func(ctx context.Context, gatewayID string) (namespace string, found bool, err error)

// RevisionRecorder records the bundle revision a gateway's sidecar applies.
type RevisionRecorder func(gatewayID, revision string)

// Server implements PolicyEventService.
type Server struct {
	eventspb.UnimplementedPolicyEventServiceServer
	reviewer TokenReviewer
	gateways GatewayResolver
	findings pb.PolicyAuditFindingServiceClient
	applied  RevisionRecorder
}

// NewServer builds the server.
func NewServer(reviewer TokenReviewer, gateways GatewayResolver, findings pb.PolicyAuditFindingServiceClient, applied RevisionRecorder) *Server {
	return &Server{reviewer: reviewer, gateways: gateways, findings: findings, applied: applied}
}

func bearer(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("missing metadata")
	}
	for _, v := range md.Get("authorization") {
		if token, ok := strings.CutPrefix(v, "Bearer "); ok && token != "" {
			return token, nil
		}
	}
	return "", errors.New("missing bearer token")
}

// ReportPolicyEvents authenticates the sidecar, checks the namespace-to-gateway
// binding, records the applied revision, and forwards the events.
func (s *Server) ReportPolicyEvents(ctx context.Context, req *eventspb.ReportPolicyEventsRequest) (*eventspb.ReportPolicyEventsResponse, error) {
	token, err := bearer(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	namespace, err := s.reviewer.Review(ctx, token)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "token review rejected the caller")
	}
	if req.GetGatewayId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id is required")
	}
	if len(req.GetEvents()) > maxEventsPerReport {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d events per report", maxEventsPerReport)
	}
	gwNamespace, found, err := s.gateways(ctx, req.GetGatewayId())
	if err != nil {
		return nil, status.Error(codes.Unavailable, "gateway lookup failed")
	}
	// A sidecar may report only for the gateway in its own namespace
	// (SEC-RBAC-32.01).
	if !found || gwNamespace != namespace {
		return nil, status.Error(codes.PermissionDenied, "the caller's namespace does not belong to the reporting gateway")
	}
	if req.GetAppliedRevision() != "" && s.applied != nil {
		s.applied(req.GetGatewayId(), req.GetAppliedRevision())
	}
	if len(req.GetEvents()) == 0 {
		return &eventspb.ReportPolicyEventsResponse{}, nil
	}
	forward := &pb.ReportPolicyAuditFindingsRequest{GatewayId: req.GetGatewayId()}
	for _, e := range req.GetEvents() {
		forward.Findings = append(forward.Findings, &pb.PolicyAuditFinding{
			Id:               e.GetId(),
			OccurredAt:       e.GetOccurredAt(),
			GatewayId:        req.GetGatewayId(),
			PolicyNames:      e.GetPolicyNames(),
			BundleRevision:   e.GetBundleRevision(),
			PrincipalSubject: e.GetPrincipalSubject(),
			Operation:        e.GetOperation(),
			Outcome:          e.GetOutcome(),
			RequestSummary:   e.GetRequestSummary(),
		})
	}
	if _, err := s.findings.ReportPolicyAuditFindings(ctx, forward); err != nil {
		// Unavailable lets the sidecar keep the events buffered and retry; the
		// event ids make the retry idempotent.
		return nil, status.Errorf(codes.Unavailable, "forward findings: %v", status.Convert(err).Message())
	}
	return &eventspb.ReportPolicyEventsResponse{Accepted: int32(len(req.GetEvents()))}, nil
}

// KubeTokenReviewer verifies tokens with the Kubernetes TokenReview API.
type KubeTokenReviewer struct {
	Kube kubernetes.Interface
}

// Review returns the namespace of an authenticated service-account token bound
// to Audience.
func (k KubeTokenReviewer) Review(ctx context.Context, token string) (string, error) {
	tr, err := k.Kube.AuthenticationV1().TokenReviews().Create(ctx, &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{Token: token, Audiences: []string{Audience}},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("token review: %w", err)
	}
	if !tr.Status.Authenticated {
		return "", errors.New("token not authenticated")
	}
	audienceOK := false
	for _, a := range tr.Status.Audiences {
		if a == Audience {
			audienceOK = true
		}
	}
	if !audienceOK {
		return "", errors.New("token not bound to the policy events audience")
	}
	parts := strings.Split(tr.Status.User.Username, ":")
	if len(parts) != 4 || parts[0] != "system" || parts[1] != "serviceaccount" || parts[2] == "" {
		return "", errors.New("token is not a service-account token")
	}
	return parts[2], nil
}

// GRPCGatewayResolver resolves gateways over the api-server connection and
// accepts only gateways assigned to clusterID.
func GRPCGatewayResolver(client pb.GatewayServiceClient, clusterID string) GatewayResolver {
	return func(ctx context.Context, gatewayID string) (string, bool, error) {
		resp, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
		if status.Code(err) == codes.NotFound {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		gw := resp.GetGateway()
		if gw.GetClusterId() != clusterID {
			return "", false, nil
		}
		return gw.GetNamespace(), true, nil
	}
}

// ListenAndServe serves the endpoint (plaintext in-cluster gRPC, restricted by
// NetworkPolicy to gateway pods) until ctx ends.
func ListenAndServe(ctx context.Context, address string, srv *Server) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	gs := grpc.NewServer(grpc.ConnectionTimeout(10 * time.Second))
	eventspb.RegisterPolicyEventServiceServer(gs, srv)
	go func() {
		<-ctx.Done()
		gs.GracefulStop()
	}()
	slog.InfoContext(ctx, "policy events endpoint listening", "address", address)
	if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("serve policy events: %w", err)
	}
	return nil
}
