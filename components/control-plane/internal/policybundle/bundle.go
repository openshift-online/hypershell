// Package policybundle is the versioned contract between the control plane and
// the policy interceptor sidecar (PLAT-PGP-12, CTRL-CORE-22): the per-gateway
// bundle of resolved platform gateway policies that the controller writes to the
// hypershell-policy-bundle ConfigMap and the sidecar watches.
package policybundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
)

// Contract constants. A change to the bundle shape requires a new SchemaVersion.
const (
	SchemaVersion      = "v1"
	ConfigMapName      = "hypershell-policy-bundle"
	DataKey            = "bundle.json"
	LabelGatewayID     = "hypershell.io/gateway-id"
	LabelSchemaVersion = "hypershell.io/policy-bundle-schema"
	LabelManagedBy     = "app.kubernetes.io/managed-by"
	ManagedByValue     = "hypershell-controller"

	ModeAudit   = "audit"
	ModeEnforce = "enforce"

	// CanaryPolicyName is the built-in e2e enforcement canary (PLAT-PGP-20).
	CanaryPolicyName = "e2e-enforcement-canary"
)

// Policy is one resolved policy in a bundle. Cedar carries the @id-annotated
// statements (PLAT-PGP-14).
type Policy struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	UserMessage      string `json:"userMessage,omitempty"`
	DocumentationURL string `json:"documentationUrl,omitempty"`
	Cedar            string `json:"cedar"`
}

// Bundle is the per-gateway policy bundle. Effective modes are already
// resolved (PLAT-PGP-06): Enforced holds the policies whose forbids reject,
// AuditOnly the policies whose forbids only record findings.
type Bundle struct {
	SchemaVersion             string   `json:"schemaVersion"`
	CedarSchemaVersion        string   `json:"cedarSchemaVersion"`
	GatewayID                 string   `json:"gatewayId"`
	Revision                  string   `json:"revision"`
	ResourcePoliciesSupported bool     `json:"resourcePoliciesSupported"`
	Enforced                  []Policy `json:"enforced"`
	AuditOnly                 []Policy `json:"auditOnly"`
}

// Skipped records a policy that targets the gateway but is not published, with
// the reason (for example a re-validation failure, PLAT-PGP-04).
type Skipped struct {
	PolicyID string
	Name     string
	Reason   string
}

// Input is everything Build needs for one gateway.
type Input struct {
	GatewayID  string
	GlobalMode string
	Policies   []*pb.PlatformGatewayPolicy
	Defaults   cedar.Defaults
	// Unsupported is true when the gateway's driver cannot honor resource
	// values (PLAT-PGP-18): every policy is published audit-only.
	Unsupported bool
	// CanaryEnabled is false when the controller setting disables the built-in
	// canary in this environment (PLAT-PGP-20).
	CanaryEnabled bool
}

// Targets reports whether policy p applies to gatewayID (PLAT-PGP-01).
func Targets(p *pb.PlatformGatewayPolicy, gatewayID string) bool {
	t := p.GetTarget()
	if t == nil {
		return true
	}
	for _, ex := range t.GetExclude() {
		if ex == gatewayID {
			return false
		}
	}
	if t.GetAll() {
		return true
	}
	for _, id := range t.GetGatewayIds() {
		if id == gatewayID {
			return true
		}
	}
	return false
}

// EffectiveMode is the stricter of the global toggle and the policy mode
// (PLAT-PGP-06).
func EffectiveMode(globalMode, policyMode string) string {
	if globalMode == ModeEnforce || policyMode == ModeEnforce {
		return ModeEnforce
	}
	return ModeAudit
}

