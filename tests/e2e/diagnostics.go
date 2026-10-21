package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

const (
	envDiagnosticsDir     = "E2E_DIAGNOSTICS_DIR"
	envDiagnosticsTimeout = "E2E_DIAGNOSTICS_TIMEOUT"
	managedNamespaceLabel = "hypershell.redhat.io/managed=true"
	diagnosticLogLimit    = int64(200)
)

type failureDiagnostics struct {
	suite *E2ESuite
	dir   string
}

// collectFailureDiagnostics captures the same cluster evidence for every Go E2E
// failure, independent of whether the suite runs in GitHub Actions, a promotion
// gate, or a developer terminal. It runs before cleanup so failed gateways and
// their pods still exist. When E2E_DIAGNOSTICS_DIR is set, each section is also
// persisted for the CI artifact uploader; the test log always receives it.
func (s *E2ESuite) collectFailureDiagnostics() {
	if s.clients == nil || s.clients.Kube == nil {
		s.Log("DIAG failure diagnostics unavailable: Kubernetes client was not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), durationSecondsEnv(envDiagnosticsTimeout, 45*time.Second))
	defer cancel()

	d := &failureDiagnostics{suite: s}
	if base := os.Getenv(envDiagnosticsDir); base != "" {
		d.dir = filepath.Join(base, diagnosticPathComponent(s.runID))
	}
	s.Logf("DIAG collecting failure diagnostics (timeout=%s)", durationSecondsEnv(envDiagnosticsTimeout, 45*time.Second))
	d.section(ctx, "All pods", "pods.txt", d.writeAllPods)
	d.section(ctx, "Pod descriptions (platform namespace)", "pod-describes.txt", d.writePlatformPodDescriptions)
	d.section(ctx, "Pod logs (platform namespace)", "pod-logs.txt", d.writePlatformPodLogs)
	d.section(ctx, "Stand-in postgres logs", "postgres.txt", func(ctx context.Context, w io.Writer) {
		d.writeSelectedPodLogs(ctx, w, "external-cloud-db", []string{"app=postgres"}, diagnosticLogLimit, false)
	})
	d.section(ctx, "Keycloak logs", "keycloak-logs.txt", d.writeKeycloakLogs)
	d.section(ctx, "Tracing diagnostics", "tracing.txt", d.writeTracingDiagnostics)
	d.section(ctx, "Platform events", "events.txt", d.writePlatformEvents)
	d.section(ctx, "Gateway API and managed namespaces", "gateway-api.txt", d.writeGatewayDiagnostics)
	d.section(ctx, "OpenShell console", "openshell-console.txt", d.writeConsoleDiagnostics)
}

func (d *failureDiagnostics) section(ctx context.Context, title, filename string, collect func(context.Context, io.Writer)) {
	var b strings.Builder
	collect(ctx, &b)
	if err := ctx.Err(); err != nil {
		writeDiagnosticf(&b, "diagnostic context: %v\n", err)
	}
	body := strings.TrimSpace(b.String())
	if body == "" {
		body = "no diagnostic data found"
	}
	body = harness.RedactDiagnostic(body)
	d.suite.Logf("DIAG %s\n%s", title, body)

	if d.dir == "" {
		return
	}
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		d.suite.Logf("DIAG create artifact directory %s: %v", d.dir, err)
		return
	}
	path := filepath.Join(d.dir, filename)
	if err := os.WriteFile(path, []byte(body+"\n"), 0o600); err != nil {
		d.suite.Logf("DIAG write artifact %s: %v", path, err)
	}
}

func (d *failureDiagnostics) writeAllPods(ctx context.Context, w io.Writer) {
	pods, err := d.suite.clients.Kube.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeDiagnosticf(w, "list pods across namespaces: %v\n", err)
		return
	}
	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[i].Namespace+"/"+pods.Items[i].Name < pods.Items[j].Namespace+"/"+pods.Items[j].Name
	})
	for i := range pods.Items {
		writePodSummary(w, &pods.Items[i])
	}
}

