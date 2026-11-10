package defaultworkspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/serviceaccountkeycloak"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	BootstrapAdminServiceAccountID = "bootstrap-admin"
	BootstrapAdminClientIDPrefix   = "hs-bootstrap-"
	BootstrapAdminDisplayName      = "HyperShell Gateway Bootstrap Admin"
	BootstrapAdminCreatorUserID    = "hypershell-system"
	AccessTokenLifetimeSeconds     = 300

	bootstrapAdminSecretName = "hypershell-bootstrap-admin"
	clientTLSSecretName      = "openshell-client-tls"
	clientIDSecretKey        = "client-id"
	clientUUIDSecretKey      = "client-uuid"
	clientSecretSecretKey    = "client-secret"
	subjectSecretKey         = "subject"
)

var (
	errBootstrapAdminCredentialMissing = errors.New("gateway bootstrap admin exists but its Kubernetes credential is missing")
	ErrInvalidRequest                  = errors.New("invalid default-workspace bootstrap request")
)

type Provider interface {
	Configured() bool
	Issuer() string
	ProvisionServiceAccount(context.Context, serviceaccountkeycloak.ServiceAccountSpec) (*serviceaccountkeycloak.ProvisionedServiceAccount, error)
	DeleteManagedServiceAccount(context.Context, string, string) error
	ListManagedClients(context.Context, string) ([]serviceaccountkeycloak.ManagedClient, error)
}

type OpenShellClient interface {
	AddMember(ctx context.Context, endpoint, namespace, accessToken, subject, role string) error
}

type AdminSpec struct {
	GatewayID        string
	GatewayNamespace string
	Issuer           string
	GatewayClientID  string
}

type MemberRequest struct {
	AdminSpec
	Endpoint    string
	Subject     string
	SubjectType string
	Role        string
}

type bootstrapAdmin struct {
	ClientID     string
	ClientUUID   string
	ClientSecret string
	Subject      string
}

type Manager struct {
	provider   Provider
	kube       kubernetes.Interface
	openShell  OpenShellClient
	httpClient *http.Client
	locks      sync.Map
}

