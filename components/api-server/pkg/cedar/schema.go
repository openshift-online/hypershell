package cedar

import (
	"fmt"
	"sync"

	"github.com/cedar-policy/cedar-go/x/exp/schema"
	"github.com/cedar-policy/cedar-go/x/exp/schema/resolved"
)

// SchemaVersion is the version of the HyperShell Cedar schema. It is carried in
// the policy bundle (PLAT-PGP-12); a change to the OpenShell request shape
// requires a new schema version and a documented compatibility note
// (PLAT-PGP-03).
const SchemaVersion = "v1"

// Cedar entity and action identifiers modeled by the schema (PLAT-PGP-03).
const (
	EntityTypeUser    = "OpenShell::User"
	EntityTypeGateway = "OpenShell::Gateway"
	ActionEntityType  = "OpenShell::Action"

	// ActionCreateSandbox is the only action modeled in v1. Later versions add
	// UpdateSandboxPolicy and the other interceptable write actions.
	ActionCreateSandbox = "CreateSandbox"
)

// Context attribute names. The driver adapter populates these after fleet
// defaults are applied; the units are integers because Cedar has no quantity
// type (PLAT-PGP-03).
const (
	CtxCPURequestMillis   = "cpuRequestMillis"
	CtxCPULimitMillis     = "cpuLimitMillis"
	CtxMemoryRequestBytes = "memoryRequestBytes"
	CtxMemoryLimitBytes   = "memoryLimitBytes"
	CtxGPUCount           = "gpuCount"
	CtxImage              = "image"
	CtxSandboxName        = "sandboxName"
)

// schemaCedar is the driver-neutral HyperShell Cedar schema for v1 (PLAT-PGP-03).
// It is intentionally the human-readable Cedar schema format so the text is the
// single source of truth that both validation and evaluation load.
const schemaCedar = `namespace OpenShell {
  entity User {
    subject: String,
    roles: Set<String>,
  };
  entity Gateway {
    id: String,
    name: String,
  };
  action "CreateSandbox" appliesTo {
    principal: [User],
    resource: [Gateway],
    context: {
      cpuRequestMillis: Long,
      cpuLimitMillis: Long,
      memoryRequestBytes: Long,
      memoryLimitBytes: Long,
      gpuCount: Long,
      image: String,
      sandboxName: String,
    }
  };
}`

// SchemaCedar returns the human-readable Cedar schema text for the current
// schema version.
func SchemaCedar() string { return schemaCedar }

var (
	schemaOnce sync.Once
	schemaVal  *resolved.Schema
	schemaErr  error
)

// resolvedSchema parses and resolves the embedded Cedar schema once and caches
// the result. A parse or resolve failure is a programming error in the embedded
// schema text and is surfaced to every caller.
func resolvedSchema() (*resolved.Schema, error) {
	schemaOnce.Do(func() {
		var s schema.Schema
		s.SetFilename("hypershell." + SchemaVersion + ".cedarschema")
		if err := s.UnmarshalCedar([]byte(schemaCedar)); err != nil {
			schemaErr = fmt.Errorf("parse HyperShell cedar schema: %w", err)
			return
		}
		rs, err := s.Resolve()
		if err != nil {
			schemaErr = fmt.Errorf("resolve HyperShell cedar schema: %w", err)
			return
		}
		schemaVal = rs
	})
	return schemaVal, schemaErr
}
