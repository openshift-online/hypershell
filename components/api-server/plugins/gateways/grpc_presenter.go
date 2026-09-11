package gateways

import (
	"encoding/json"

	"github.com/golang/glog"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func gatewayToProto(d *Gateway) *pb.Gateway {
	gw := &pb.Gateway{
		Metadata: &pb.ObjectReference{
			Id:          d.ID,
			CreatedAt:   timestamppb.New(d.CreatedAt),
			UpdatedAt:   timestamppb.New(d.UpdatedAt),
			Kind:        "Gateway",
			Href:        "/api/hypershell/v1/gateways/" + d.ID,
			Traceparent: d.Traceparent,
			Tracestate:  d.Tracestate,
		},
		Name:             d.Name,
		ClusterId:        d.ClusterId,
		ReleaseId:        d.ReleaseId,
		DatabaseId:       d.DatabaseId,
		Namespace:        d.Namespace,
		ExternalDns:      d.ExternalDns,
		TlsMode:          d.TlsMode,
		ServiceType:      d.ServiceType,
		Status:           d.Status,
		Phase:            d.Phase,
		Image:            d.Image,
		SupervisorImage:  d.SupervisorImage,
		RouteAddress:     d.RouteAddress,
		ConsoleAddress:   d.ConsoleAddress,
		GatewayVersion:   d.GatewayVersion,
		Oidc:             d.Oidc,
		Route:            d.Route,
		CredentialDriver: d.CredentialDriver,
		ActiveSandboxCount: func() *int32 {
			if d.ActiveSandboxCount != nil {
				v := int32(*d.ActiveSandboxCount)
				return &v
			}
			return nil
		}(),
	}

	if d.ServerDnsNames != nil {
		var names []string
		if err := json.Unmarshal([]byte(*d.ServerDnsNames), &names); err == nil {
			gw.ServerDnsNames = names
		}
	}

	if d.ProvisioningConditions != nil {
		var conditions []struct {
			Type            string `json:"type"`
			ConditionStatus string `json:"condition_status"`
			Message         string `json:"message"`
		}
		if err := json.Unmarshal([]byte(*d.ProvisioningConditions), &conditions); err != nil {
			glog.Warningf("failed to unmarshal provisioning_conditions for gateway %s: %v", d.ID, err)
		} else {
			for _, c := range conditions {
				gw.ProvisioningConditions = append(gw.ProvisioningConditions, &pb.ProvisioningCondition{
					Type:            c.Type,
					ConditionStatus: conditionStatusToProto(c.ConditionStatus),
					Message:         c.Message,
				})
			}
		}
	}

	return gw
}

func conditionStatusToProto(s string) pb.ProvisioningConditionStatus {
	switch s {
	case "Pending":
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING
	case "InProgress":
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS
	case "Complete":
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE
	case "Failed":
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_FAILED
	default:
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_UNSPECIFIED
	}
}

func conditionStatusFromProto(s pb.ProvisioningConditionStatus) string {
	switch s {
	case pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING:
		return "Pending"
	case pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS:
		return "InProgress"
	case pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE:
		return "Complete"
	case pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_FAILED:
		return "Failed"
	default:
		glog.Warningf("unknown ProvisioningConditionStatus enum value: %d", int32(s))
		return "Pending"
	}
}
