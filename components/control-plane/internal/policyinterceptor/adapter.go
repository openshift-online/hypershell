package policyinterceptor

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	gi "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/gatewayinterceptorv1"
)

// The Kubernetes driver adapter (PLAT-PGP-03, PLAT-PGP-18) maps the protobuf
// JSON of an OpenShell CreateSandboxRequest to the driver-neutral Cedar context.
// It reads the same inputs the OpenShell Kubernetes driver uses to build the
// agent container resources:
//
//   - spec.template.resources.{requests,limits} (a free-form Struct carrying
//     Kubernetes resource names and quantity strings);
//   - spec.template.driverConfig.kubernetes.containers.agent.resources.
//     {requests,limits}, which the driver merges without overriding values
//     already set by the template;
//   - spec.resourceRequirements.gpu.count (one GPU when gpu is present without a
//     count), falling back to an nvidia.com/gpu quantity;
//   - spec.template.image and the request name.
//
// When only a limit is set, the driver uses the limit as the request, so the
// effective request is the limit.

const gpuResourceName = "nvidia.com/gpu"

// resourceSet is one requests/limits pair as raw quantity strings ("" = unset).
type resourceSet struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
	GPURequest    string
	GPULimit      string
}

// SandboxRequest is what the adapter reads from a CreateSandbox operation.
type SandboxRequest struct {
	Name      string
	Image     string
	RequestID string
	// Template is spec.template.resources.
	Template resourceSet
	// Driver is the Kubernetes driver_config agent container resources.
	Driver resourceSet
	// GPURequirement is set when spec.resourceRequirements.gpu is present.
	GPURequirement bool
	GPUCount       int64

	// presence of the containers the default patches add to.
	hasSpec, hasTemplate, hasResources, hasRequests, hasLimits bool
}

// ParseCreateSandbox reads a CreateSandboxRequest in protobuf JSON form.
func ParseCreateSandbox(op map[string]any) (SandboxRequest, error) {
	var r SandboxRequest
	r.Name = str(op["name"])
	r.RequestID = str(field(op, "requestId", "request_id"))

	spec, ok := op["spec"].(map[string]any)
	if !ok {
		return r, nil
	}
	r.hasSpec = true
	if reqs, ok := field(spec, "resourceRequirements", "resource_requirements").(map[string]any); ok {
		if gpu, ok := reqs["gpu"].(map[string]any); ok {
			r.GPURequirement = true
			r.GPUCount = 1
			if c, present := gpu["count"]; present && c != nil {
				n, err := parseCount(c)
				if err != nil {
					return r, fmt.Errorf("spec.resourceRequirements.gpu.count: %w", err)
				}
				r.GPUCount = n
			}
		}
	}
	tmpl, ok := spec["template"].(map[string]any)
	if !ok {
		return r, nil
	}
	r.hasTemplate = true
	r.Image = str(tmpl["image"])
	if res, ok := tmpl["resources"].(map[string]any); ok {
		r.hasResources = true
		_, r.hasRequests = res["requests"].(map[string]any)
		_, r.hasLimits = res["limits"].(map[string]any)
		r.Template = readResourceSet(res)
	}
	if dc, ok := field(tmpl, "driverConfig", "driver_config").(map[string]any); ok {
		if k8s, ok := dc["kubernetes"].(map[string]any); ok {
			if containers, ok := k8s["containers"].(map[string]any); ok {
				if agent, ok := containers["agent"].(map[string]any); ok {
					if res, ok := agent["resources"].(map[string]any); ok {
						r.Driver = readResourceSet(res)
					}
				}
			}
		}
	}
	return r, nil
}

func readResourceSet(res map[string]any) resourceSet {
	var s resourceSet
	if req, ok := res["requests"].(map[string]any); ok {
		s.CPURequest = quantity(req["cpu"])
		s.MemoryRequest = quantity(req["memory"])
		s.GPURequest = quantity(req[gpuResourceName])
	}
	if lim, ok := res["limits"].(map[string]any); ok {
		s.CPULimit = quantity(lim["cpu"])
		s.MemoryLimit = quantity(lim["memory"])
		s.GPULimit = quantity(lim[gpuResourceName])
	}
	return s
}

// defaultAddition is one default the interceptor would add to
// spec.template.resources.
type defaultAddition struct {
	Section string // "limits" or "requests"
	Key     string // "cpu" or "memory"
	Value   string
}

