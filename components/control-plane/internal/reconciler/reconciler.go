package reconciler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type FleetReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewFleetReconciler() *FleetReconciler {
	return &FleetReconciler{active: make(map[string]struct{})}
}

func (r *FleetReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Fleet]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	log.Printf("INFO reconciling Fleet %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

type ManagedClusterReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewManagedClusterReconciler() *ManagedClusterReconciler {
	return &ManagedClusterReconciler{active: make(map[string]struct{})}
}

func (r *ManagedClusterReconciler) Handle(ctx context.Context, event watcher.Event[*pb.ManagedCluster]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	log.Printf("INFO reconciling ManagedCluster %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

type ManagedDatabaseReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewManagedDatabaseReconciler() *ManagedDatabaseReconciler {
	return &ManagedDatabaseReconciler{active: make(map[string]struct{})}
}

func (r *ManagedDatabaseReconciler) Handle(ctx context.Context, event watcher.Event[*pb.ManagedDatabase]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	log.Printf("INFO reconciling ManagedDatabase %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

type GatewayReleaseReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewGatewayReleaseReconciler() *GatewayReleaseReconciler {
	return &GatewayReleaseReconciler{active: make(map[string]struct{})}
}

func (r *GatewayReleaseReconciler) Handle(ctx context.Context, event watcher.Event[*pb.GatewayRelease]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	log.Printf("INFO reconciling GatewayRelease %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

type GatewayReconciler struct {
	mu                    sync.Mutex
	active                map[string]struct{}
	dynamicClient         dynamic.Interface
	clientset             *kubernetes.Clientset
	grpcConn              *grpc.ClientConn
	manifests             map[string][]*unstructured.Unstructured
	isOpenShift           bool
	hasCertManager        bool
	hasGatewayAPI         bool
	manifestsDir          string
	controlPlaneNamespace string
	keycloakClient        *keycloak.Client
	keycloakConfig        *gateway.KeycloakConfig
}

func NewGatewayReconciler(
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	grpcConn *grpc.ClientConn,
	manifestsDir string,
	controlPlaneNamespace string,
	keycloakConfig *gateway.KeycloakConfig,
) (*GatewayReconciler, error) {
	manifests, err := gateway.LoadGatewayManifests(manifestsDir)
	if err != nil {
		return nil, fmt.Errorf("load gateway manifests from %s: %w", manifestsDir, err)
	}

	isOpenShift := gateway.DetectOpenShift(clientset)
	hasCertManager := gateway.DetectCertManager(clientset)
	hasGatewayAPI := gateway.DetectGatewayAPI(clientset)

	var kcClient *keycloak.Client
	if keycloakConfig != nil {
		kcClient = keycloak.NewClient(
			keycloakConfig.ServerURL,
			keycloakConfig.Realm,
			keycloakConfig.ClientID,
			keycloakConfig.ClientSecret,
		)
		log.Printf("INFO keycloak integration enabled: server=%s realm=%s", keycloakConfig.ServerURL, keycloakConfig.Realm)
	}

	log.Printf("INFO gateway reconciler initialized: manifests=%d openshift=%v certmanager=%v gatewayapi=%v keycloak=%v",
		len(manifests), isOpenShift, hasCertManager, hasGatewayAPI, kcClient != nil)

	return &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         dynamicClient,
		clientset:             clientset,
		grpcConn:              grpcConn,
		manifests:             manifests,
		isOpenShift:           isOpenShift,
		hasCertManager:        hasCertManager,
		hasGatewayAPI:         hasGatewayAPI,
		manifestsDir:          manifestsDir,
		controlPlaneNamespace: controlPlaneNamespace,
		keycloakClient:        kcClient,
		keycloakConfig:        keycloakConfig,
	}, nil
}

func (r *GatewayReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Gateway]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	gw := event.Resource
	if gw == nil {
		log.Printf("WARN gateway event %s has nil resource, skipping", event.ResourceID)
		return nil
	}

	if event.Type == watcher.EventDeleted {
		namespace := gatewayNamespace(gw)

		log.Printf("INFO gateway %s deleted, cleaning up resources in namespace %s", event.ResourceID, namespace)
		opts := gateway.ReconcileOpts{
			IsOpenShift:           r.isOpenShift,
			HasCertManager:        r.hasCertManager,
			HasGatewayAPI:         r.hasGatewayAPI,
			ControlPlaneNamespace: r.controlPlaneNamespace,
			KeycloakClient:        r.keycloakClient,
			GatewayID:             event.ResourceID,
			GatewayName:           gw.Name,
		}
		var credentialNamespaces []string
		if gw.CredentialDriver != nil && *gw.CredentialDriver != "" {
			if strings.Contains(*gw.CredentialDriver, "kubernetes_secrets") {
				var credCfg gateway.CredentialDriverConfig
				if err := json.Unmarshal([]byte(*gw.CredentialDriver), &credCfg); err == nil {
					if credCfg.KubernetesSecrets != nil && credCfg.KubernetesSecrets.Namespace != "" {
						credentialNamespaces = append(credentialNamespaces, credCfg.KubernetesSecrets.Namespace)
					}
				}
			}
		}
		if err := gateway.DeleteGatewayResources(ctx, r.dynamicClient, r.clientset, namespace, opts, credentialNamespaces...); err != nil {
			return fmt.Errorf("delete gateway resources in %s: %w", namespace, err)
		}
		log.Printf("INFO gateway %s resources cleaned up from namespace %s", event.ResourceID, namespace)
		return nil
	}

	log.Printf("INFO reconciling Gateway %s name=%s namespace=%s (event=%d)",
		event.ResourceID, gw.Name, gw.Namespace, event.Type)

	// The phase gate prevents redundant re-provisioning (re-applying manifests)
	// of a Gateway that has already been acted upon. Running, Provisioning, and
	// Degraded gateways are owned by the continuous health reconciler, which
	// keeps their phase synchronized with workload health via a separate path
	// that this gate does not suppress. See openshell-gateway-health.spec.md.
	if gw.Phase != nil && (*gw.Phase == "Running" || *gw.Phase == "Provisioning" || *gw.Phase == "Degraded") {
		log.Printf("DEBUG gateway %s phase=%s, skipping reconciliation", event.ResourceID, *gw.Phase)
		return nil
	}

	namespace := gatewayNamespace(gw)

	dnsNames := gw.ServerDnsNames
	if len(dnsNames) == 0 {
		dnsNames = []string{
			fmt.Sprintf("openshell-gateway.%s.svc.cluster.local", namespace),
		}
		if gw.ExternalDns != nil && *gw.ExternalDns != "" {
			dnsNames = append(dnsNames, *gw.ExternalDns)
		}
	}

	externalDns := ""
	if gw.ExternalDns != nil {
		externalDns = *gw.ExternalDns
	}

	gwConfig := gateway.GatewayConfig{
		ServerDnsNames: dnsNames,
		ExternalDns:    externalDns,
	}

	if gw.Image != nil && *gw.Image != "" {
		gwConfig.Image = *gw.Image
	}

	if gw.SupervisorImage != nil && *gw.SupervisorImage != "" {
		gwConfig.SupervisorImage = *gw.SupervisorImage
	}

	if gw.Oidc != nil && *gw.Oidc != "" {
		var oidcConfig gateway.OIDCConfig
		if err := json.Unmarshal([]byte(*gw.Oidc), &oidcConfig); err != nil {
			return fmt.Errorf("invalid oidc config for gateway %s: %w", gw.Name, err)
		}
		gwConfig.OIDC = oidcConfig
	}

	if gw.Route != nil && *gw.Route != "" {
		var routeConfig gateway.RouteConfig
		if err := json.Unmarshal([]byte(*gw.Route), &routeConfig); err != nil {
			return fmt.Errorf("invalid route config for gateway %s: %w", gw.Name, err)
		}
		gwConfig.Route = routeConfig
	}

	if gw.DatabaseConfig != nil && *gw.DatabaseConfig != "" {
		var dbConfig gateway.DatabaseConfig
		if err := json.Unmarshal([]byte(*gw.DatabaseConfig), &dbConfig); err != nil {
			return fmt.Errorf("invalid database config for gateway %s: %w", gw.Name, err)
		}
		gwConfig.Database = dbConfig
	}

	if gw.CredentialDriver != nil && *gw.CredentialDriver != "" {
		var credDriverConfig gateway.CredentialDriverConfig
		decoder := json.NewDecoder(bytes.NewReader([]byte(*gw.CredentialDriver)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&credDriverConfig); err != nil {
			return fmt.Errorf("invalid credential driver config for gateway %s: %w", gw.Name, err)
		}
		gwConfig.CredentialDriver = &credDriverConfig
	}

	nsConfig := gateway.NamespaceConfig{
		Name:    namespace,
		Gateway: gwConfig,
	}

	opts := gateway.ReconcileOpts{
		IsOpenShift:           r.isOpenShift,
		HasCertManager:        r.hasCertManager,
		HasGatewayAPI:         r.hasGatewayAPI,
		ControlPlaneNamespace: r.controlPlaneNamespace,
		GatewayID:             event.ResourceID,
		UpdateRouteAddress:    r.makeRouteAddressUpdater(event.ResourceID),
		Keycloak:              r.keycloakConfig,
		KeycloakClient:        r.keycloakClient,
		GatewayName:           gw.Name,
		UpdateOIDC:            r.makeOIDCUpdater(event.ResourceID),
	}

	r.updateGatewayPhase(ctx, event.ResourceID, "Provisioning")

	if err := gateway.ReconcileGateway(ctx, r.dynamicClient, r.clientset, nsConfig, r.manifests, opts); err != nil {
		r.updateGatewayPhase(ctx, event.ResourceID, "Failed")
		return fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
	}

	// Manifests are applied, but the gateway is not Running until its workload
	// is observed Ready. Wait within the provisioning readiness window: on
	// readiness set Running; otherwise set Degraded and record why. The
	// continuous health reconciler keeps the phase synchronized thereafter.
	ready, reason := gateway.WaitForGatewayReady(ctx, r.clientset, namespace, 2*time.Minute)
	if ready {
		r.updateGatewayHealth(ctx, event.ResourceID, "Running", "Healthy")
		log.Printf("INFO gateway %s provisioned and ready in namespace %s", gw.Name, namespace)
	} else {
		r.updateGatewayHealth(ctx, event.ResourceID, "Degraded", reason)
		log.Printf("WARN gateway %s applied but not ready in namespace %s: %s", gw.Name, namespace, reason)
	}
	return nil
}

// gatewayNamespace returns the Kubernetes namespace for a Gateway, deriving the
// conventional `openshell-<name>` namespace when the resource does not carry an
// explicit one.
func gatewayNamespace(gw *pb.Gateway) string {
	if gw.GetNamespace() != "" {
		return gw.GetNamespace()
	}
	return fmt.Sprintf("openshell-%s", gw.GetName())
}

// updateGatewayHealth sets the Gateway `phase` and `status` together in a single
// gRPC update so the console and CLI observe a consistent lifecycle state and
// health descriptor.
func (r *GatewayReconciler) updateGatewayHealth(ctx context.Context, gatewayID, phase, status string) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:     gatewayID,
		Phase:  &phase,
		Status: &status,
	})
	if err != nil {
		log.Printf("WARN failed to update gateway %s health to %s (%s): %v", gatewayID, phase, status, err)
	}
}

func (r *GatewayReconciler) updateGatewayPhase(ctx context.Context, gatewayID string, phase string) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Phase: &phase,
	})
	if err != nil {
		log.Printf("WARN failed to update gateway %s phase to %s: %v", gatewayID, phase, err)
	}
}

