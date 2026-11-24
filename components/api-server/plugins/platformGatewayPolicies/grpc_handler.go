package platformGatewayPolicies

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server/grpcutil"
)

func brokerSubscribe(ctx context.Context, brokerFunc func() *pkgserver.EventBroker, name string) (*pkgserver.Subscription, error) {
	broker := brokerFunc()
	if broker == nil {
		return nil, status.Error(codes.Unavailable, "event broker not available")
	}
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	slog.DebugContext(ctx, name+" subscriber connected", "subscriber_id", sub.ID)
	return sub, nil
}

type policyGRPCHandler struct {
	pb.UnimplementedPlatformGatewayPolicyServiceServer
	service    PolicyService
	brokerFunc func() *pkgserver.EventBroker
}

func (h *policyGRPCHandler) GetPlatformGatewayPolicy(ctx context.Context, req *pb.GetPlatformGatewayPolicyRequest) (*pb.GetPlatformGatewayPolicyResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	out, err := policyToProto(item)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.GetPlatformGatewayPolicyResponse{PlatformGatewayPolicy: out}, nil
}

// ListPlatformGatewayPolicies returns every policy. The set is small and the
// control plane needs all of it to build bundles, so the page and size are
// reported but the full set is returned in one page.
func (h *policyGRPCHandler) ListPlatformGatewayPolicies(ctx context.Context, req *pb.ListPlatformGatewayPoliciesRequest) (*pb.ListPlatformGatewayPoliciesResponse, error) {
	items, svcErr := h.service.All(ctx)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	out := make([]*pb.PlatformGatewayPolicy, 0, len(items))
	for _, item := range items {
		p, err := policyToProto(item)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		out = append(out, p)
	}
	return &pb.ListPlatformGatewayPoliciesResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: 1, Size: int32(len(out)), Total: int32(len(out))},
	}, nil
}

func (h *policyGRPCHandler) WatchPlatformGatewayPolicies(_ *pb.WatchPlatformGatewayPoliciesRequest, stream grpc.ServerStreamingServer[pb.WatchPlatformGatewayPoliciesResponse]) error {
	ctx := stream.Context()
	sub, err := brokerSubscribe(ctx, h.brokerFunc, "WatchPlatformGatewayPolicies")
	if err != nil {
		return err
	}
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != policyEventSource {
				continue
			}
			resp := &pb.WatchPlatformGatewayPoliciesResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					slog.WarnContext(ctx, "WatchPlatformGatewayPolicies failed to load policy", "resource_id", evt.SourceID, "error", svcErr)
					continue
				}
				p, err := policyToProto(item)
				if err != nil {
					slog.WarnContext(ctx, "WatchPlatformGatewayPolicies failed to encode policy", "resource_id", evt.SourceID, "error", err)
					continue
				}
				resp.PlatformGatewayPolicy = p
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}

func (h *policyGRPCHandler) UpdatePlatformGatewayPolicyStatus(ctx context.Context, req *pb.UpdatePlatformGatewayPolicyStatusRequest) (*pb.UpdatePlatformGatewayPolicyStatusResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	entries := make([]GatewayStatus, 0, len(req.GatewayStatuses))
	for _, g := range req.GatewayStatuses {
		entry := GatewayStatus{
			GatewayID:             g.GatewayId,
			Condition:             g.Condition,
			Revision:              g.Revision,
			AppliedRevision:       g.AppliedRevision,
			EffectiveMode:         g.EffectiveMode,
			Message:               g.Message,
			NonCompliantSandboxes: g.NonCompliantSandboxes,
		}
		if g.ObservedAt != nil {
			t := g.ObservedAt.AsTime()
			entry.ObservedAt = &t
		}
		entries = append(entries, entry)
	}
	updated, svcErr := h.service.UpdateStatus(ctx, req.Id, entries, req.RemovedGatewayIds)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	out, err := policyToProto(updated)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.UpdatePlatformGatewayPolicyStatusResponse{PlatformGatewayPolicy: out}, nil
}

type settingsGRPCHandler struct {
	pb.UnimplementedPlatformPolicySettingsServiceServer
	service    PolicyService
	brokerFunc func() *pkgserver.EventBroker
}

func (h *settingsGRPCHandler) GetPlatformPolicySettings(ctx context.Context, _ *pb.GetPlatformPolicySettingsRequest) (*pb.GetPlatformPolicySettingsResponse, error) {
	st, svcErr := h.service.Settings(ctx)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	out, err := settingsToProto(st)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.GetPlatformPolicySettingsResponse{Settings: out}, nil
}

// WatchPlatformPolicySettings sends the current settings first, then the new
// settings after every change, so a control plane always starts from the
// authoritative mode and fleet defaults (PLAT-PGP-08).
func (h *settingsGRPCHandler) WatchPlatformPolicySettings(_ *pb.WatchPlatformPolicySettingsRequest, stream grpc.ServerStreamingServer[pb.WatchPlatformPolicySettingsResponse]) error {
	ctx := stream.Context()
	sub, err := brokerSubscribe(ctx, h.brokerFunc, "WatchPlatformPolicySettings")
	if err != nil {
		return err
	}
	send := func() error {
		st, svcErr := h.service.Settings(ctx)
		if svcErr != nil {
			return grpcutil.ServiceErrorToGRPC(svcErr)
		}
		out, err := settingsToProto(st)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		return stream.Send(&pb.WatchPlatformPolicySettingsResponse{Settings: out})
	}
	if err := send(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != settingsEventSource {
				continue
			}
			if err := send(); err != nil {
				return err
			}
		}
	}
}

type findingGRPCHandler struct {
	pb.UnimplementedPolicyAuditFindingServiceServer
	service PolicyService
}

// maxFindingsPerReport bounds one forwarded batch.
const maxFindingsPerReport = 500

func (h *findingGRPCHandler) ReportPolicyAuditFindings(ctx context.Context, req *pb.ReportPolicyAuditFindingsRequest) (*pb.ReportPolicyAuditFindingsResponse, error) {
	if req.GatewayId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id is required")
	}
	if len(req.Findings) > maxFindingsPerReport {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d findings per report", maxFindingsPerReport)
	}
	findings := make([]Finding, 0, len(req.Findings))
	for _, f := range req.Findings {
		if f.GatewayId != req.GatewayId {
			return nil, status.Error(codes.InvalidArgument, "every finding must name the request gateway_id")
		}
		var occurred time.Time
		if f.OccurredAt != nil {
			occurred = f.OccurredAt.AsTime()
		}
		findings = append(findings, Finding{
			ID:               f.Id,
			OccurredAt:       occurred,
			GatewayID:        f.GatewayId,
			PolicyNames:      f.PolicyNames,
			BundleRevision:   f.BundleRevision,
			PrincipalSubject: f.PrincipalSubject,
			Operation:        f.Operation,
			Outcome:          f.Outcome,
			RequestSummary:   f.RequestSummary,
		})
	}
	n, svcErr := h.service.ReportFindings(ctx, findings)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.ReportPolicyAuditFindingsResponse{Stored: int32(n)}, nil
}