func (d *failureDiagnostics) writePlatformPodDescriptions(ctx context.Context, w io.Writer) {
	ns := d.platformNamespace(w)
	if ns == "" {
		return
	}
	pods, err := d.suite.clients.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeDiagnosticf(w, "list pods in %s: %v\n", ns, err)
		return
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	for i := range pods.Items {
		writePodDescription(w, &pods.Items[i])
	}
}

func (d *failureDiagnostics) writePlatformPodLogs(ctx context.Context, w io.Writer) {
	ns := d.platformNamespace(w)
	if ns == "" {
		return
	}
	d.writeSelectedPodLogs(ctx, w, ns, []string{
		"app.kubernetes.io/name=hypershell",
		"app=hypershell-api-server",
		"app=hypershell-controller",
		"app=hypershell-web-console",
	}, diagnosticLogLimit, false)
}

func (d *failureDiagnostics) writeKeycloakLogs(ctx context.Context, w io.Writer) {
	namespaces := []string{"keycloak"}
	if platform := d.platformNamespace(w); platform != "" {
		namespaces = append(namespaces, platform, platform+"-keycloak")
	}
	seen := map[string]bool{}
	for _, ns := range namespaces {
		if seen[ns] {
			continue
		}
		seen[ns] = true
		d.writeSelectedPodLogs(ctx, w, ns, []string{"app=keycloak"}, 100, false)
	}
}

func (d *failureDiagnostics) writeTracingDiagnostics(ctx context.Context, w io.Writer) {
	ns := d.platformNamespace(w)
	if ns == "" {
		return
	}
	pods, err := d.suite.clients.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "app=jaeger"})
	if err != nil {
		writeDiagnosticf(w, "list Jaeger pods in %s: %v\n", ns, err)
	} else {
		for i := range pods.Items {
			writePodSummary(w, &pods.Items[i])
		}
	}
	services, err := d.suite.clients.Kube.CoreV1().Services(ns).List(ctx, metav1.ListOptions{LabelSelector: "app=jaeger"})
	if err != nil {
		writeDiagnosticf(w, "list Jaeger services in %s: %v\n", ns, err)
	} else {
		for i := range services.Items {
			writeServiceSummary(w, &services.Items[i])
		}
	}
	if d.suite.clients.Gateway != nil {
		routes, err := d.suite.clients.Gateway.GatewayV1().HTTPRoutes(ns).List(ctx, metav1.ListOptions{LabelSelector: "app=jaeger"})
		if err != nil {
			writeDiagnosticf(w, "list Jaeger HTTPRoutes in %s: %v\n", ns, err)
		} else {
			for i := range routes.Items {
				writeHTTPRouteSummary(w, &routes.Items[i])
			}
		}
	}
	d.writeSelectedPodLogs(ctx, w, ns, []string{"app=jaeger"}, 100, false)

	deployment, err := d.suite.clients.Kube.AppsV1().Deployments(ns).Get(ctx, "hypershell-web-console", metav1.GetOptions{})
	if err != nil {
		writeDiagnosticf(w, "get deployment %s/hypershell-web-console: %v\n", ns, err)
		return
	}
	writeDiagnosticf(w, "--- web-console OTEL env ---\n")
	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if !strings.Contains(strings.ToLower(env.Name), "otel") {
				continue
			}
			value := env.Value
			if env.ValueFrom != nil {
				value = "<valueFrom>"
			}
			writeDiagnosticf(w, "%s/%s=%s\n", container.Name, env.Name, value)
		}
	}
}

func (d *failureDiagnostics) writePlatformEvents(ctx context.Context, w io.Writer) {
	ns := d.platformNamespace(w)
	if ns != "" {
		d.writeEvents(ctx, w, ns)
	}
}

