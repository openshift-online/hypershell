// Package policy implements the sandbox resource request policy: fill
// missing CPU and memory requests at 250m / 1Gi and deny requests below that
// floor. The policy never sets or checks limits, never denies on size (no
// ceiling), and has no audit mode: the only switch is off / enforce.
//
// Governed paths in the CreateSandbox proposed operation (ProtoJSON,
// camelCase field names):
//
//   - spec.template.resources.requests.{cpu,memory} - the Kubernetes-style
//     workload container resources.
//   - spec.template.driverConfig.kubernetes.containers.agent.resources.requests.{cpu,memory}
//   - the caller driver JSON for the agent container on operator-workspace
//     gateways (the same requests/limits maps the kubernetes driver
//     applies).
//
// Sandboxes created from a workload template (workloadTemplate set) resolve
// their resources from the stored template and discard the inline
// spec.template, so they stay ungoverned until upstream makes template
// resources interceptable (NVIDIA/OpenShell#5537).
package policy

import (
	"fmt"
	"strconv"

	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/api/resource"

	pb "github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/pkg/api/grpc"
)

// Mode is the interceptor enforcement mode.
type Mode string

const (
	// ModeOff allows every operation unchanged: no patches, no denials.
	ModeOff Mode = "off"
	// ModeEnforce fills missing requests at the floor and denies requests
	// below it.
	ModeEnforce Mode = "enforce"
)

// Settled policy values (plan decision set, Nov 14): the default and the
// floor are the same numbers. Requests at or above the floor pass; requests
// strictly below it are denied. Limits are never set or checked.
const (
	// CPUFloor is the CPU request floor, also used to fill missing CPU
	// requests.
	CPUFloor = "250m"
	// MemoryFloor is the memory request floor, also used to fill missing
	// memory requests.
	MemoryFloor = "1Gi"
)

// StatusCode is the gRPC status code name returned with denials. The gateway
// maps it onto the caller-facing denial status (FAILED_PRECONDITION: the
// request itself must change, not the caller's permissions).
const StatusCode = "FAILED_PRECONDITION"

// Outcome labels the evaluation outcome for metrics.
type Outcome string

const (
	OutcomeAllowed Outcome = "allowed"
	OutcomePatched Outcome = "patched"
	OutcomeDenied  Outcome = "denied"
)

// DenialReason is the machine-readable denial reason used as a metrics label.
type DenialReason string

const (
	DenialCPUBelowFloor         DenialReason = "cpu_below_floor"
	DenialMemoryBelowFloor      DenialReason = "memory_below_floor"
	DenialCPUInvalidQuantity    DenialReason = "cpu_invalid_quantity"
	DenialMemoryInvalidQuantity DenialReason = "memory_invalid_quantity"
	DenialResourcesNotObject    DenialReason = "resources_not_object"
	DenialRequestsNotObject     DenialReason = "requests_not_object"
)

// governedResource pairs a resource name with its denial reasons.
type governedResource struct {
	name         string
	belowFloor   DenialReason
	invalidValue DenialReason
}

// pathSpec describes one governed resources object in the proposed
// operation.
type pathSpec struct {
	// pointer is the RFC 6902 JSON pointer to the resources object,
	// e.g. "/spec/template/resources".
	pointer string
	// label is the human-readable location used in denial reasons,
	// e.g. "spec.template.resources".
	label string
	// chain lists the object segments above the resources object that must
	// all exist as objects for this path to be governed. When any link is
	// absent or not an object the path is skipped: the gateway's typed
	// validation rejects a spec-less request on its own, and the
	// driver-config envelope belongs to the caller and the owning driver.
	chain []string
	// create names one segment after the chain that is created when absent,
	// so requests are filled even when the caller omitted the whole level.
	// The gateway inserts a default template for template-less creates, so
	// those sandboxes run with the patched requests.
	create *levelSpec
}

// levelSpec describes an object level the policy may create.
type levelSpec struct {
	segment string
	pointer string
	label   string
	value   func(p Policy) *structpb.Value
}

// governedPaths are the two resource paths named by the plan, in evaluation
// order.
var governedPaths = []pathSpec{
	{
		pointer: "/spec/template/resources",
		label:   "spec.template.resources",
		chain:   []string{"spec"},
		create: &levelSpec{
			segment: "template",
			pointer: "/spec/template",
			label:   "spec.template",
			value: func(p Policy) *structpb.Value {
				return structpb.NewStructValue(&structpb.Struct{Fields: map[string]*structpb.Value{
					"resources": structpb.NewStructValue(&structpb.Struct{Fields: map[string]*structpb.Value{
						"requests": requestsObject(p),
					}}),
				}})
			},
		},
	},
	{
		pointer: "/spec/template/driverConfig/kubernetes/containers/agent/resources",
		label:   "spec.template.driverConfig.kubernetes.containers.agent.resources",
		chain:   []string{"spec", "template", "driverConfig", "kubernetes", "containers", "agent"},
	},
}

