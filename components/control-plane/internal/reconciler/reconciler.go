package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
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
}

func NewGatewayReconciler(
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	grpcConn *grpc.ClientConn,
	manifestsDir string,
	controlPlaneNamespace string,
) (*GatewayReconciler, error) {
	manifests, err := gateway.LoadGatewayManifests(manifestsDir)
	if err != nil {
		return nil, fmt.Errorf("load gateway manifests from %s: %w", manifestsDir, err)
	}

	isOpenShift := gateway.DetectOpenShift(clientset)
	hasCertManager := gateway.DetectCertManager(clientset)
	hasGatewayAPI := gateway.DetectGatewayAPI(clientset)
	log.Printf("INFO gateway reconciler initialized: manifests=%d openshift=%v certmanager=%v gatewayapi=%v", len(manifests), isOpenShift, hasCertManager, hasGatewayAPI)

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
		namespace := gw.Namespace
		if namespace == "" {
			namespace = fmt.Sprintf("openshell-%s", gw.Name)
		}

		log.Printf("INFO gateway %s deleted, cleaning up resources in namespace %s", event.ResourceID, namespace)
		opts := gateway.ReconcileOpts{
			IsOpenShift:           r.isOpenShift,
			HasCertManager:        r.hasCertManager,
			HasGatewayAPI:         r.hasGatewayAPI,
			ControlPlaneNamespace: r.controlPlaneNamespace,
		}
		if err := gateway.DeleteGatewayResources(ctx, r.dynamicClient, r.clientset, namespace, opts); err != nil {
			return fmt.Errorf("delete gateway resources in %s: %w", namespace, err)
		}
		log.Printf("INFO gateway %s resources cleaned up from namespace %s", event.ResourceID, namespace)
		return nil
	}

	log.Printf("INFO reconciling Gateway %s name=%s namespace=%s (event=%d)",
		event.ResourceID, gw.Name, gw.Namespace, event.Type)

	if gw.Phase != nil && (*gw.Phase == "Running" || *gw.Phase == "Provisioning") {
		log.Printf("DEBUG gateway %s phase=%s, skipping reconciliation", event.ResourceID, *gw.Phase)
		return nil
	}

	namespace := gw.Namespace
	if namespace == "" {
		namespace = fmt.Sprintf("openshell-%s", gw.Name)
	}

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

	// When a gateway is created without OIDC and the platform has an issuer
	// configured, default its OIDC so it is authenticated and the console/CLI
	// receive the issuer, client ID, and audience. The default is persisted back
	// to the Gateway below so clients can read it.
	defaultedOIDC := false
	if gwConfig.OIDC.Issuer == "" {
		if oidcDefault, ok := resolveDefaultGatewayOIDC(); ok {
			gwConfig.OIDC = oidcDefault
			defaultedOIDC = true
		}
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
	}

	r.updateGatewayPhase(ctx, event.ResourceID, "Provisioning")

	// Persist the defaulted OIDC after the phase is set so the resulting update
	// event is ignored by the phase gate rather than triggering re-reconciliation.
	if defaultedOIDC {
		r.updateGatewayOIDC(ctx, event.ResourceID, gwConfig.OIDC)
	}

	if err := gateway.ReconcileGateway(ctx, r.dynamicClient, r.clientset, nsConfig, r.manifests, opts); err != nil {
		r.updateGatewayPhase(ctx, event.ResourceID, "Failed")
		return fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
	}

	r.updateGatewayPhase(ctx, event.ResourceID, "Running")
	log.Printf("INFO gateway %s provisioned in namespace %s", gw.Name, namespace)
	return nil
}

// resolveDefaultGatewayOIDC builds the default gateway OIDC configuration from
// control-plane environment. It returns false when no platform issuer is
// configured, in which case gateways remain unauthenticated.
func resolveDefaultGatewayOIDC() (gateway.OIDCConfig, bool) {
	issuer := os.Getenv("GATEWAY_OIDC_ISSUER")
	if issuer == "" {
		issuer = os.Getenv("OIDC_ISSUER")
	}
	if issuer == "" {
		return gateway.OIDCConfig{}, false
	}

	clientID := os.Getenv("GATEWAY_OIDC_CLIENT_ID")
	if clientID == "" {
		clientID = "openshell-cli"
	}
	audience := os.Getenv("GATEWAY_OIDC_AUDIENCE")
	if audience == "" {
		audience = "openshell-cli"
	}

	return gateway.OIDCConfig{
		Issuer:   issuer,
		ClientID: clientID,
		Audience: audience,
	}, true
}

// updateGatewayOIDC persists an OIDC configuration onto the Gateway resource via
// gRPC so the console and CLI can read the issuer, client ID, and audience.
func (r *GatewayReconciler) updateGatewayOIDC(ctx context.Context, gatewayID string, oidc gateway.OIDCConfig) {
	data, err := json.Marshal(oidc)
	if err != nil {
		log.Printf("WARN failed to marshal default oidc for gateway %s: %v", gatewayID, err)
		return
	}
	oidcJSON := string(data)

	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err = client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:   gatewayID,
		Oidc: &oidcJSON,
	})
	if err != nil {
		log.Printf("WARN failed to update gateway %s oidc: %v", gatewayID, err)
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