// makeRouteAddressUpdater returns a RouteAddressUpdater callback that PATCHes
// the route_address field on the API-server Gateway via gRPC.
func (r *GatewayReconciler) makeRouteAddressUpdater(gatewayID string) gateway.RouteAddressUpdater {
	return func(ctx context.Context, routeAddress string) error {
		return r.updateRouteAddress(ctx, gatewayID, routeAddress)
	}
}

func (r *GatewayReconciler) updateRouteAddress(ctx context.Context, gatewayID string, routeAddress string) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:           gatewayID,
		RouteAddress: &routeAddress,
	})
	if err != nil {
		return fmt.Errorf("update gateway %s route_address to %s: %w", gatewayID, routeAddress, err)
	}
	return nil
}

func (r *GatewayReconciler) makeOIDCUpdater(gatewayID string) func(ctx context.Context, oidcJSON string) error {
	return func(ctx context.Context, oidcJSON string) error {
		client := pb.NewGatewayServiceClient(r.grpcConn)
		_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
			Id:   gatewayID,
			Oidc: &oidcJSON,
		})
		if err != nil {
			return fmt.Errorf("update gateway %s oidc: %w", gatewayID, err)
		}
		return nil
	}
}

type StubGatewayReconciler struct{}

func NewStubGatewayReconciler() *StubGatewayReconciler {
	return &StubGatewayReconciler{}
}

func (r *StubGatewayReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Gateway]) error {
	log.Printf("INFO [stub] reconciling Gateway %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

type GatewayNetworkReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewGatewayNetworkReconciler() *GatewayNetworkReconciler {
	return &GatewayNetworkReconciler{active: make(map[string]struct{})}
}

func (r *GatewayNetworkReconciler) Handle(ctx context.Context, event watcher.Event[*pb.GatewayNetwork]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	log.Printf("INFO reconciling GatewayNetwork %s (event=%d)", event.ResourceID, event.Type)
	return nil
}
