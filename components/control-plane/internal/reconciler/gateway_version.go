package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
)

const (
	defaultGatewayVersionTimeout  = 3 * time.Second
	maxGatewayHealthResponseBytes = 4 * 1024
	maxGatewayVersionLength       = 128
)

type gatewayVersionObserver interface {
	Observe(ctx context.Context, namespace string) (string, error)
}

type httpGatewayVersionObserver struct {
	client   *http.Client
	timeout  time.Duration
	endpoint func(namespace string) string
}

func (h *GatewayHealthReconciler) reconcileGatewayVersion(ctx context.Context, client pb.GatewayServiceClient, gatewayRecord *pb.Gateway, namespace string) {
	if h.versionObserver == nil {
		return
	}
	gatewayID := gatewayRecord.GetMetadata().GetId()
	if gatewayID == "" {
		return
	}

	if err := gateway.ReconcileGatewayHealthAccess(ctx, h.clientset, namespace, h.controlPlaneNamespace, h.skipNetworkPolicies); err != nil {
		log.Printf("WARN gateway version: reconcile access for %s: %v", gatewayID, err)
		return
	}
	observedVersion, err := h.versionObserver.Observe(ctx, namespace)
	if err != nil {
		log.Printf("WARN gateway version: observe runtime for %s: %v", gatewayID, err)
		return
	}
	request := gatewayVersionUpdateRequest(gatewayRecord, observedVersion)
	if request == nil {
		return
	}

	updateCtx, cancel := context.WithTimeout(ctx, defaultGatewayVersionTimeout)
	defer cancel()
	if _, err := client.SetGatewayVersion(updateCtx, request); err != nil {
		log.Printf("WARN gateway version: store runtime version for %s: %v", gatewayID, err)
		return
	}
	log.Printf("INFO gateway version: %s runtime version set to %s", gatewayID, observedVersion)
}

func gatewayVersionUpdateRequest(gatewayRecord *pb.Gateway, observedVersion string) *pb.SetGatewayVersionRequest {
	if gatewayRecord == nil || gatewayRecord.GetMetadata().GetId() == "" || observedVersion == "" || observedVersion == gatewayRecord.GetGatewayVersion() {
		return nil
	}
	return &pb.SetGatewayVersionRequest{
		Id:             gatewayRecord.GetMetadata().GetId(),
		GatewayVersion: observedVersion,
	}
}

func newHTTPGatewayVersionObserver() *httpGatewayVersionObserver {
	return &httpGatewayVersionObserver{
		client: &http.Client{
			Timeout: defaultGatewayVersionTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		timeout: defaultGatewayVersionTimeout,
		endpoint: func(namespace string) string {
			return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/health", gateway.GatewayHealthServiceName, namespace, gateway.GatewayHealthPort)
		},
	}
}

func (o *httpGatewayVersionObserver) Observe(ctx context.Context, namespace string) (string, error) {
	if strings.TrimSpace(namespace) == "" {
		return "", fmt.Errorf("gateway namespace is required")
	}

	requestCtx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, o.endpoint(namespace), nil)
	if err != nil {
		return "", fmt.Errorf("create gateway health request: %w", err)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request gateway health: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices) && resp.StatusCode != http.StatusServiceUnavailable {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxGatewayHealthResponseBytes))
		return "", fmt.Errorf("gateway health returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGatewayHealthResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("read gateway health response: %w", err)
	}
	if len(body) > maxGatewayHealthResponseBytes {
		return "", fmt.Errorf("gateway health response is too large")
	}

	var health struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		return "", fmt.Errorf("decode gateway health response: %w", err)
	}

	version := strings.TrimSpace(health.Version)
	if version == "" {
		return "", fmt.Errorf("gateway health response has no version")
	}
	if strings.IndexFunc(version, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("gateway health version contains a control character")
	}
	if len(version) > maxGatewayVersionLength {
		return "", fmt.Errorf("gateway health version is too long")
	}

	return version, nil
}