func (d *failureDiagnostics) writeGatewayDiagnostics(ctx context.Context, w io.Writer) {
	if d.suite.clients.Gateway == nil {
		writeDiagnosticf(w, "Gateway API client was not initialized\n")
	} else {
		gateways, err := d.suite.clients.Gateway.GatewayV1().Gateways("").List(ctx, metav1.ListOptions{})
		if err != nil {
			writeDiagnosticf(w, "list Gateways: %v\n", err)
		} else {
			for i := range gateways.Items {
				writeGatewaySummary(w, &gateways.Items[i])
			}
		}
		grpcRoutes, err := d.suite.clients.Gateway.GatewayV1().GRPCRoutes("").List(ctx, metav1.ListOptions{})
		if err != nil {
			writeDiagnosticf(w, "list GRPCRoutes: %v\n", err)
		} else {
			for i := range grpcRoutes.Items {
				writeGRPCRouteSummary(w, &grpcRoutes.Items[i])
			}
		}
		httpRoutes, err := d.suite.clients.Gateway.GatewayV1().HTTPRoutes("").List(ctx, metav1.ListOptions{})
		if err != nil {
			writeDiagnosticf(w, "list HTTPRoutes: %v\n", err)
		} else {
			for i := range httpRoutes.Items {
				writeHTTPRouteSummary(w, &httpRoutes.Items[i])
			}
		}
	}

	d.writeOpenShiftRoutes(ctx, w)
	for _, ns := range d.managedNamespaces(ctx, w) {
		writeDiagnosticf(w, "--- managed namespace %s ---\n", ns)
		pods, err := d.suite.clients.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			writeDiagnosticf(w, "list pods: %v\n", err)
		} else {
			for i := range pods.Items {
				writePodDescription(w, &pods.Items[i])
			}
		}
		services, err := d.suite.clients.Kube.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			writeDiagnosticf(w, "list services: %v\n", err)
		} else {
			for i := range services.Items {
				writeServiceSummary(w, &services.Items[i])
			}
		}
		deployments, err := d.suite.clients.Kube.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			writeDiagnosticf(w, "list deployments: %v\n", err)
		} else {
			for i := range deployments.Items {
				writeDeploymentSummary(w, &deployments.Items[i])
			}
		}
		d.writeEvents(ctx, w, ns)
		d.writeSelectedPodLogs(ctx, w, ns, []string{""}, 80, true)
	}
}

func (d *failureDiagnostics) writeConsoleDiagnostics(ctx context.Context, w io.Writer) {
	for _, ns := range d.managedNamespaces(ctx, w) {
		writeDiagnosticf(w, "--- deployment %s/openshell-console ---\n", ns)
		deployment, err := d.suite.clients.Kube.AppsV1().Deployments(ns).Get(ctx, "openshell-console", metav1.GetOptions{})
		if err != nil {
			writeDiagnosticf(w, "get deployment: %v\n", err)
		} else {
			writeDeploymentSummary(w, deployment)
		}
		if d.suite.clients.Gateway != nil {
			route, err := d.suite.clients.Gateway.GatewayV1().HTTPRoutes(ns).Get(ctx, "openshell-console", metav1.GetOptions{})
			if err != nil {
				writeDiagnosticf(w, "get HTTPRoute: %v\n", err)
			} else {
				writeHTTPRouteSummary(w, route)
			}
		}
		d.writeSelectedPodLogs(ctx, w, ns, []string{"app.kubernetes.io/instance=openshell-console"}, 150, true)
	}
}

func (d *failureDiagnostics) writeOpenShiftRoutes(ctx context.Context, w io.Writer) {
	if d.suite.driver == nil || d.suite.driver.Name() != "openshift" || d.suite.clients.Dynamic == nil {
		return
	}
	routes, err := d.suite.clients.Dynamic.Resource(schema.GroupVersionResource{
		Group: "route.openshift.io", Version: "v1", Resource: "routes",
	}).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeDiagnosticf(w, "list OpenShift Routes: %v\n", err)
		return
	}
	for _, route := range routes.Items {
		host, _, _ := unstructured.NestedString(route.Object, "spec", "host")
		writeDiagnosticf(w, "Route %s/%s host=%q generation=%d\n", route.GetNamespace(), route.GetName(), host, route.GetGeneration())
	}
}

