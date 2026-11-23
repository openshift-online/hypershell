package policyevents

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	eventspb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/policyevents/v1"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

type fakeReviewer map[string]string // token -> namespace

func (f fakeReviewer) Review(_ context.Context, token string) (string, error) {
	if ns, ok := f[token]; ok {
		return ns, nil
	}
	return "", errors.New("unauthenticated")
}

type fakeFindings struct {
	pb.PolicyAuditFindingServiceClient
	got []*pb.ReportPolicyAuditFindingsRequest
}

func (f *fakeFindings) ReportPolicyAuditFindings(_ context.Context, req *pb.ReportPolicyAuditFindingsRequest, _ ...grpc.CallOption) (*pb.ReportPolicyAuditFindingsResponse, error) {
	f.got = append(f.got, req)
	return &pb.ReportPolicyAuditFindingsResponse{Stored: int32(len(req.Findings))}, nil
}

func withToken(token string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
}

func gateways(ctx context.Context, id string) (string, bool, error) {
	switch id {
	case "gw-g":
		return "ns-g", true, nil
	case "gw-h":
		return "ns-h", true, nil
	}
	return "", false, nil
}

// Verifies: SEC-RBAC-32.01, SEC-RBAC-32.02, CTRL-CORE-22.02, PLAT-PGP-15.01
func TestReportPolicyEventsIsNamespaceChecked(t *testing.T) {
	findings := &fakeFindings{}
	var applied []string
	s := NewServer(fakeReviewer{"tok-g": "ns-g"}, gateways, findings, func(gw, rev string) { applied = append(applied, gw+"="+rev) })
	req := &eventspb.ReportPolicyEventsRequest{GatewayId: "gw-g", AppliedRevision: "rev1", Events: []*eventspb.PolicyEvent{{Id: "e1", Outcome: "would_reject"}}}

	resp, err := s.ReportPolicyEvents(withToken("tok-g"), req)
	if err != nil || resp.GetAccepted() != 1 {
		t.Fatalf("own-namespace report must be accepted: %v", err)
	}
	if len(findings.got) != 1 || findings.got[0].Findings[0].GetGatewayId() != "gw-g" || len(applied) != 1 {
		t.Fatalf("events must be forwarded with the gateway id and the revision recorded: %+v %v", findings.got, applied)
	}

	// The sidecar of G cannot report for H.
	_, err = s.ReportPolicyEvents(withToken("tok-g"), &eventspb.ReportPolicyEventsRequest{GatewayId: "gw-h"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("cross-namespace report = %v, want PermissionDenied", err)
	}
	// A user or gateway caller without a valid sidecar token is rejected.
	_, err = s.ReportPolicyEvents(withToken("user-jwt"), req)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unknown token = %v, want Unauthenticated", err)
	}
	_, err = s.ReportPolicyEvents(context.Background(), req)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing token = %v, want Unauthenticated", err)
	}
}

// Verifies: SEC-RBAC-32.01
func TestKubeTokenReviewerRequiresAudienceAndServiceAccount(t *testing.T) {
	kube := fake.NewSimpleClientset()
	result := authv1.TokenReviewStatus{}
	kube.PrependReactor("create", "tokenreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.TokenReview{Status: result}, nil
	})
	r := KubeTokenReviewer{Kube: kube}
	result = authv1.TokenReviewStatus{Authenticated: true, Audiences: []string{Audience}, User: authv1.UserInfo{Username: "system:serviceaccount:ns-g:openshell-gateway"}}
	if ns, err := r.Review(context.Background(), "t"); err != nil || ns != "ns-g" {
		t.Fatalf("valid token: ns=%q err=%v", ns, err)
	}
	result = authv1.TokenReviewStatus{Authenticated: true, Audiences: []string{"other"}, User: authv1.UserInfo{Username: "system:serviceaccount:ns-g:x"}}
	if _, err := r.Review(context.Background(), "t"); err == nil {
		t.Fatal("a token for another audience must be rejected")
	}
	result = authv1.TokenReviewStatus{Authenticated: true, Audiences: []string{Audience}, User: authv1.UserInfo{Username: "alice"}}
	if _, err := r.Review(context.Background(), "t"); err == nil {
		t.Fatal("a non-service-account identity must be rejected")
	}
}
