package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
)

// Rollout conditions written back into PlatformGatewayPolicy status
// (CTRL-CORE-22.03).
const (
	PolicyConditionApplied     = "Applied"
	PolicyConditionPending     = "Pending"
	PolicyConditionUnsupported = "Unsupported"

	// PolicyCanaryDisabledMessage marks the canary's status when the controller
	// setting disables it, so the short-mode e2e check reports skipped
	// (PLAT-PGP-20.02, E2E-CORE-49.03).
	PolicyCanaryDisabledMessage = "disabled: HYPERSHELL_POLICY_CANARY_ENABLED=false in this environment"

	sandboxPodLabel  = "agents.x-k8s.io/sandbox-name-hash"
	gatewayPageSize  = 500
	statusWriteLimit = 20 // non-compliant sandbox names kept per gateway
)

// AppliedRevisions records, per gateway, the bundle revision its interceptor
// sidecar last reported as applied (via ReportPolicyEvents). Safe for
// concurrent use.
type AppliedRevisions struct {
	mu   sync.RWMutex
	revs map[string]string
}

// NewAppliedRevisions returns an empty record.
func NewAppliedRevisions() *AppliedRevisions {
	return &AppliedRevisions{revs: map[string]string{}}
}

// Set records the applied revision and reports whether it changed.
func (a *AppliedRevisions) Set(gatewayID, revision string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.revs[gatewayID] == revision {
		return false
	}
	a.revs[gatewayID] = revision
	return true
}

// Get returns the last reported revision for the gateway.
func (a *AppliedRevisions) Get(gatewayID string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.revs[gatewayID]
}

// PolicyGateway is the subset of a gateway the bundle reconcile needs.
type PolicyGateway struct {
	ID        string
	Name      string
	Namespace string
	// Unsupported is true when the gateway's driver cannot honor sandbox
	// resource values (PLAT-PGP-18).
	Unsupported bool
}

// PolicyBundleReconciler distributes platform gateway policies to the
// interceptor sidecars of this cluster's gateways (CTRL-CORE-22): it builds a
// per-gateway bundle, writes it to the hypershell-policy-bundle ConfigMap in the
// gateway namespace with update-or-create (never through Helm, so a policy
// change never rolls the gateway), and copies the applied revisions and
// conditions into policy status as a status-only write-back.
type PolicyBundleReconciler struct {
	kube          kubernetes.Interface
	policies      pb.PlatformGatewayPolicyServiceClient
	settings      pb.PlatformPolicySettingsServiceClient
	gateways      pb.GatewayServiceClient
	clusterID     string
	defaults      *FleetDefaultsSource
	canaryEnabled bool
	resync        time.Duration
	applied       *AppliedRevisions
	nudge         chan struct{}
	now           func() time.Time
	// unsupported reports whether a gateway's driver cannot honor resource
	// values. Every gateway this control plane deploys runs the OpenShell
	// Kubernetes compute driver (the chart's server.drivers.kubernetes), which
	// honors spec.template.resources, so the default reports false.
	unsupported func(gw *pb.Gateway) bool
}

// NewPolicyBundleReconciler wires the reconciler to the api-server connection
// and the cluster's Kubernetes API.
func NewPolicyBundleReconciler(kube kubernetes.Interface, conn grpc.ClientConnInterface, clusterID string, defaults *FleetDefaultsSource, canaryEnabled bool, resync time.Duration, applied *AppliedRevisions) *PolicyBundleReconciler {
	if resync <= 0 {
		resync = 30 * time.Second
	}
	return &PolicyBundleReconciler{
		kube:          kube,
		policies:      pb.NewPlatformGatewayPolicyServiceClient(conn),
		settings:      pb.NewPlatformPolicySettingsServiceClient(conn),
		gateways:      pb.NewGatewayServiceClient(conn),
		clusterID:     clusterID,
		defaults:      defaults,
		canaryEnabled: canaryEnabled,
		resync:        resync,
		applied:       applied,
		nudge:         make(chan struct{}, 1),
		now:           time.Now,
		unsupported:   func(*pb.Gateway) bool { return false },
	}
}