func NewManager(provider Provider, kube kubernetes.Interface, openShell OpenShellClient) *Manager {
	return &Manager{
		provider: provider, kube: kube, openShell: openShell,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (m *Manager) Configured() bool {
	return m != nil && m.provider != nil && m.provider.Configured() && m.kube != nil && m.openShell != nil
}

func (m *Manager) AddMember(ctx context.Context, request MemberRequest) error {
	if !m.Configured() {
		return errors.New("default-workspace bootstrap manager is not configured")
	}
	if err := validateMemberRequest(request); err != nil {
		return err
	}
	admin, err := m.ensureAdmin(ctx, request.AdminSpec)
	if err != nil {
		return err
	}
	accessToken, err := m.accessToken(ctx, m.provider.Issuer(), admin)
	if err != nil {
		return fmt.Errorf("authenticate gateway bootstrap admin: %w", err)
	}
	if err := m.openShell.AddMember(
		ctx, clusterLocalGatewayEndpoint(request.GatewayNamespace), request.GatewayNamespace,
		accessToken, request.Subject, request.Role,
	); err != nil {
		return fmt.Errorf("add default-workspace member: %w", err)
	}
	return nil
}

func clusterLocalGatewayEndpoint(namespace string) string {
	return fmt.Sprintf("https://openshell-gateway.%s.svc.cluster.local:8080", namespace)
}

func (m *Manager) EnsureAdmin(ctx context.Context, spec AdminSpec) error {
	_, err := m.ensureAdmin(ctx, spec)
	return err
}

func (m *Manager) ensureAdmin(ctx context.Context, spec AdminSpec) (*bootstrapAdmin, error) {
	if err := validateAdminSpec(spec); err != nil {
		return nil, err
	}
	lock, _ := m.locks.LoadOrStore(spec.GatewayID, &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()

	admin, err := m.readAdmin(ctx, spec.GatewayNamespace)
	if err != nil {
		return nil, err
	}
	clients, err := m.provider.ListManagedClients(ctx, spec.GatewayID)
	if err != nil {
		return nil, fmt.Errorf("check existing gateway bootstrap admin: %w", err)
	}

	var liveClient *serviceaccountkeycloak.ManagedClient
	for index := range clients {
		if clients[index].ServiceAccountID == BootstrapAdminServiceAccountID {
			liveClient = &clients[index]
			break
		}
	}
	if admin != nil {
		if liveClient != nil {
			if admin.ClientID == liveClient.ClientID && admin.ClientUUID == liveClient.UUID {
				return admin, nil
			}
			return nil, errBootstrapAdminCredentialMissing
		}
		if err := m.kube.CoreV1().Secrets(spec.GatewayNamespace).Delete(
			ctx, bootstrapAdminSecretName, metav1.DeleteOptions{},
		); err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("remove stale gateway bootstrap admin secret: %w", err)
		}
	} else if liveClient != nil {
		return nil, errBootstrapAdminCredentialMissing
	}

	provisioned, err := m.provider.ProvisionServiceAccount(ctx, serviceaccountkeycloak.ServiceAccountSpec{
		ClientID:    BootstrapAdminClientIDPrefix + spec.GatewayID,
		DisplayName: BootstrapAdminDisplayName, GatewayClientID: spec.GatewayClientID,
		GatewayID: spec.GatewayID, ServiceAccountID: BootstrapAdminServiceAccountID,
		CreatorUserID: BootstrapAdminCreatorUserID, Role: serviceaccountkeycloak.RoleAdmin,
		ExpectedIssuer: spec.Issuer, AccessTokenLifetimeSeconds: AccessTokenLifetimeSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("provision gateway bootstrap admin: %w", err)
	}
	admin = &bootstrapAdmin{
		ClientID: provisioned.ClientID, ClientUUID: provisioned.ClientUUID,
		ClientSecret: provisioned.ClientSecret, Subject: provisioned.Subject,
	}
	if err := m.writeAdmin(ctx, spec.GatewayNamespace, admin); err != nil {
		_ = m.provider.DeleteManagedServiceAccount(ctx, spec.GatewayID, BootstrapAdminServiceAccountID)
		return nil, fmt.Errorf("store gateway bootstrap admin credential: %w", err)
	}
	m.recordAdminEvent(ctx, spec.GatewayNamespace, spec.GatewayID, "BootstrapAdminCreated", corev1.EventTypeNormal, "HyperShell created the gateway bootstrap admin")
	return admin, nil
}

func (m *Manager) recordAdminEvent(ctx context.Context, namespace, gatewayID, reason, eventType, message string) {
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "bootstrap-admin-",
			Namespace:    namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Gateway",
			Name:      gatewayID,
			Namespace: namespace,
		},
		Reason:  reason,
		Message: message,
		Type:    eventType,
		Source:  corev1.EventSource{Component: "hypershell-control-plane"},
	}
	if _, err := m.kube.CoreV1().Events(namespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		slog.WarnContext(ctx, "Failed to record gateway bootstrap-admin event",
			"gateway_id", gatewayID,
			"namespace", namespace,
			"reason", reason,
			"error", err,
		)
	}
}

func (m *Manager) readAdmin(ctx context.Context, namespace string) (*bootstrapAdmin, error) {
	secret, err := m.kube.CoreV1().Secrets(namespace).Get(ctx, bootstrapAdminSecretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read gateway bootstrap admin secret: %w", err)
	}
	admin := &bootstrapAdmin{
		ClientID: string(secret.Data[clientIDSecretKey]), ClientUUID: string(secret.Data[clientUUIDSecretKey]),
		ClientSecret: string(secret.Data[clientSecretSecretKey]), Subject: string(secret.Data[subjectSecretKey]),
	}
	if admin.ClientID == "" || admin.ClientUUID == "" || admin.ClientSecret == "" || admin.Subject == "" {
		return nil, errBootstrapAdminCredentialMissing
	}
	return admin, nil
}

func (m *Manager) writeAdmin(ctx context.Context, namespace string, admin *bootstrapAdmin) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapAdminSecretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.io/component":      "default-workspace-bootstrap",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			clientIDSecretKey: []byte(admin.ClientID), clientUUIDSecretKey: []byte(admin.ClientUUID),
			clientSecretSecretKey: []byte(admin.ClientSecret), subjectSecretKey: []byte(admin.Subject),
		},
	}
	_, err := m.kube.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing, getErr := m.kube.CoreV1().Secrets(namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if getErr != nil {
		return getErr
	}
	if existing.Data == nil {
		return errBootstrapAdminCredentialMissing
	}
	if string(existing.Data[clientIDSecretKey]) != admin.ClientID ||
		string(existing.Data[clientUUIDSecretKey]) != admin.ClientUUID ||
		string(existing.Data[clientSecretSecretKey]) != admin.ClientSecret ||
		string(existing.Data[subjectSecretKey]) != admin.Subject {
		return errBootstrapAdminCredentialMissing
	}
	return nil
}

func validateAdminSpec(spec AdminSpec) error {
	if strings.TrimSpace(spec.GatewayID) == "" || strings.TrimSpace(spec.GatewayNamespace) == "" ||
		strings.TrimSpace(spec.Issuer) == "" || strings.TrimSpace(spec.GatewayClientID) == "" {
		return errors.New("gateway ID, namespace, issuer, and gateway client ID are required")
	}
	return nil
}

func validateMemberRequest(request MemberRequest) error {
	if err := validateAdminSpec(request.AdminSpec); err != nil {
		return err
	}
	if strings.TrimSpace(request.Endpoint) == "" || strings.TrimSpace(request.Subject) == "" ||
		request.Role != "user" ||
		(request.SubjectType != "user" && request.SubjectType != "service_account") {
		return fmt.Errorf("%w: gateway endpoint, subject, subject type, and role user are required", ErrInvalidRequest)
	}
	return nil
}
