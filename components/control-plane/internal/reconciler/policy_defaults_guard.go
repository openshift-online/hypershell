package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
)

// PolicyDefaultsConfigMap records, in the controller namespace, the fleet
// defaults the controller last rolled out to the interceptor sidecars.
const PolicyDefaultsConfigMap = "hypershell-policy-applied-defaults"

const policyDefaultsKey = "defaults.json"

// FleetDefaults are the sandbox resource defaults as quantities (PLAT-PGP-08).
type FleetDefaults struct {
	CPURequest    string `json:"cpuRequest"`
	CPULimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

func (d FleetDefaults) normalized() (cedar.Defaults, error) {
	return cedar.ParseDefaults(d.CPURequest, d.CPULimit, d.MemoryRequest, d.MemoryLimit)
}

// PolicyDefaultsGuard decides which fleet defaults the sidecars receive. A
// configured defaults change is accepted only when no enforced policy forbids
// the resulting default request; otherwise the controller refuses it and keeps
// rolling out the previously accepted defaults (PLAT-PGP-08.02).
type PolicyDefaultsGuard struct {
	kube       kubernetes.Interface
	namespace  string
	configured FleetDefaults

	mu       sync.RWMutex
	accepted FleetDefaults
	loaded   bool
	refusal  string
}

// NewPolicyDefaultsGuard builds the guard. kube may be nil out of cluster, in
// which case the configured defaults are used and nothing is persisted.
func NewPolicyDefaultsGuard(kube kubernetes.Interface, namespace string, configured FleetDefaults) *PolicyDefaultsGuard {
	return &PolicyDefaultsGuard{kube: kube, namespace: namespace, configured: configured, accepted: configured}
}

// Accepted returns the defaults to pass to the sidecars.
func (g *PolicyDefaultsGuard) Accepted() FleetDefaults {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.accepted
}

// Refusal returns the reason the configured defaults are refused, or "".
func (g *PolicyDefaultsGuard) Refusal() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.refusal
}

// Check evaluates the configured defaults against every policy whose effective
// mode is enforce and updates the accepted defaults. It persists an accepted
// change so a controller restart keeps refusing a forbidden one.
func (g *PolicyDefaultsGuard) Check(ctx context.Context, policies []*pb.PlatformGatewayPolicy, globalMode string) error {
	if err := g.load(ctx); err != nil {
		return err
	}
	normalized, err := g.configured.normalized()
	if err != nil {
		return fmt.Errorf("configured fleet defaults: %w", err)
	}
	var enforced []cedar.PolicyInput
	for _, p := range policies {
		if policybundle.EffectiveMode(globalMode, p.GetEnforcementMode()) == policybundle.ModeEnforce {
			enforced = append(enforced, cedar.PolicyInput{Name: p.GetName(), Cedar: p.GetCedar()})
		}
	}
	forbiddenBy, err := forbiddingPolicies(enforced, normalized)
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(forbiddenBy) > 0 {
		g.refusal = fmt.Sprintf("fleet defaults change refused: enforced policies %v forbid the default request; keeping cpu request %s, cpu limit %s, memory request %s, memory limit %s",
			forbiddenBy, g.accepted.CPURequest, g.accepted.CPULimit, g.accepted.MemoryRequest, g.accepted.MemoryLimit)
		if g.accepted != g.configured {
			slog.ErrorContext(ctx, "refusing a fleet defaults change that an enforced policy forbids", "policies", forbiddenBy)
		}
		return nil
	}
	g.refusal = ""
	if g.accepted == g.configured {
		return g.persistLocked(ctx)
	}
	g.accepted = g.configured
	return g.persistLocked(ctx)
}

// forbiddingPolicies returns the names of the policies that forbid the default
// request (the same default-request check as PLAT-PGP-04 rule 3).
func forbiddingPolicies(enforced []cedar.PolicyInput, d cedar.Defaults) ([]string, error) {
	var out []string
	for _, p := range enforced {
		ev, err := cedar.NewEvaluator([]cedar.PolicyInput{p})
		if err != nil {
			// A policy that does not compile is not published and cannot forbid.
			continue
		}
		if decision := ev.Evaluate(cedar.Request{Context: d.DefaultContext()}); !decision.Allowed {
			out = append(out, p.Name)
		}
	}
	return out, nil
}

func (g *PolicyDefaultsGuard) load(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.loaded || g.kube == nil {
		g.loaded = true
		return nil
	}
	cm, err := g.kube.CoreV1().ConfigMaps(g.namespace).Get(ctx, PolicyDefaultsConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		g.loaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("read applied fleet defaults: %w", err)
	}
	var prev FleetDefaults
	if err := json.Unmarshal([]byte(cm.Data[policyDefaultsKey]), &prev); err != nil {
		return fmt.Errorf("decode applied fleet defaults: %w", err)
	}
	if _, err := prev.normalized(); err != nil {
		return fmt.Errorf("applied fleet defaults are invalid: %w", err)
	}
	g.accepted = prev
	g.loaded = true
	return nil
}

func (g *PolicyDefaultsGuard) persistLocked(ctx context.Context) error {
	if g.kube == nil {
		return nil
	}
	raw, err := json.Marshal(g.accepted)
	if err != nil {
		return fmt.Errorf("encode applied fleet defaults: %w", err)
	}
	cms := g.kube.CoreV1().ConfigMaps(g.namespace)
	existing, err := cms.Get(ctx, PolicyDefaultsConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = cms.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: PolicyDefaultsConfigMap, Namespace: g.namespace,
				Labels: map[string]string{policybundle.LabelManagedBy: policybundle.ManagedByValue}},
			Data: map[string]string{policyDefaultsKey: string(raw)},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("record applied fleet defaults: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read applied fleet defaults: %w", err)
	}
	if existing.Data[policyDefaultsKey] == string(raw) {
		return nil
	}
	updated := existing.DeepCopy()
	updated.Data = map[string]string{policyDefaultsKey: string(raw)}
	if _, err := cms.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("record applied fleet defaults: %w", err)
	}
	return nil
}
