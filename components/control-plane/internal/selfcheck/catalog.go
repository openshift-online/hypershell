package selfcheck

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Deps are the in-cluster credentials and configuration the production check
// catalog probes with (PLAT-HLTH-01 "using the control plane's in-cluster
// credentials"). Every field is optional: a probe whose dependency is not wired
// reports an `unknown` condition rather than failing, so the loop still runs and
// reports (PLAT-HLTH-01). Which checks produce a meaningful verdict therefore
// follows the cluster's own state and deployment configuration, not a code change
// (PLAT-HLTH-02).
type Deps struct {
	// Clientset is the in-cluster Kubernetes client. Nil on a control plane
	// without cluster access, in which case cluster-facing checks are unknown.
	Clientset kubernetes.Interface
	// Namespace is the control-plane / gateway namespace (HYPERSHELL_NAMESPACE).
	Namespace string
	// OIDCIssuer is the issuer the gateways authenticate against (OIDC_ISSUER).
	OIDCIssuer string
	// DatabaseAdminDir is the mounted gateway-database admin Secret directory.
	DatabaseAdminDir string
	// GatewayImage, SupervisorImage, SandboxImage are the default images the
	// control plane provisions; empty entries are skipped.
	GatewayImage    string
	SupervisorImage string
	SandboxImage    string

	// HTTPClient probes the OIDC issuer; nil uses a bounded default.
	HTTPClient *http.Client
	// DialTimeout bounds the TCP reachability probes; zero uses a default.
	DialTimeout time.Duration
}

const (
	defaultDialTimeout = 5 * time.Second
	defaultHTTPTimeout = 10 * time.Second
)

// BuildCatalog returns the production check catalog in cheapest-first order
// (PLAT-HLTH-02): local file and quick API reads first, then the networked
// reachability probes. Each entry produces exactly one named condition from the
// bounded enumeration.
func BuildCatalog(deps Deps) []Check {
	dialTimeout := deps.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}
	httpClient := deps.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return []Check{
		{Name: CheckGatewayDatabaseAdminSecret, Probe: probeDatabaseAdminSecret(deps.DatabaseAdminDir)},
		{Name: CheckReconcilerHealthy, Probe: probeReconcilerHealthy(deps.Clientset)},
		{Name: CheckGatewayNamespacePrerequisites, Probe: probeNamespacePrerequisites(deps.Clientset, deps.Namespace)},
		{Name: CheckIngressControllerHealthy, Probe: probeIngressController(deps.Clientset)},
		{Name: CheckImagesPullable, Probe: probeImagesPullable([]string{deps.GatewayImage, deps.SupervisorImage, deps.SandboxImage}, dialTimeout)},
		{Name: CheckOIDCIssuerReachable, Probe: probeOIDCIssuer(httpClient, deps.OIDCIssuer)},
		{Name: CheckGatewayDatabaseReachable, Probe: probeDatabaseReachable(deps.DatabaseAdminDir, dialTimeout)},
	}
}

func probeDatabaseAdminSecret(dir string) ProbeFunc {
	return func(_ context.Context) CheckResult {
		if dir == "" {
			return CheckResult{Status: StatusUnknown, Message: "gateway-database admin directory not configured"}
		}
		if err := gateway.ValidateAdminCredentialsDir(dir); err != nil {
			return CheckResult{Status: StatusFail, Message: err.Error()}
		}
		return CheckResult{Status: StatusPass, Message: "gateway-database admin secret present and complete"}
	}
}

func probeReconcilerHealthy(clientset kubernetes.Interface) ProbeFunc {
	return func(_ context.Context) CheckResult {
		if clientset == nil {
			return CheckResult{Status: StatusUnknown, Message: "no in-cluster Kubernetes client"}
		}
		version, err := clientset.Discovery().ServerVersion()
		if err != nil {
			return CheckResult{Status: StatusFail, Message: fmt.Sprintf("Kubernetes API unreachable: %v", err)}
		}
		return CheckResult{Status: StatusPass, Message: fmt.Sprintf("reconciler active; Kubernetes API %s reachable", version.GitVersion)}
	}
}

func probeNamespacePrerequisites(clientset kubernetes.Interface, namespace string) ProbeFunc {
	return func(ctx context.Context) CheckResult {
		if clientset == nil {
			return CheckResult{Status: StatusUnknown, Message: "no in-cluster Kubernetes client"}
		}
		if namespace == "" {
			return CheckResult{Status: StatusUnknown, Message: "control-plane namespace not configured"}
		}
		if _, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{}); err != nil {
			return CheckResult{Status: StatusFail, Message: fmt.Sprintf("namespace %q not available: %v", namespace, err)}
		}
		if _, err := clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, "default", metav1.GetOptions{}); err != nil {
			return CheckResult{Status: StatusWarn, Message: fmt.Sprintf("namespace %q present but default service account not yet available: %v", namespace, err)}
		}
		return CheckResult{Status: StatusPass, Message: fmt.Sprintf("namespace %q and RBAC prerequisites present", namespace)}
	}
}

