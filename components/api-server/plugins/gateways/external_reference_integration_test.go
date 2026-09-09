package gateways_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/plugins/generic"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func TestGatewayExternalReferenceConcurrentReplay(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	client := referenceClient(t, h)
	ctx := referenceContext(t, h, "caller-a")
	reference := "control/instance/project/incarnation"
	input := openapi.GatewayCreateRequest{Name: "bound-project", ExternalReference: &reference}
	first, _, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(input).Execute()
	if err != nil {
		t.Fatal(err)
	}
	const requests = 8
	results := make(chan string, requests)
	failures := make(chan error, requests)
	var group sync.WaitGroup
	for i := 0; i < requests; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			changed := input
			changed.Name = "must-not-update"
			result, response, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(changed).Execute()
			if err != nil {
				failures <- err
				return
			}
			if response.StatusCode != http.StatusCreated || result.Name != first.Name || result.Namespace != first.Namespace {
				failures <- fmt.Errorf("replay changed gateway")
				return
			}
			results <- result.GetId()
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	for id := range results {
		if id != first.GetId() {
			t.Errorf("replay ID %s differs from %s", id, first.GetId())
		}
	}
	list, _, err := client.DefaultAPI.ListGateways(ctx).ExternalReference(reference).Execute()
	if err != nil || len(list.Items) != 1 || list.Items[0].GetId() != first.GetId() {
		t.Fatalf("lookup = %#v, %v", list, err)
	}
	var gatewayCount, bindingCount, databaseCount int64
	session := h.Env().Database.SessionFactory.New(context.Background())
	if err := session.Table("gateways").Where("external_reference = ?", reference).Count(&gatewayCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := session.Table("role_bindings").Where("gateway_id = ? AND deleted_at IS NULL", first.GetId()).Count(&bindingCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := session.Table("managed_databases").Count(&databaseCount).Error; err != nil {
		t.Fatal(err)
	}
	if gatewayCount != 1 || bindingCount != 1 || databaseCount != 1 {
		t.Fatalf("replay created extra records: gateways=%d bindings=%d databases=%d", gatewayCount, bindingCount, databaseCount)
	}

	other := referenceContext(t, h, "caller-b")
	foreign, _, err := client.DefaultAPI.ListGateways(other).ExternalReference(reference).Execute()
	if err != nil || len(foreign.Items) != 0 {
		t.Fatalf("foreign lookup = %#v, %v", foreign, err)
	}
	separate, _, err := client.DefaultAPI.CreateGateway(other).GatewayCreateRequest(input).Execute()
	if err != nil || separate.GetId() == first.GetId() {
		t.Fatalf("foreign creation adopted original: %#v, %v", separate, err)
	}

	// Revocation must remain effective after a lost-response retry.
	if err := session.Exec("UPDATE role_bindings SET deleted_at = now() WHERE gateway_id = ?", first.GetId()).Error; err != nil {
		t.Fatal(err)
	}
	_, response, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(input).Execute()
	if err == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked replay = %v, %v", response, err)
	}
	if err := session.Table("role_bindings").Where("gateway_id = ? AND deleted_at IS NULL", first.GetId()).Count(&bindingCount).Error; err != nil {
		t.Fatal(err)
	}
	if bindingCount != 0 {
		t.Fatal("replay restored revoked access")
	}

	// A tombstone reserves the reference and cannot recreate the resource.
	if err := session.Exec("UPDATE gateways SET deleted_at = now() WHERE id = ?", first.GetId()).Error; err != nil {
		t.Fatal(err)
	}
	_, response, err = client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(input).Execute()
	if err == nil || response.StatusCode != http.StatusConflict {
		t.Fatalf("deleted replay = %v, %v", response, err)
	}
}

type unavailableOwnerBinding struct{}

func (unavailableOwnerBinding) CreateOwnerBinding(context.Context, string, string) error {
	return fmt.Errorf("owner store unavailable")
}