// Nudge requests a reconcile soon. Non-blocking; coalesces bursts.
func (r *PolicyBundleReconciler) Nudge() {
	select {
	case r.nudge <- struct{}{}:
	default:
	}
}

// Run reconciles on every policy or settings watch event, every sidecar
// revision report (through Nudge), and on the periodic resync, until ctx ends.
func (r *PolicyBundleReconciler) Run(ctx context.Context) error {
	go r.watchPolicies(ctx)
	go r.watchSettings(ctx)
	ticker := time.NewTicker(r.resync)
	defer ticker.Stop()
	r.Nudge()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-r.nudge:
		}
		if err := r.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "policy bundle reconcile failed; retrying on the next event or resync", "error", err)
		}
	}
}

func (r *PolicyBundleReconciler) watchPolicies(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		stream, err := r.policies.WatchPlatformGatewayPolicies(ctx, &pb.WatchPlatformGatewayPoliciesRequest{})
		if err == nil {
			backoff = time.Second
			for {
				if _, err = stream.Recv(); err != nil {
					break
				}
				r.Nudge()
			}
		}
		if ctx.Err() != nil {
			return
		}
		slog.WarnContext(ctx, "PlatformGatewayPolicy watch ended; reconnecting", "error", err, "backoff", backoff)
		sleepCtx(ctx, backoff)
		backoff = min(backoff*2, time.Minute)
		r.Nudge()
	}
}