// governedResources are the request keys the policy fills and floors.
var governedResources = []governedResource{
	{name: "cpu", belowFloor: DenialCPUBelowFloor, invalidValue: DenialCPUInvalidQuantity},
	{name: "memory", belowFloor: DenialMemoryBelowFloor, invalidValue: DenialMemoryInvalidQuantity},
}

// Policy is the resolved request policy.
type Policy struct {
	Mode        Mode
	cpuFloor    resource.Quantity
	memoryFloor resource.Quantity
}

// New parses the floor quantities and returns a Policy in the given mode.
func New(mode Mode) (Policy, error) {
	cpu, err := resource.ParseQuantity(CPUFloor)
	if err != nil {
		return Policy{}, fmt.Errorf("parsing CPU floor %q: %w", CPUFloor, err)
	}
	memory, err := resource.ParseQuantity(MemoryFloor)
	if err != nil {
		return Policy{}, fmt.Errorf("parsing memory floor %q: %w", MemoryFloor, err)
	}
	return Policy{Mode: mode, cpuFloor: cpu, memoryFloor: memory}, nil
}

// MustNew is New for callers with compile-time-valid modes and floors.
func MustNew(mode Mode) Policy {
	pol, err := New(mode)
	if err != nil {
		panic(fmt.Sprintf("policy: %v", err))
	}
	return pol
}

// floorFor returns the floor quantity for a governed resource name.
func (p Policy) floorFor(name string) resource.Quantity {
	if name == "memory" {
		return p.memoryFloor
	}
	return p.cpuFloor
}

// Decision is the outcome of evaluating one proposed operation.
type Decision struct {
	// Allowed is false when the request is denied.
	Allowed bool
	// Reason is the human-readable denial reason, empty when allowed.
	Reason string
	// StatusCode is the gRPC status code name for denials.
	StatusCode string
	// Patches are the RFC 6902 add patches filling missing requests; empty
	// when denied or unchanged.
	Patches []*pb.JsonPatch
	// Outcome labels the decision for metrics.
	Outcome Outcome
	// DenialReason is the machine-readable denial reason for metrics; empty
	// unless Outcome is OutcomeDenied.
	DenialReason DenialReason
}

// Evaluate applies the policy to one CreateSandbox modify_operation payload.
// A nil proposed operation is treated as empty.
func (p Policy) Evaluate(proposed *structpb.Struct) Decision {
	if p.Mode == ModeOff {
		return Decision{Allowed: true, Outcome: OutcomeAllowed}
	}
	root := proposed.GetFields()

	// Sandboxes created from a stored workload template resolve their
	// resources from the template; the gateway handler discards the inline
	// spec.template except for policy, providers, command, tty and restart
	// policy. Governing the discarded inline requests would deny sandboxes
	// that never run at those values, so template-created sandboxes pass
	// ungoverned (upstream NVIDIA/OpenShell#5537 tracks the real fix).
	if isSet(root["workloadTemplate"]) {
		return Decision{Allowed: true, Outcome: OutcomeAllowed}
	}

	decision := Decision{Allowed: true, Outcome: OutcomeAllowed}
	for _, spec := range governedPaths {
		p.governPath(&decision, root, spec)
		if !decision.Allowed {
			return decision
		}
	}
	if len(decision.Patches) > 0 {
		decision.Outcome = OutcomePatched
	}
	return decision
}

