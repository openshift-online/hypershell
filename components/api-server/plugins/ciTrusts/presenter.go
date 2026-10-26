package ciTrusts

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertCITrust(r openapi.CITrust) *CITrust {
	c := &CITrust{
		Meta: api.Meta{
			ID: util.NilToEmptyString(r.Id),
		},
		CIProviderID:        r.CiProviderId,
		GatewayID:           r.GatewayId,
		SubjectRepository:   r.Subject.Repository,
		SubjectProjectPath:  r.Subject.ProjectPath,
		SubjectRef:          r.Subject.Ref,
		SubjectEnvironment:  r.Subject.Environment,
		SubjectRefProtected: r.Subject.RefProtected,
		Role:                r.Role,
	}
	if r.CreatedAt != nil {
		c.CreatedAt = *r.CreatedAt
		c.UpdatedAt = *r.UpdatedAt
	}
	if r.Status != nil {
		c.StatusExchangeEndpoint = r.Status.ExchangeEndpoint
		c.StatusWorkspace = r.Status.Workspace
		c.StatusWorkflowHint = r.Status.WorkflowHint
	}
	return c
}

func PresentCITrust(t *CITrust) openapi.CITrust {
	reference := presenters.PresentReference(t.ID, t)
	result := openapi.CITrust{
		Id:           reference.Id,
		Kind:         reference.Kind,
		Href:         reference.Href,
		CreatedAt:    openapi.PtrTime(t.CreatedAt),
		UpdatedAt:    openapi.PtrTime(t.UpdatedAt),
		CiProviderId: t.CIProviderID,
		GatewayId:    t.GatewayID,
		Subject: openapi.CITrustAllOfSubject{
			Repository:   t.SubjectRepository,
			ProjectPath:  t.SubjectProjectPath,
			Ref:          t.SubjectRef,
			Environment:  t.SubjectEnvironment,
			RefProtected: t.SubjectRefProtected,
		},
		Role: t.Role,
	}
	if t.StatusExchangeEndpoint != nil || t.StatusWorkspace != nil || t.StatusWorkflowHint != nil {
		result.Status = &openapi.CITrustAllOfStatus{
			ExchangeEndpoint: t.StatusExchangeEndpoint,
			Workspace:        t.StatusWorkspace,
			WorkflowHint:     t.StatusWorkflowHint,
		}
	}
	return result
}