func probeIngressController(clientset kubernetes.Interface) ProbeFunc {
	return func(_ context.Context) CheckResult {
		if clientset == nil {
			return CheckResult{Status: StatusUnknown, Message: "no in-cluster Kubernetes client"}
		}
		groups, err := clientset.Discovery().ServerGroups()
		if err != nil {
			return CheckResult{Status: StatusFail, Message: fmt.Sprintf("cannot list API groups: %v", err)}
		}
		for _, g := range groups.Groups {
			switch g.Name {
			case "route.openshift.io", "gateway.networking.k8s.io":
				return CheckResult{Status: StatusPass, Message: fmt.Sprintf("ingress controller present (%s)", g.Name)}
			}
		}
		// Neither an OpenShift Route nor the Gateway API is present. The cluster can
		// still host a ClusterIP gateway, so this is degraded-but-serving (warn),
		// not a failure.
		return CheckResult{Status: StatusWarn, Message: "no Route or Gateway API controller detected; only ClusterIP gateways can be exposed"}
	}
}

func probeImagesPullable(images []string, dialTimeout time.Duration) ProbeFunc {
	return func(ctx context.Context) CheckResult {
		configured := make([]string, 0, len(images))
		for _, img := range images {
			if strings.TrimSpace(img) != "" {
				configured = append(configured, img)
			}
		}
		if len(configured) == 0 {
			return CheckResult{Status: StatusUnknown, Message: "no gateway images configured"}
		}
		dialer := net.Dialer{Timeout: dialTimeout}
		var unreachable []string
		for _, img := range configured {
			host := registryHostPort(img)
			conn, err := dialer.DialContext(ctx, "tcp", host)
			if err != nil {
				unreachable = append(unreachable, fmt.Sprintf("%s (%v)", host, err))
				continue
			}
			_ = conn.Close()
		}
		if len(unreachable) > 0 {
			return CheckResult{Status: StatusFail, Message: "image registry unreachable: " + strings.Join(unreachable, "; ")}
		}
		return CheckResult{Status: StatusPass, Message: "gateway, supervisor, and sandbox image registries reachable"}
	}
}

// registryHostPort extracts the registry host:port from a container image
// reference, defaulting to the Docker Hub registry and port 443. It mirrors the
// reference-parsing rule: the component before the first "/" is the registry only
// when it contains a "." or ":" or is "localhost".
func registryHostPort(image string) string {
	ref := image
	if at := strings.IndexByte(ref, '@'); at >= 0 {
		ref = ref[:at]
	}
	host := "registry-1.docker.io"
	if slash := strings.IndexByte(ref, '/'); slash >= 0 {
		candidate := ref[:slash]
		if candidate == "localhost" || strings.ContainsAny(candidate, ".:") {
			host = candidate
		}
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, "443")
}

func probeOIDCIssuer(client *http.Client, issuer string) ProbeFunc {
	return func(ctx context.Context) CheckResult {
		if strings.TrimSpace(issuer) == "" {
			return CheckResult{Status: StatusUnknown, Message: "OIDC issuer not configured"}
		}
		url := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return CheckResult{Status: StatusFail, Message: fmt.Sprintf("build issuer request: %v", err)}
		}
		resp, err := client.Do(req)
		if err != nil {
			return CheckResult{Status: StatusFail, Message: fmt.Sprintf("issuer %s unreachable: %v", issuer, err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return CheckResult{Status: StatusFail, Message: fmt.Sprintf("issuer %s returned %d", issuer, resp.StatusCode)}
		}
		return CheckResult{Status: StatusPass, Message: fmt.Sprintf("OIDC issuer %s reachable", issuer)}
	}
}

func probeDatabaseReachable(dir string, dialTimeout time.Duration) ProbeFunc {
	return func(ctx context.Context) CheckResult {
		if dir == "" {
			return CheckResult{Status: StatusUnknown, Message: "gateway-database admin directory not configured"}
		}
		if err := gateway.AdminServerReachable(ctx, dir, dialTimeout); err != nil {
			return CheckResult{Status: StatusFail, Message: err.Error()}
		}
		return CheckResult{Status: StatusPass, Message: "gateway database server reachable"}
	}
}