// governPath fills and floors the requests of one governed resources object,
// stopping at the first denial.
func (p Policy) governPath(decision *Decision, root map[string]*structpb.Value, spec pathSpec) {
	// Walk the parent chain; a broken chain means the caller set nothing on
	// this path for the gateway or driver to consume, so there is nothing to
	// govern.
	parent := root
	for _, segment := range spec.chain {
		next, ok := asObject(parent[segment])
		if !ok {
			return
		}
		parent = next
	}

	if spec.create != nil {
		value, present := parent[spec.create.segment]
		if !present {
			// The whole level is missing: create it carrying the full
			// requests map. The gateway fills in the rest (for the main
			// path, the default template image).
			decision.Patches = append(decision.Patches,
				addPatch(spec.create.pointer, spec.create.value(p)))
			return
		}
		next, ok := asObject(value)
		if !ok {
			return
		}
		parent = next
	}

	resourcesValue, present := parent["resources"]
	if !present {
		// No resources object at all: add the deepest missing level with
		// the full requests map (RFC 6902 add needs the parent, which the
		// chain walk established).
		decision.Patches = append(decision.Patches,
			addPatch(spec.pointer, requestsObject(p)))
		return
	}
	resources, ok := asObject(resourcesValue)
	if !ok {
		p.deny(decision, DenialResourcesNotObject, "%s must be an object holding requests and limits maps", spec.label)
		return
	}

	requestsValue, present := resources["requests"]
	if !present {
		decision.Patches = append(decision.Patches,
			addPatch(spec.pointer+"/requests", requestsObject(p)))
		return
	}
	requests, ok := asObject(requestsValue)
	if !ok {
		p.deny(decision, DenialRequestsNotObject, "%s.requests must be an object of resource quantities", spec.label)
		return
	}

	for _, gr := range governedResources {
		value, present := requests[gr.name]
		if !present {
			// Missing request: patch in the floor.
			decision.Patches = append(decision.Patches,
				addPatch(spec.pointer+"/requests/"+gr.name, structpb.NewStringValue(p.floorString(gr.name))))
			continue
		}
		if _, isNull := value.GetKind().(*structpb.Value_NullValue); isNull {
			// A JSON null request means no request: patch in the floor. RFC
			// 6902 add on an existing member replaces it, so the same patch
			// covers the null case.
			decision.Patches = append(decision.Patches,
				addPatch(spec.pointer+"/requests/"+gr.name, structpb.NewStringValue(p.floorString(gr.name))))
			continue
		}
		quantity, original, err := parseQuantity(value)
		if err != nil {
			p.deny(decision, gr.invalidValue, "%s.requests.%s: %v", spec.label, gr.name, err)
			return
		}
		if quantity.Cmp(p.floorFor(gr.name)) < 0 {
			p.deny(decision, gr.belowFloor, "%s.requests.%s: %s request %s is below the %s floor",
				spec.label, gr.name, gr.name, original, p.floorString(gr.name))
			return
		}
		// At or above the floor: pass through untouched. Large requests,
		// limits and non-governed request keys (GPU devices, ephemeral
		// storage) are never modified or denied.
	}
}

// deny records a denial on the decision.
func (p Policy) deny(decision *Decision, reason DenialReason, format string, args ...any) {
	decision.Allowed = false
	decision.Reason = fmt.Sprintf(format, args...)
	decision.StatusCode = StatusCode
	decision.Outcome = OutcomeDenied
	decision.DenialReason = reason
	decision.Patches = nil
}

// floorString returns the canonical floor quantity string for a resource.
func (p Policy) floorString(name string) string {
	if name == "memory" {
		return MemoryFloor
	}
	return CPUFloor
}

// requestsObject builds the full requests map value used when the requests
// object itself is added.
func requestsObject(p Policy) *structpb.Value {
	fields := map[string]*structpb.Value{}
	for _, gr := range governedResources {
		fields[gr.name] = structpb.NewStringValue(p.floorString(gr.name))
	}
	return structpb.NewStructValue(&structpb.Struct{Fields: fields})
}

// addPatch builds an RFC 6902 add patch.
func addPatch(pointer string, value *structpb.Value) *pb.JsonPatch {
	return &pb.JsonPatch{Op: "add", Path: pointer, Value: value}
}

// asObject returns the value's object fields when it is an object.
func asObject(value *structpb.Value) (map[string]*structpb.Value, bool) {
	if value == nil {
		return nil, false
	}
	obj := value.GetStructValue()
	if obj == nil {
		return nil, false
	}
	return obj.GetFields(), true
}

// isSet reports whether a scalar JSON value counts as present-and-meaningful
// for the workload-template check.
func isSet(value *structpb.Value) bool {
	if value == nil {
		return false
	}
	switch kind := value.GetKind().(type) {
	case *structpb.Value_NullValue:
		return false
	case *structpb.Value_StringValue:
		return kind.StringValue != ""
	default:
		return true
	}
}

// parseQuantity parses a JSON request value as a Kubernetes quantity.
// Kubernetes accepts quantities as strings ("250m") and bare JSON numbers
// (0.25), so both are supported; anything else is not a quantity.
func parseQuantity(value *structpb.Value) (resource.Quantity, string, error) {
	switch kind := value.GetKind().(type) {
	case *structpb.Value_StringValue:
		quantity, err := resource.ParseQuantity(kind.StringValue)
		if err != nil {
			return resource.Quantity{}, kind.StringValue, fmt.Errorf("%q is not a valid resource quantity", kind.StringValue)
		}
		return quantity, kind.StringValue, nil
	case *structpb.Value_NumberValue:
		original := strconv.FormatFloat(kind.NumberValue, 'f', -1, 64)
		quantity, err := resource.ParseQuantity(original)
		if err != nil {
			return resource.Quantity{}, original, fmt.Errorf("%q is not a valid resource quantity", original)
		}
		return quantity, original, nil
	default:
		return resource.Quantity{}, "", fmt.Errorf("request value must be a quantity string or number, got %s", valueKindName(value))
	}
}

// valueKindName names a structpb value kind for denial reasons.
func valueKindName(value *structpb.Value) string {
	switch value.GetKind().(type) {
	case *structpb.Value_BoolValue:
		return "a boolean"
	case *structpb.Value_StructValue:
		return "an object"
	case *structpb.Value_ListValue:
		return "an array"
	default:
		return "an unrecognised value"
	}
}