// Build resolves the bundle for one gateway. Policies are ordered by name so
// the output, and therefore the revision, is deterministic (STD-RCON-02).
func Build(in Input) (Bundle, []Skipped, error) {
	if in.GatewayID == "" {
		return Bundle{}, nil, errors.New("gateway id is required")
	}
	policies := append([]*pb.PlatformGatewayPolicy{}, in.Policies...)
	sort.Slice(policies, func(i, j int) bool { return policies[i].GetName() < policies[j].GetName() })

	b := Bundle{
		SchemaVersion:             SchemaVersion,
		CedarSchemaVersion:        cedar.SchemaVersion,
		GatewayID:                 in.GatewayID,
		ResourcePoliciesSupported: !in.Unsupported,
		Enforced:                  []Policy{},
		AuditOnly:                 []Policy{},
	}
	var skipped []Skipped
	for _, p := range policies {
		if !Targets(p, in.GatewayID) {
			continue
		}
		if p.GetName() == CanaryPolicyName && p.GetBuiltIn() && !in.CanaryEnabled {
			skipped = append(skipped, Skipped{PolicyID: p.GetMetadata().GetId(), Name: p.GetName(), Reason: "disabled by the controller setting in this environment"})
			continue
		}
		// The control plane re-validates before it publishes (PLAT-PGP-04).
		if err := cedar.ValidatePolicy(p.GetCedar(), in.Defaults); err != nil {
			skipped = append(skipped, Skipped{PolicyID: p.GetMetadata().GetId(), Name: p.GetName(), Reason: err.Error()})
			continue
		}
		annotated, err := cedar.Annotate(p.GetCedar(), p.GetName())
		if err != nil {
			skipped = append(skipped, Skipped{PolicyID: p.GetMetadata().GetId(), Name: p.GetName(), Reason: err.Error()})
			continue
		}
		entry := Policy{
			ID:               p.GetMetadata().GetId(),
			Name:             p.GetName(),
			UserMessage:      p.GetUserMessage(),
			DocumentationURL: p.GetDocumentationUrl(),
			Cedar:            annotated,
		}
		if !in.Unsupported && EffectiveMode(in.GlobalMode, p.GetEnforcementMode()) == ModeEnforce {
			b.Enforced = append(b.Enforced, entry)
		} else {
			b.AuditOnly = append(b.AuditOnly, entry)
		}
	}
	rev, err := revision(b)
	if err != nil {
		return Bundle{}, nil, err
	}
	b.Revision = rev
	return b, skipped, nil
}

// revision hashes the resolved content (everything but the revision itself).
func revision(b Bundle) (string, error) {
	b.Revision = ""
	raw, err := json.Marshal(b)
	if err != nil {
		return "", fmt.Errorf("encode bundle for revision: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16], nil
}

// Encode returns the deterministic JSON form stored in the ConfigMap.
func (b Bundle) Encode() ([]byte, error) {
	return json.MarshalIndent(b, "", "  ")
}

// Decode parses and checks a bundle. A schema version the reader does not
// understand is an error, never silently evaluated (PLAT-PGP-03.02).
func Decode(raw []byte) (Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return Bundle{}, fmt.Errorf("decode policy bundle: %w", err)
	}
	if b.SchemaVersion != SchemaVersion {
		return Bundle{}, fmt.Errorf("policy bundle schema version %q is not supported (want %q)", b.SchemaVersion, SchemaVersion)
	}
	if b.CedarSchemaVersion != cedar.SchemaVersion {
		return Bundle{}, fmt.Errorf("policy bundle Cedar schema version %q does not match this build (%q)", b.CedarSchemaVersion, cedar.SchemaVersion)
	}
	want, err := revision(b)
	if err != nil {
		return Bundle{}, err
	}
	if b.Revision != want {
		return Bundle{}, fmt.Errorf("policy bundle revision %q does not match its content", b.Revision)
	}
	return b, nil
}

func inputs(policies []Policy) []cedar.PolicyInput {
	out := make([]cedar.PolicyInput, 0, len(policies))
	for _, p := range policies {
		out = append(out, cedar.PolicyInput{Name: p.Name, Cedar: p.Cedar})
	}
	return out
}

// Evaluators compiles the enforced and audit-only sets.
func (b Bundle) Evaluators() (enforced, audit *cedar.Evaluator, err error) {
	if enforced, err = cedar.NewEvaluator(inputs(b.Enforced)); err != nil {
		return nil, nil, fmt.Errorf("compile enforced set: %w", err)
	}
	if audit, err = cedar.NewEvaluator(inputs(b.AuditOnly)); err != nil {
		return nil, nil, fmt.Errorf("compile audit-only set: %w", err)
	}
	return enforced, audit, nil
}

// Messages returns the user-facing metadata by policy name for denial text.
func (b Bundle) Messages() map[string]cedar.PolicyMessage {
	out := map[string]cedar.PolicyMessage{}
	for _, p := range append(append([]Policy{}, b.Enforced...), b.AuditOnly...) {
		out[p.Name] = cedar.PolicyMessage{UserMessage: p.UserMessage, DocumentationURL: p.DocumentationURL}
	}
	return out
}

// Contains reports whether a policy name is in the bundle and in which set.
func (b Bundle) Contains(name string) (present bool, enforced bool) {
	for _, p := range b.Enforced {
		if p.Name == name {
			return true, true
		}
	}
	for _, p := range b.AuditOnly {
		if p.Name == name {
			return true, false
		}
	}
	return false, false
}
