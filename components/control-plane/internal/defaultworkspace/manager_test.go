package defaultworkspace

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openshift-online/hypershell/components/control-plane/internal/serviceaccountkeycloak"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestManagerAddMemberProvisionsAdminAndReusesStoredCredential(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse token form: %v", err)
		}
		if request.Form.Get("grant_type") != "client_credentials" ||
			request.Form.Get("client_id") != "hs-bootstrap-gateway-id" ||
			request.Form.Get("client_secret") != "one-time-secret" {
			t.Errorf("token request form = %#v", request.Form)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"bootstrap-token"}`))
	}))
	defer tokenServer.Close()

	provider := &fakeProvider{issuer: tokenServer.URL}
	openShell := &fakeOpenShellClient{}
	manager := NewManager(provider, fake.NewSimpleClientset(), openShell)
	request := MemberRequest{
		AdminSpec: AdminSpec{
			GatewayID: "gateway-id", GatewayNamespace: "gateway-namespace",
			Issuer: tokenServer.URL, GatewayClientID: "gateway-client",
		},
		Endpoint: "https://gateway.example:443", Subject: "subject-id",
		SubjectType: "user", Role: "user",
	}

	if err := manager.AddMember(t.Context(), request); err != nil {
		t.Fatalf("first AddMember() error = %v", err)
	}
	if err := manager.AddMember(t.Context(), request); err != nil {
		t.Fatalf("second AddMember() error = %v", err)
	}
	if provider.provisionCalls != 1 {
		t.Fatalf("provision calls = %d, want 1", provider.provisionCalls)
	}
	if openShell.calls != 2 || openShell.endpoint != "https://openshell-gateway.gateway-namespace.svc.cluster.local:8080" ||
		openShell.accessToken != "bootstrap-token" || openShell.subject != "subject-id" {
		t.Fatalf("OpenShell calls = %d, endpoint = %q, token = %q, subject = %q",
			openShell.calls, openShell.endpoint, openShell.accessToken, openShell.subject)
	}
}

func TestManagerRecordsBootstrapAdminCreationEvent(t *testing.T) {
	tokenServer := newTokenServer(t)
	defer tokenServer.Close()
	kube := fake.NewSimpleClientset()
	manager := NewManager(&fakeProvider{issuer: tokenServer.URL}, kube, &fakeOpenShellClient{})

	if err := manager.EnsureAdmin(t.Context(), validMemberRequest(tokenServer.URL).AdminSpec); err != nil {
		t.Fatalf("EnsureAdmin() error = %v", err)
	}
	events, err := kube.CoreV1().Events("gateway-namespace").List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 1 || events.Items[0].Reason != "BootstrapAdminCreated" {
		t.Fatalf("events = %#v, want one BootstrapAdminCreated event", events.Items)
	}
}

func TestManagerAddMemberTreatsOpenShellAlreadyExistsAsSuccess(t *testing.T) {
	tokenServer := newTokenServer(t)
	defer tokenServer.Close()
	manager := NewManager(&fakeProvider{issuer: tokenServer.URL}, fake.NewSimpleClientset(), &fakeOpenShellClient{err: status.Error(codes.AlreadyExists, "member exists")})
	request := validMemberRequest("https://public-issuer.example/realms/hypershell")
	if err := manager.AddMember(t.Context(), request); err != nil {
		t.Fatalf("AddMember() error = %v", err)
	}
}

func TestManagerAddMemberRejectsUnknownSubjectType(t *testing.T) {
	request := validMemberRequest("https://issuer.example/realms/hypershell")
	request.SubjectType = "robot"
	manager := NewManager(&fakeProvider{}, fake.NewSimpleClientset(), &fakeOpenShellClient{})

	err := manager.AddMember(t.Context(), request)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("AddMember() error = %v, want ErrInvalidRequest", err)
	}
}

func TestManagerAddMemberReprovisionsAfterBootstrapAdminClientDisappears(t *testing.T) {
	tokenServer := newTokenServer(t)
	defer tokenServer.Close()
	kube := fake.NewSimpleClientset(adminSecret("old-client-id", "old-client-uuid", "old-client-secret", "old-subject"))
	provider := &fakeProvider{issuer: tokenServer.URL}
	manager := NewManager(provider, kube, &fakeOpenShellClient{})

	if err := manager.AddMember(t.Context(), validMemberRequest("https://public-issuer.example/realms/hypershell")); err != nil {
		t.Fatalf("AddMember() error = %v", err)
	}
	if provider.provisionCalls != 1 {
		t.Fatalf("provision calls = %d, want 1", provider.provisionCalls)
	}
	secret, err := kube.CoreV1().Secrets("gateway-namespace").Get(
		t.Context(), bootstrapAdminSecretName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("read replaced secret: %v", err)
	}
	if string(secret.Data[clientIDSecretKey]) != "hs-bootstrap-gateway-id" ||
		string(secret.Data[clientUUIDSecretKey]) != "client-uuid" ||
		string(secret.Data[clientSecretSecretKey]) != "one-time-secret" ||
		string(secret.Data[subjectSecretKey]) != "bootstrap-subject" {
		t.Fatalf("replaced secret data = %#v", secret.Data)
	}
}

func TestManagerAddMemberFailsClosedWhenSecretDoesNotMatchLiveClient(t *testing.T) {
	tokenServer := newTokenServer(t)
	defer tokenServer.Close()
	kube := fake.NewSimpleClientset(adminSecret("old-client-id", "old-client-uuid", "old-client-secret", "old-subject"))
	provider := &fakeProvider{issuer: tokenServer.URL, clients: []serviceaccountkeycloak.ManagedClient{{
		UUID: "live-client-uuid", ClientID: "hs-bootstrap-gateway-id",
		GatewayID: "gateway-id", ServiceAccountID: BootstrapAdminServiceAccountID,
	}}}
	manager := NewManager(provider, kube, &fakeOpenShellClient{})

	err := manager.AddMember(t.Context(), validMemberRequest("https://public-issuer.example/realms/hypershell"))
	if !errors.Is(err, errBootstrapAdminCredentialMissing) {
		t.Fatalf("AddMember() error = %v, want missing credential", err)
	}
	if provider.provisionCalls != 0 {
		t.Fatalf("provision calls = %d, want 0", provider.provisionCalls)
	}
}

func TestWriteAdminReconcilesAnEquivalentExistingSecret(t *testing.T) {
	kube := fake.NewSimpleClientset(adminSecret("client-id", "client-uuid", "client-secret", "subject"))
	manager := NewManager(&fakeProvider{}, kube, &fakeOpenShellClient{})
	if err := manager.writeAdmin(t.Context(), "gateway-namespace", &bootstrapAdmin{
		ClientID: "client-id", ClientUUID: "client-uuid", ClientSecret: "client-secret", Subject: "subject",
	}); err != nil {
		t.Fatalf("writeAdmin() error = %v", err)
	}
}

func TestWriteAdminRejectsConflictingExistingSecret(t *testing.T) {
	kube := fake.NewSimpleClientset(adminSecret("other-client-id", "client-uuid", "client-secret", "subject"))
	manager := NewManager(&fakeProvider{}, kube, &fakeOpenShellClient{})
	err := manager.writeAdmin(t.Context(), "gateway-namespace", &bootstrapAdmin{
		ClientID: "client-id", ClientUUID: "client-uuid", ClientSecret: "client-secret", Subject: "subject",
	})
	if !errors.Is(err, errBootstrapAdminCredentialMissing) {
		t.Fatalf("writeAdmin() error = %v, want missing credential", err)
	}
}

func TestWriteAdminReportsExistingSecretReadFailure(t *testing.T) {
	kube := fake.NewSimpleClientset()
	kube.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewAlreadyExists(corev1.Resource("secrets"), bootstrapAdminSecretName)
	})
	kube.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("cluster unavailable")
	})
	manager := NewManager(&fakeProvider{}, kube, &fakeOpenShellClient{})
	if err := manager.writeAdmin(t.Context(), "gateway-namespace", &bootstrapAdmin{}); err == nil {
		t.Fatal("writeAdmin() error = nil, want read failure")
	}
}

func newTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"bootstrap-token"}`))
	}))
}