func (d *failureDiagnostics) writeSelectedPodLogs(ctx context.Context, w io.Writer, namespace string, selectors []string, tail int64, previous bool) {
	if namespace == "" {
		return
	}
	podsByName := map[string]corev1.Pod{}
	for _, selector := range selectors {
		pods, err := d.suite.clients.Kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			writeDiagnosticf(w, "list pods in %s selector %q: %v\n", namespace, selector, err)
			continue
		}
		for _, pod := range pods.Items {
			podsByName[pod.Name] = pod
		}
	}
	names := make([]string, 0, len(podsByName))
	for name := range podsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pod := podsByName[name]
		for _, container := range pod.Spec.Containers {
			d.writeOnePodLog(ctx, w, &pod, container.Name, tail, false)
			if previous && containerRestartCount(&pod, container.Name) > 0 {
				d.writeOnePodLog(ctx, w, &pod, container.Name, tail, true)
			}
		}
	}
}

func (d *failureDiagnostics) writeOnePodLog(ctx context.Context, w io.Writer, pod *corev1.Pod, container string, tail int64, previous bool) {
	label := "logs"
	if previous {
		label = "previous logs"
	}
	writeDiagnosticf(w, "--- %s %s/%s container=%s ---\n", label, pod.Namespace, pod.Name, container)
	data, err := d.suite.clients.Kube.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: container,
		Previous:  previous,
		TailLines: &tail,
	}).DoRaw(ctx)
	if err != nil {
		writeDiagnosticf(w, "read logs: %v\n", err)
		return
	}
	_, _ = w.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		writeDiagnosticf(w, "\n")
	}
}

func (d *failureDiagnostics) writeEvents(ctx context.Context, w io.Writer, namespace string) {
	events, err := d.suite.clients.Kube.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeDiagnosticf(w, "list events in %s: %v\n", namespace, err)
		return
	}
	sort.Slice(events.Items, func(i, j int) bool { return eventTime(&events.Items[i]).Before(eventTime(&events.Items[j])) })
	for i := range events.Items {
		event := &events.Items[i]
		writeDiagnosticf(w, "%s %s %s/%s count=%d reason=%q message=%q\n", eventTime(event).Format(time.RFC3339), event.Type,
			event.InvolvedObject.Kind, event.InvolvedObject.Name, event.Count, event.Reason, event.Message)
	}
}

func (d *failureDiagnostics) managedNamespaces(ctx context.Context, w io.Writer) []string {
	namespaces, err := d.suite.clients.Kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: managedNamespaceLabel})
	if err != nil {
		writeDiagnosticf(w, "list managed namespaces: %v\n", err)
		return nil
	}
	names := make([]string, 0, len(namespaces.Items))
	for _, namespace := range namespaces.Items {
		names = append(names, namespace.Name)
	}
	sort.Strings(names)
	return names
}

func (d *failureDiagnostics) platformNamespace(w io.Writer) string {
	if d.suite.driver == nil {
		writeDiagnosticf(w, "platform namespace unavailable: infrastructure driver was not initialized\n")
		return ""
	}
	return d.suite.driver.PlatformNamespace()
}

func writePodSummary(w io.Writer, pod *corev1.Pod) {
	ready := 0
	restarts := int32(0)
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
		restarts += status.RestartCount
	}
	writeDiagnosticf(w, "Pod %s/%s phase=%s ready=%d/%d restarts=%d node=%q reason=%q message=%q\n",
		pod.Namespace, pod.Name, pod.Status.Phase, ready, len(pod.Spec.Containers), restarts, pod.Spec.NodeName, pod.Status.Reason, pod.Status.Message)
}

func writePodDescription(w io.Writer, pod *corev1.Pod) {
	writePodSummary(w, pod)
	for _, condition := range pod.Status.Conditions {
		writeDiagnosticf(w, "  condition %s=%s reason=%q message=%q\n", condition.Type, condition.Status, condition.Reason, condition.Message)
	}
	for _, status := range append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...) {
		writeDiagnosticf(w, "  container %s image=%q ready=%t restarts=%d state=%s\n", status.Name, status.Image, status.Ready, status.RestartCount, containerState(status.State))
	}
}