func TestGatewayExternalReferenceRollsBackOwnerFailure(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	client := referenceClient(t, h)
	reference := "control/instance/project/rollback"
	input := openapi.GatewayCreateRequest{Name: "rollback-project", ExternalReference: &reference}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := db.NewContext(context.Background(), h.Env().Database.SessionFactory)
	if err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, rbac.ContextUserIDKey, "caller-for-rollback")
	handler := gateways.NewGatewayHandler(gateways.Service(&h.Env().Services), nil, unavailableOwnerBinding{}, nil, nil)
	response := httptest.NewRecorder()
	handler.Create(response, httptest.NewRequest(http.MethodPost, "/gateways", bytes.NewReader(body)).WithContext(ctx))
	db.Resolve(ctx)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	session := h.Env().Database.SessionFactory.New(context.Background())
	for _, table := range []string{"gateways", "managed_databases"} {
		var count int64
		if err := session.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("owner failure left %d records in %s", count, table)
		}
	}
	var eventCount int64
	if err := session.Table("events").Where("source IN ?", []string{"Gateways", "ManagedDatabases", "RoleBindings"}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatalf("owner failure left %d create events", eventCount)
	}
	authenticated := referenceContext(t, h, "caller-for-retry")
	if _, _, err := client.DefaultAPI.CreateGateway(authenticated).GatewayCreateRequest(input).Execute(); err != nil {
		t.Fatalf("retry after rollback failed: %v", err)
	}
}

// This server uses the production handler, services, DAOs, and transaction
// middleware. The test supplies identities after the authentication boundary.
func referenceClient(t *testing.T, h *test.Helper) *openapi.APIClient {
	t.Helper()
	rb := roleBindings.Service(&h.Env().Services)
	visibility := rbac.NewGatewayVisibilityFilter(func(ctx context.Context, userID string) ([]string, error) {
		ids, err := rb.FindGatewayIDsByUserID(ctx, userID)
		if err != nil {
			return nil, err
		}
		return ids, nil
	})
	handler := gateways.NewGatewayHandler(gateways.Service(&h.Env().Services), generic.Service(&h.Env().Services), rbac.NewGatewayBootstrapper(rb), visibility, rb)
	router := mux.NewRouter()
	router.HandleFunc("/api/hypershell/v1/gateways", handler.Create).Methods(http.MethodPost)
	router.HandleFunc("/api/hypershell/v1/gateways", handler.List).Methods(http.MethodGet)
	router.HandleFunc("/api/hypershell/v1/gateways/deletion", handler.DeletionStatus).Methods(http.MethodGet)
	router.HandleFunc("/api/hypershell/v1/gateways/{id}", handler.Patch).Methods(http.MethodPatch)
	identity := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), rbac.ContextUserIDKey, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		router.ServeHTTP(w, r.WithContext(ctx))
	})
	server := httptest.NewServer(db.TransactionMiddleware(identity, h.Env().Database.SessionFactory))
	t.Cleanup(server.Close)
	config := openapi.NewConfiguration()
	config.Servers = openapi.ServerConfigurations{{URL: server.URL}}
	return openapi.NewAPIClient(config)
}

func referenceContext(t *testing.T, h *test.Helper, username string) context.Context {
	t.Helper()
	id, err := users.Service(&h.Env().Services).UpsertByUsername(context.Background(), username, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return context.WithValue(context.Background(), openapi.ContextAccessToken, id)
}

func TestGatewayExternalReferenceConcurrentFirstCreate(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	client := referenceClient(t, h)
	ctx := referenceContext(t, h, "concurrent-creator")
	reference := "control/instance/project/concurrent"
	input := openapi.GatewayCreateRequest{Name: "concurrent-project", ExternalReference: &reference}
	const requests = 8
	start := make(chan struct{})
	results := make(chan string, requests)
	failures := make(chan error, requests)
	var group sync.WaitGroup
	for i := 0; i < requests; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			result, _, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(input).Execute()
			if err != nil {
				failures <- err
				return
			}
			results <- result.GetId()
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	expected := ""
	for id := range results {
		if expected == "" {
			expected = id
		}
		if id != expected {
			t.Errorf("duplicate gateway: %s and %s", expected, id)
		}
	}
	session := h.Env().Database.SessionFactory.New(context.Background())
	for _, table := range []string{"gateways", "managed_databases", "role_bindings"} {
		var count int64
		if err := session.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("concurrent create left %d records in %s", count, table)
		}
	}
	// Whole-row replacements must preserve the original reference and caller.
	service := gateways.Service(&h.Env().Services)
	current, err := service.Get(context.Background(), expected)
	if err != nil {
		t.Fatal(err)
	}
	altered := "another-reference"
	current.ExternalReference = &altered
	current.ExternalReferenceOwner = &altered
	if _, err := service.Replace(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	stored, err := service.Get(context.Background(), expected)
	if err != nil {
		t.Fatal(err)
	}
	if *stored.ExternalReference != reference || *stored.ExternalReferenceOwner == altered {
		t.Fatal("replace reassigned the reference")
	}
}
