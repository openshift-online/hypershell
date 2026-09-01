package gatewayProfiles

import (
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/openshift-online/rh-trex-ai/pkg/errors"
)

// validateQuantity validates a single optional Kubernetes resource quantity
// field. Empty/unset is valid ("not set"). A non-empty value must parse as a
// valid, non-negative quantity; otherwise a validation error is returned.
func validateQuantity(field string, value *string) *errors.ServiceError {
	if value == nil || *value == "" {
		return nil
	}
	q, err := resource.ParseQuantity(*value)
	if err != nil {
		return errors.Validation("invalid quantity for %s: %q is not a valid Kubernetes resource quantity", field, *value)
	}
	if q.Sign() < 0 {
		return errors.Validation("invalid quantity for %s: %q must not be negative", field, *value)
	}
	return nil
}

// validateCount validates an optional non-negative count field. Nil or zero is
// valid ("not set"); a negative value is rejected.
func validateCount(field string, value *int32) *errors.ServiceError {
	if value == nil {
		return nil
	}
	if *value < 0 {
		return errors.Validation("invalid count for %s: %d must not be negative", field, *value)
	}
	return nil
}

// validateRequestNotExceedsLimit checks that a request quantity does not exceed
// its corresponding limit when both are set. Skipped when either is unset.
func validateRequestNotExceedsLimit(requestField string, requestValue *string, limitField string, limitValue *string) *errors.ServiceError {
	if requestValue == nil || *requestValue == "" || limitValue == nil || *limitValue == "" {
		return nil
	}
	req, _ := resource.ParseQuantity(*requestValue)
	lim, _ := resource.ParseQuantity(*limitValue)
	if req.Cmp(lim) > 0 {
		return errors.Validation("%s (%s) must not exceed %s (%s)", requestField, *requestValue, limitField, *limitValue)
	}
	return nil
}

// validateProfileFields validates every quantity and count field on a
// GatewayProfile at the API boundary so invalid values are rejected with HTTP
// 400 rather than persisted and later failing control-plane reconciliation.
func validateProfileFields(p *GatewayProfile) *errors.ServiceError {
	quantities := []struct {
		field string
		value *string
	}{
		{"cpu_request_total", p.CpuRequestTotal},
		{"cpu_limit_total", p.CpuLimitTotal},
		{"memory_request_total", p.MemoryRequestTotal},
		{"memory_limit_total", p.MemoryLimitTotal},
		{"ephemeral_storage_total", p.EphemeralStorageTotal},
		{"container_cpu_request_default", p.ContainerCpuRequestDefault},
		{"container_cpu_limit_max", p.ContainerCpuLimitMax},
		{"container_memory_request_default", p.ContainerMemoryRequestDefault},
		{"container_memory_limit_max", p.ContainerMemoryLimitMax},
	}
	for _, q := range quantities {
		if svcErr := validateQuantity(q.field, q.value); svcErr != nil {
			return svcErr
		}
	}
	if svcErr := validateCount("pod_count", p.PodCount); svcErr != nil {
		return svcErr
	}
	if svcErr := validateCount("pvc_count", p.PvcCount); svcErr != nil {
		return svcErr
	}

	crossChecks := []struct {
		reqField string
		reqValue *string
		limField string
		limValue *string
	}{
		{"cpu_request_total", p.CpuRequestTotal, "cpu_limit_total", p.CpuLimitTotal},
		{"memory_request_total", p.MemoryRequestTotal, "memory_limit_total", p.MemoryLimitTotal},
		{"container_cpu_request_default", p.ContainerCpuRequestDefault, "container_cpu_limit_max", p.ContainerCpuLimitMax},
		{"container_memory_request_default", p.ContainerMemoryRequestDefault, "container_memory_limit_max", p.ContainerMemoryLimitMax},
	}
	for _, c := range crossChecks {
		if svcErr := validateRequestNotExceedsLimit(c.reqField, c.reqValue, c.limField, c.limValue); svcErr != nil {
			return svcErr
		}
	}
	return nil
}
