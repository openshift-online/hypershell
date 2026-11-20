package cedar

import (
	"fmt"

	"github.com/cedar-policy/cedar-go"
	"github.com/cedar-policy/cedar-go/types"
)

// Defaults are the fleet-wide sandbox resource defaults, already normalized to
// the schema's integer units (PLAT-PGP-08). They are controller configuration,
// not policy data, and are used by the write-time default-request check
// (PLAT-PGP-04) and, later, by the interceptor's modify_operation.
type Defaults struct {
	CPURequestMillis   int64
	CPULimitMillis     int64
	MemoryRequestBytes int64
	MemoryLimitBytes   int64
	GPUCount           int64
}

// ParseDefaults normalizes fleet-default quantity strings (for example "500m",
// "2", "1Gi", "4Gi") into Defaults. GPU has no default resource and is 0.
func ParseDefaults(cpuRequest, cpuLimit, memoryRequest, memoryLimit string) (Defaults, error) {
	var d Defaults
	var err error
	if d.CPURequestMillis, err = ParseMillis(cpuRequest); err != nil {
		return Defaults{}, fmt.Errorf("default CPU request: %w", err)
	}
	if d.CPULimitMillis, err = ParseMillis(cpuLimit); err != nil {
		return Defaults{}, fmt.Errorf("default CPU limit: %w", err)
	}
	if d.MemoryRequestBytes, err = ParseBytes(memoryRequest); err != nil {
		return Defaults{}, fmt.Errorf("default memory request: %w", err)
	}
	if d.MemoryLimitBytes, err = ParseBytes(memoryLimit); err != nil {
		return Defaults{}, fmt.Errorf("default memory limit: %w", err)
	}
	return d, nil
}

// context builds the Cedar context record for the default request: every fleet
// default applied and no user value set (empty image and sandbox name).
func (d Defaults) context() types.Record {
	return types.NewRecord(types.RecordMap{
		CtxCPURequestMillis:   types.Long(d.CPURequestMillis),
		CtxCPULimitMillis:     types.Long(d.CPULimitMillis),
		CtxMemoryRequestBytes: types.Long(d.MemoryRequestBytes),
		CtxMemoryLimitBytes:   types.Long(d.MemoryLimitBytes),
		CtxGPUCount:           types.Long(d.GPUCount),
		CtxImage:              types.String(""),
		CtxSandboxName:        types.String(""),
	})
}

// request builds the default Cedar request used by the default-request check
// (PLAT-PGP-04 rule 3): the request that results when every fleet default is
// applied and no user value is set.
func (d Defaults) request() cedar.Request {
	return cedar.Request{
		Principal: types.NewEntityUID(EntityTypeUser, "default"),
		Action:    types.NewEntityUID(ActionEntityType, ActionCreateSandbox),
		Resource:  types.NewEntityUID(EntityTypeGateway, "default"),
		Context:   d.context(),
	}
}