func validMemberRequest(issuer string) MemberRequest {
	return MemberRequest{
		AdminSpec: AdminSpec{
			GatewayID: "gateway-id", GatewayNamespace: "gateway-namespace",
			Issuer: issuer, GatewayClientID: "gateway-client",
		},
		Endpoint: "https://gateway.example:443", Subject: "subject-id",
		SubjectType: "user", Role: "user",
	}
}

func adminSecret(clientID, clientUUID, clientSecret, subject string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: bootstrapAdminSecretName, Namespace: "gateway-namespace"},
		Data: map[string][]byte{
			clientIDSecretKey: []byte(clientID), clientUUIDSecretKey: []byte(clientUUID),
			clientSecretSecretKey: []byte(clientSecret), subjectSecretKey: []byte(subject),
		},
	}
}

type fakeProvider struct {
	provisionCalls int
	clients        []serviceaccountkeycloak.ManagedClient
	issuer         string
}

func (f *fakeProvider) Configured() bool { return true }

func (f *fakeProvider) Issuer() string { return f.issuer }

func (f *fakeProvider) ProvisionServiceAccount(_ context.Context, spec serviceaccountkeycloak.ServiceAccountSpec) (*serviceaccountkeycloak.ProvisionedServiceAccount, error) {
	f.provisionCalls++
	f.clients = append(f.clients, serviceaccountkeycloak.ManagedClient{
		UUID: "client-uuid", ClientID: spec.ClientID, GatewayID: spec.GatewayID,
		ServiceAccountID: BootstrapAdminServiceAccountID,
	})
	return &serviceaccountkeycloak.ProvisionedServiceAccount{
		ClientID: spec.ClientID, ClientUUID: "client-uuid",
		ClientSecret: "one-time-secret", Subject: "bootstrap-subject",
	}, nil
}

func (f *fakeProvider) DeleteManagedServiceAccount(context.Context, string, string) error { return nil }

func (f *fakeProvider) ListManagedClients(context.Context, string) ([]serviceaccountkeycloak.ManagedClient, error) {
	return f.clients, nil
}

type fakeOpenShellClient struct {
	calls       int
	endpoint    string
	accessToken string
	subject     string
	err         error
}

func (f *fakeOpenShellClient) AddMember(_ context.Context, endpoint, _, accessToken, subject, _ string) error {
	f.calls++
	f.endpoint, f.accessToken, f.subject = endpoint, accessToken, subject
	if status.Code(f.err) == codes.AlreadyExists {
		return nil
	}
	return f.err
}