func writeServiceSummary(w io.Writer, service *corev1.Service) {
	ports := make([]string, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		ports = append(ports, fmt.Sprintf("%s:%d/%s", port.Name, port.Port, port.Protocol))
	}
	writeDiagnosticf(w, "Service %s/%s type=%s clusterIP=%q ports=%s\n", service.Namespace, service.Name, service.Spec.Type, service.Spec.ClusterIP, strings.Join(ports, ","))
}

func writeDeploymentSummary(w io.Writer, deployment *appsv1.Deployment) {
	images := make([]string, 0, len(deployment.Spec.Template.Spec.Containers))
	for _, container := range deployment.Spec.Template.Spec.Containers {
		images = append(images, container.Name+"="+container.Image)
	}
	writeDiagnosticf(w, "Deployment %s/%s replicas=%d ready=%d available=%d updated=%d images=%s\n", deployment.Namespace, deployment.Name,
		valueOrZero(deployment.Spec.Replicas), deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas, strings.Join(images, ","))
	for _, condition := range deployment.Status.Conditions {
		writeDiagnosticf(w, "  condition %s=%s reason=%q message=%q\n", condition.Type, condition.Status, condition.Reason, condition.Message)
	}
}

func writeGatewaySummary(w io.Writer, gateway *gatewayv1.Gateway) {
	addresses := make([]string, 0, len(gateway.Status.Addresses))
	for _, address := range gateway.Status.Addresses {
		addresses = append(addresses, address.Value)
	}
	writeDiagnosticf(w, "Gateway %s/%s generation=%d addresses=%s conditions=%s\n", gateway.Namespace, gateway.Name, gateway.Generation,
		strings.Join(addresses, ","), formatConditions(gateway.Status.Conditions))
}

func writeGRPCRouteSummary(w io.Writer, route *gatewayv1.GRPCRoute) {
	writeDiagnosticf(w, "GRPCRoute %s/%s generation=%d parents=%s\n", route.Namespace, route.Name, route.Generation, formatRouteParents(route.Status.Parents))
}

func writeHTTPRouteSummary(w io.Writer, route *gatewayv1.HTTPRoute) {
	hostnames := make([]string, 0, len(route.Spec.Hostnames))
	for _, hostname := range route.Spec.Hostnames {
		hostnames = append(hostnames, string(hostname))
	}
	writeDiagnosticf(w, "HTTPRoute %s/%s generation=%d hostnames=%s parents=%s\n", route.Namespace, route.Name, route.Generation,
		strings.Join(hostnames, ","), formatRouteParents(route.Status.Parents))
}

func formatRouteParents(parents []gatewayv1.RouteParentStatus) string {
	parts := make([]string, 0, len(parents))
	for _, parent := range parents {
		parts = append(parts, fmt.Sprintf("%s controller=%s conditions=%s", parent.ParentRef.Name, parent.ControllerName, formatConditions(parent.Conditions)))
	}
	return strings.Join(parts, "; ")
}

func formatConditions(conditions []metav1.Condition) string {
	parts := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		parts = append(parts, fmt.Sprintf("%s=%s reason=%q message=%q observedGeneration=%d", condition.Type, condition.Status,
			condition.Reason, condition.Message, condition.ObservedGeneration))
	}
	return strings.Join(parts, "; ")
}

func containerState(state corev1.ContainerState) string {
	switch {
	case state.Waiting != nil:
		return fmt.Sprintf("waiting reason=%q message=%q", state.Waiting.Reason, state.Waiting.Message)
	case state.Terminated != nil:
		return fmt.Sprintf("terminated exit=%d reason=%q message=%q", state.Terminated.ExitCode, state.Terminated.Reason, state.Terminated.Message)
	case state.Running != nil:
		return "running since=" + state.Running.StartedAt.Format(time.RFC3339)
	default:
		return "unknown"
	}
}

func containerRestartCount(pod *corev1.Pod, name string) int32 {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == name {
			return status.RestartCount
		}
	}
	return 0
}

func eventTime(event *corev1.Event) time.Time {
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	return event.CreationTimestamp.Time
}

func valueOrZero(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func writeDiagnosticf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func diagnosticPathComponent(value string) string {
	if value == "" {
		return "setup-failure"
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, value)
}