func (r *PolicyBundleReconciler) watchSettings(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		stream, err := r.settings.WatchPlatformPolicySettings(ctx, &pb.WatchPlatformPolicySettingsRequest{})
		if err == nil {
			backoff = time.Second
			for {
				if _, err = stream.Recv(); err != nil {
					break
				}
				r.Nudge()
			}
		}
		if ctx.Err() != nil {
			return
		}
		slog.WarnContext(ctx, "PlatformPolicySettings watch ended; reconnecting", "error", err, "backoff", backoff)
		sleepCtx(ctx, backoff)
		backoff = min(backoff*2, time.Minute)
		r.Nudge()
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// ListPolicyGateways lists this cluster's live gateways that have a namespace.
func (r *PolicyBundleReconciler) ListPolicyGateways(ctx context.Context) ([]PolicyGateway, error) {
	var out []PolicyGateway
	cluster := r.clusterID
	for page := int32(1); ; page++ {
		resp, err := r.gateways.ListGateways(ctx, &pb.ListGatewaysRequest{Page: page, Size: gatewayPageSize, ClusterId: &cluster})
		if err != nil {
			return nil, fmt.Errorf("list gateways: %w", err)
		}
		for _, gw := range resp.GetItems() {
			if gw.GetNamespace() == "" || strings.EqualFold(gw.GetPhase(), "Deleting") {
				continue
			}
			out = append(out, PolicyGateway{
				ID:          gw.GetMetadata().GetId(),
				Name:        gw.GetName(),
				Namespace:   gw.GetNamespace(),
				Unsupported: r.unsupported(gw),
			})
		}
		if len(resp.GetItems()) < gatewayPageSize {
			return out, nil
		}
	}
}

// bundleInputs are the fleet-wide inputs every gateway's bundle is built from.
type bundleInputs struct {
	policies   []*pb.PlatformGatewayPolicy
	globalMode string
	defaults   FleetDefaults
}

// loadBundleInputs reads the policies and the settings from the api-server.
// Bundles validate against the fleet defaults the api-server accepted and
// distributes through the settings (PLAT-PGP-08). Without them no bundle can be
// built, so sidecars without a bundle fail closed (PLAT-PGP-12).
func (r *PolicyBundleReconciler) loadBundleInputs(ctx context.Context) (bundleInputs, error) {
	list, err := r.policies.ListPlatformGatewayPolicies(ctx, &pb.ListPlatformGatewayPoliciesRequest{})
	if err != nil {
		return bundleInputs{}, fmt.Errorf("list platform gateway policies: %w", err)
	}
	settings, err := r.settings.GetPlatformPolicySettings(ctx, &pb.GetPlatformPolicySettingsRequest{})
	if err != nil {
		return bundleInputs{}, fmt.Errorf("get platform policy settings: %w", err)
	}
	accepted, err := r.defaults.Update(settings.GetSettings())
	if err != nil {
		return bundleInputs{}, fmt.Errorf("fleet defaults: %w", err)
	}
	if refusal := settings.GetSettings().GetDefaultsRefusal(); refusal != "" {
		slog.WarnContext(ctx, "the api-server refused a fleet defaults change; applying the accepted defaults", "reason", refusal)
	}
	return bundleInputs{policies: list.GetItems(), globalMode: settings.GetSettings().GetMode(), defaults: accepted}, nil
}

func (r *PolicyBundleReconciler) buildBundle(gw PolicyGateway, in bundleInputs) (policybundle.Bundle, []policybundle.Skipped, error) {
	return policybundle.Build(policybundle.Input{
		GatewayID:         gw.ID,
		GlobalMode:        in.globalMode,
		Policies:          in.policies,
		Defaults:          in.defaults.Normalized,
		DefaultQuantities: in.defaults.BundleDefaults(),
		Unsupported:       gw.Unsupported,
		CanaryEnabled:     r.canaryEnabled,
	})
}

// EnsureGatewayBundle builds and writes (update-or-create) the current bundle
// for one gateway, with the same code as the periodic reconcile. The gateway
// reconciler calls it before the Helm install or upgrade of a gateway with the
// interceptor, so a new or rolled sidecar finds its bundle at startup
// (PLAT-PGP-12.03, CTRL-CORE-22.06). Any failure, including settings without
// fleet defaults or a namespace that does not exist yet, is an error so the
// gateway reconcile retries instead of deploying a sidecar that fails closed.
func (r *PolicyBundleReconciler) EnsureGatewayBundle(ctx context.Context, gw *pb.Gateway, namespace string) error {
	in, err := r.loadBundleInputs(ctx)
	if err != nil {
		return err
	}
	pg := PolicyGateway{ID: gw.GetMetadata().GetId(), Name: gw.GetName(), Namespace: namespace, Unsupported: r.unsupported(gw)}
	bundle, _, err := r.buildBundle(pg, in)
	if err != nil {
		return fmt.Errorf("build bundle for gateway %s: %w", pg.ID, err)
	}
	written, err := r.writeBundle(ctx, pg, bundle)
	if err != nil {
		return err
	}
	if !written {
		return fmt.Errorf("write policy bundle for gateway %s: namespace %s does not exist yet", pg.ID, namespace)
	}
	r.Nudge()
	return nil
}

// ReconcileOnce performs one full, idempotent reconcile (STD-RCON-02): an
// unchanged policy set converges to the same ConfigMaps without a revision
// bump, and status is written only when it changed.
func (r *PolicyBundleReconciler) ReconcileOnce(ctx context.Context) error {
	in, err := r.loadBundleInputs(ctx)
	if err != nil {
		return err
	}
	gateways, err := r.ListPolicyGateways(ctx)
	if err != nil {
		return err
	}
	policies := in.policies
	globalMode := in.globalMode

	pods, podsErr := r.listSandboxPods(ctx)
	if podsErr != nil {
		slog.WarnContext(ctx, "listing sandbox pods for policy compliance failed; non-compliant sandboxes are not reported this pass", "error", podsErr)
	}

	desired := map[string]map[string]GatewayPolicyStatus{} // policy id -> gateway id -> status
	var errs []error
	for _, gw := range gateways {
		bundle, skipped, err := r.buildBundle(gw, in)
		if err != nil {
			errs = append(errs, fmt.Errorf("build bundle for gateway %s: %w", gw.ID, err))
			continue
		}
		written, err := r.writeBundle(ctx, gw, bundle)
		if err != nil {
			errs = append(errs, err)
		}
		nonCompliant := r.nonCompliant(bundle, gw, pods)
		applied := r.applied.Get(gw.ID)
		skippedByID := map[string]policybundle.Skipped{}
		for _, s := range skipped {
			skippedByID[s.PolicyID] = s
		}
		for _, p := range policies {
			if !policybundle.Targets(p, gw.ID) {
				continue
			}
			id := p.GetMetadata().GetId()
			st := GatewayPolicyStatus{
				GatewayID:     gw.ID,
				Revision:      bundle.Revision,
				Applied:       applied,
				EffectiveMode: policybundle.EffectiveMode(globalMode, p.GetEnforcementMode()),
				Condition:     PolicyConditionPending,
			}
			present, enforced := bundle.Contains(p.GetName())
			switch {
			case gw.Unsupported:
				st.Condition = PolicyConditionUnsupported
				st.EffectiveMode = policybundle.ModeAudit
				st.Message = "the gateway's compute driver cannot honor sandbox resource values"
			case !present:
				if s, ok := skippedByID[id]; ok {
					if strings.HasPrefix(s.Reason, "disabled") {
						st.Message = PolicyCanaryDisabledMessage
					} else {
						st.Message = "not published: " + s.Reason
					}
				}
			case written && applied == bundle.Revision:
				st.Condition = PolicyConditionApplied
			default:
				st.Message = "waiting for the gateway's policy interceptor to apply revision " + bundle.Revision
			}
			if enforced {
				st.NonCompliant = nonCompliant[p.GetName()]
			}
			if desired[id] == nil {
				desired[id] = map[string]GatewayPolicyStatus{}
			}
			desired[id][gw.ID] = st
		}
	}

	own := map[string]bool{}
	for _, gw := range gateways {
		own[gw.ID] = true
	}
	for _, p := range policies {
		if err := r.writeStatus(ctx, p, desired[p.GetMetadata().GetId()], own); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// writeBundle update-or-creates the gateway's bundle ConfigMap. It reports
// whether the ConfigMap now holds this revision. A missing gateway namespace
// (not yet provisioned) is not an error: the next pass retries.
func (r *PolicyBundleReconciler) writeBundle(ctx context.Context, gw PolicyGateway, bundle policybundle.Bundle) (bool, error) {
	raw, err := bundle.Encode()
	if err != nil {
		return false, fmt.Errorf("encode bundle for gateway %s: %w", gw.ID, err)
	}
	labels := map[string]string{
		policybundle.LabelGatewayID:     gw.ID,
		policybundle.LabelSchemaVersion: policybundle.SchemaVersion,
		policybundle.LabelManagedBy:     policybundle.ManagedByValue,
	}
	cms := r.kube.CoreV1().ConfigMaps(gw.Namespace)
	existing, err := cms.Get(ctx, policybundle.ConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: policybundle.ConfigMapName, Namespace: gw.Namespace, Labels: labels},
			Data:       map[string]string{policybundle.DataKey: string(raw)},
		}
		if _, err := cms.Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("create policy bundle for gateway %s: %w", gw.ID, err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("get policy bundle for gateway %s: %w", gw.ID, err)
	}
	if existing.Data[policybundle.DataKey] == string(raw) && labelsMatch(existing.Labels, labels) {
		return true, nil
	}
	updated := existing.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	for k, v := range labels {
		updated.Labels[k] = v
	}
	updated.Data = map[string]string{policybundle.DataKey: string(raw)}
	if _, err := cms.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return false, fmt.Errorf("update policy bundle for gateway %s: %w", gw.ID, err)
	}
	slog.InfoContext(ctx, "policy bundle written", "gateway_id", gw.ID, "namespace", gw.Namespace, "revision", bundle.Revision,
		"enforced", len(bundle.Enforced), "audit_only", len(bundle.AuditOnly))
	return true, nil
}

func labelsMatch(have, want map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

func (r *PolicyBundleReconciler) listSandboxPods(ctx context.Context) ([]corev1.Pod, error) {
	list, err := r.kube.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sandboxPodLabel})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// nonCompliant evaluates the enforced set against this gateway's running
// sandbox pods and returns, per policy name, the sandboxes it would reject
// (PLAT-PGP-16). It never stops a sandbox.
func (r *PolicyBundleReconciler) nonCompliant(bundle policybundle.Bundle, gw PolicyGateway, pods []corev1.Pod) map[string][]string {
	out := map[string][]string{}
	if len(bundle.Enforced) == 0 || len(pods) == 0 {
		return out
	}
	enforced, _, err := bundle.Evaluators()
	if err != nil {
		return out
	}
	for i := range pods {
		pod := &pods[i]
		if pod.Status.Phase != corev1.PodRunning || gateway.SandboxGatewayNamespace(pod.Namespace) != gw.Namespace {
			continue
		}
		ctx := podContext(pod)
		decision := enforced.Evaluate(cedar.Request{GatewayID: gw.ID, GatewayName: gw.Name, Context: ctx})
		for _, name := range decision.MatchedPolicies() {
			if len(out[name]) < statusWriteLimit {
				out[name] = append(out[name], pod.Name)
			}
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// podContext reads the agent container's resources from a running sandbox
// pod into the Cedar context. The first container is the sandbox workload.
func podContext(pod *corev1.Pod) cedar.Context {
	c := cedar.Context{SandboxName: pod.Name}
	if len(pod.Spec.Containers) == 0 {
		return c
	}
	ctr := pod.Spec.Containers[0]
	c.Image = ctr.Image
	if q, ok := ctr.Resources.Requests[corev1.ResourceCPU]; ok {
		c.CPURequestMillis = q.MilliValue()
	}
	if q, ok := ctr.Resources.Limits[corev1.ResourceCPU]; ok {
		c.CPULimitMillis = q.MilliValue()
	}
	if q, ok := ctr.Resources.Requests[corev1.ResourceMemory]; ok {
		c.MemoryRequestBytes = q.Value()
	}
	if q, ok := ctr.Resources.Limits[corev1.ResourceMemory]; ok {
		c.MemoryLimitBytes = q.Value()
	}
	if q, ok := ctr.Resources.Limits["nvidia.com/gpu"]; ok {
		c.GPUCount = q.Value()
	}
	return c
}

// GatewayPolicyStatus is the desired rollout entry for one policy on one
// gateway.
type GatewayPolicyStatus struct {
	GatewayID     string
	Condition     string
	Revision      string
	Applied       string
	EffectiveMode string
	Message       string
	NonCompliant  []string
}

func (s GatewayPolicyStatus) equal(p *pb.PlatformGatewayPolicyGatewayStatus) bool {
	return p != nil && p.GetCondition() == s.Condition && p.GetRevision() == s.Revision &&
		p.GetAppliedRevision() == s.Applied && p.GetEffectiveMode() == s.EffectiveMode &&
		p.GetMessage() == s.Message && strings.Join(p.GetNonCompliantSandboxes(), ",") == strings.Join(s.NonCompliant, ",")
}

// writeStatus writes the policy's rollout entries for this cluster's gateways
// when they changed, and removes entries for this cluster's gateways the policy
// no longer targets. Entries of other clusters' gateways are never touched.
func (r *PolicyBundleReconciler) writeStatus(ctx context.Context, p *pb.PlatformGatewayPolicy, desired map[string]GatewayPolicyStatus, own map[string]bool) error {
	current := map[string]*pb.PlatformGatewayPolicyGatewayStatus{}
	for _, g := range p.GetGatewayStatuses() {
		current[g.GetGatewayId()] = g
	}
	req := &pb.UpdatePlatformGatewayPolicyStatusRequest{Id: p.GetMetadata().GetId()}
	ids := make([]string, 0, len(desired))
	for id := range desired {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	now := timestamppb.New(r.now())
	for _, id := range ids {
		d := desired[id]
		if d.equal(current[id]) {
			continue
		}
		req.GatewayStatuses = append(req.GatewayStatuses, &pb.PlatformGatewayPolicyGatewayStatus{
			GatewayId:             d.GatewayID,
			Condition:             d.Condition,
			Revision:              d.Revision,
			AppliedRevision:       d.Applied,
			EffectiveMode:         d.EffectiveMode,
			Message:               d.Message,
			NonCompliantSandboxes: d.NonCompliant,
			ObservedAt:            now,
		})
	}
	for id := range current {
		if own[id] {
			if _, still := desired[id]; !still {
				req.RemovedGatewayIds = append(req.RemovedGatewayIds, id)
			}
		}
	}
	sort.Strings(req.RemovedGatewayIds)
	if len(req.GatewayStatuses) == 0 && len(req.RemovedGatewayIds) == 0 {
		return nil
	}
	if _, err := r.policies.UpdatePlatformGatewayPolicyStatus(ctx, req); err != nil {
		return fmt.Errorf("write status of policy %s: %w", p.GetName(), err)
	}
	return nil
}