// completed is the request after fleet defaults are applied.
type completed struct {
	cpuRequest, cpuLimit, memRequest, memLimit string
	added                                      []defaultAddition
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// complete applies the effective-request rule and fills missing values from the
// fleet defaults. A missing limit takes the default limit unless the effective
// request exceeds it, in which case it takes the request (the smallest valid
// limit), so a default never produces a request greater than its limit. Additions
// are ordered limits then requests, cpu then memory, so patches are
// deterministic (PLAT-PGP-11).
func (r SandboxRequest) complete(d DefaultQuantities) (completed, error) {
	var c completed
	var err error
	c.cpuRequest, c.cpuLimit, err = completePair("CPU",
		firstNonEmpty(r.Template.CPURequest, r.Template.CPULimit, r.Driver.CPURequest, r.Driver.CPULimit),
		firstNonEmpty(r.Template.CPULimit, r.Driver.CPULimit),
		d.CPURequest, d.CPULimit, cedar.ParseMillis, "cpu", &c.added)
	if err != nil {
		return completed{}, err
	}
	c.memRequest, c.memLimit, err = completePair("memory",
		firstNonEmpty(r.Template.MemoryRequest, r.Template.MemoryLimit, r.Driver.MemoryRequest, r.Driver.MemoryLimit),
		firstNonEmpty(r.Template.MemoryLimit, r.Driver.MemoryLimit),
		d.MemoryRequest, d.MemoryLimit, cedar.ParseBytes, "memory", &c.added)
	if err != nil {
		return completed{}, err
	}
	// Order: limits first, then requests; cpu before memory within a section.
	ordered := make([]defaultAddition, 0, len(c.added))
	for _, section := range []string{"limits", "requests"} {
		for _, key := range []string{"cpu", "memory"} {
			for _, a := range c.added {
				if a.Section == section && a.Key == key {
					ordered = append(ordered, a)
				}
			}
		}
	}
	c.added = ordered
	return c, nil
}

func completePair(label, request, limit, defRequest, defLimit string, parse func(string) (int64, error), key string, added *[]defaultAddition) (string, string, error) {
	if request == "" {
		request = defRequest
		*added = append(*added, defaultAddition{Section: "requests", Key: key, Value: defRequest})
	}
	if limit == "" {
		limit = defLimit
		reqN, err := parse(request)
		if err != nil {
			return "", "", fmt.Errorf("invalid %s request %q: %w", label, request, err)
		}
		limN, err := parse(defLimit)
		if err != nil {
			return "", "", fmt.Errorf("invalid default %s limit %q: %w", label, defLimit, err)
		}
		if reqN > limN {
			limit = request
		}
		*added = append(*added, defaultAddition{Section: "limits", Key: key, Value: limit})
	}
	return request, limit, nil
}

// gpuCount is the effective GPU count.
func (r SandboxRequest) gpuCount() (int64, error) {
	if r.GPURequirement {
		return r.GPUCount, nil
	}
	q := firstNonEmpty(r.Template.GPULimit, r.Driver.GPULimit, r.Template.GPURequest, r.Driver.GPURequest)
	if q == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(q, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid %s quantity %q", gpuResourceName, q)
	}
	return n, nil
}

// Context builds the Cedar context after fleet defaults are applied, with
// quantities normalized to millicores and bytes (PLAT-PGP-03).
func (r SandboxRequest) Context(d DefaultQuantities) (cedar.Context, error) {
	c, err := r.complete(d)
	if err != nil {
		return cedar.Context{}, err
	}
	var out cedar.Context
	if out.CPURequestMillis, err = cedar.ParseMillis(c.cpuRequest); err != nil {
		return cedar.Context{}, fmt.Errorf("invalid CPU request %q: %w", c.cpuRequest, err)
	}
	if out.CPULimitMillis, err = cedar.ParseMillis(c.cpuLimit); err != nil {
		return cedar.Context{}, fmt.Errorf("invalid CPU limit %q: %w", c.cpuLimit, err)
	}
	if out.MemoryRequestBytes, err = cedar.ParseBytes(c.memRequest); err != nil {
		return cedar.Context{}, fmt.Errorf("invalid memory request %q: %w", c.memRequest, err)
	}
	if out.MemoryLimitBytes, err = cedar.ParseBytes(c.memLimit); err != nil {
		return cedar.Context{}, fmt.Errorf("invalid memory limit %q: %w", c.memLimit, err)
	}
	if out.GPUCount, err = r.gpuCount(); err != nil {
		return cedar.Context{}, err
	}
	out.Image = r.Image
	out.SandboxName = r.Name
	return out, nil
}

// MissingDefaults returns the defaults modify_operation would add.
func (r SandboxRequest) MissingDefaults(d DefaultQuantities) ([]defaultAddition, error) {
	c, err := r.complete(d)
	if err != nil {
		return nil, err
	}
	return c.added, nil
}

// DefaultPatches returns RFC 6902 "add" patches that insert the missing
// defaults into spec.template.resources. Missing parents are created as empty
// objects first, so every patch value is a scalar string or an empty object and
// the same input always yields byte-identical patches (PLAT-PGP-11.02).
func (r SandboxRequest) DefaultPatches(d DefaultQuantities) ([]*gi.JsonPatch, error) {
	added, err := r.MissingDefaults(d)
	if err != nil || len(added) == 0 {
		return nil, err
	}
	var patches []*gi.JsonPatch
	addObject := func(path string) {
		patches = append(patches, &gi.JsonPatch{Op: "add", Path: path, Value: structpb.NewStructValue(&structpb.Struct{})})
	}
	if !r.hasSpec {
		addObject("/spec")
	}
	if !r.hasTemplate {
		addObject("/spec/template")
	}
	hasSection := map[string]bool{"requests": r.hasRequests, "limits": r.hasLimits}
	if !r.hasResources {
		addObject("/spec/template/resources")
		hasSection = map[string]bool{}
	}
	for _, a := range added {
		if !hasSection[a.Section] {
			addObject("/spec/template/resources/" + a.Section)
			hasSection[a.Section] = true
		}
		patches = append(patches, &gi.JsonPatch{
			Op:    "add",
			Path:  "/spec/template/resources/" + a.Section + "/" + a.Key,
			Value: structpb.NewStringValue(a.Value),
		})
	}
	return patches, nil
}

func field(m map[string]any, names ...string) any {
	for _, n := range names {
		if v, ok := m[n]; ok {
			return v
		}
	}
	return nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// quantity reads a quantity that a client may have sent as a string or a JSON
// number.
func quantity(v any) string {
	switch q := v.(type) {
	case string:
		return strings.TrimSpace(q)
	case float64:
		return strconv.FormatFloat(q, 'f', -1, 64)
	default:
		return ""
	}
}

func parseCount(v any) (int64, error) {
	switch c := v.(type) {
	case float64:
		if c < 0 || c != float64(int64(c)) {
			return 0, fmt.Errorf("invalid count %v", c)
		}
		return int64(c), nil
	case string:
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid count %q", c)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("invalid count type %T", v)
	}
}
