package gateways

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

func TestExternalReferenceValidation(t *testing.T) {
	for _, reference := range []string{"", " ", " leading", "trailing ", "line\nbreak", strings.Repeat("a", 256)} {
		if validateExternalReference(reference) == nil {
			t.Errorf("accepted invalid reference %q", reference)
		}
	}
	for _, reference := range []string{"control/instance/project/generation", "literal' OR 1=1", strings.Repeat("a", 255)} {
		if err := validateExternalReference(reference); err != nil {
			t.Errorf("rejected reference %q: %v", reference, err)
		}
	}
}

func TestExternalReferenceReplayAndScope(t *testing.T) {
	reference, owner := "control/instance/project/generation", "caller-a"
	existing := &Gateway{Meta: api.Meta{ID: "original"}, Name: "original-name", ExternalReference: &reference, ExternalReferenceOwner: &owner}
	dao := NewMockGatewayDao()
	if _, err := dao.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	service := &sqlGatewayService{gatewayDao: dao}
	ctx := context.WithValue(context.Background(), rbac.ContextUserIDKey, owner)
	result, err := service.Create(ctx, &Gateway{Name: "changed-name", ExternalReference: &reference})
	if err != nil || result.ID != existing.ID || result.Name != existing.Name || !result.replayed {
		t.Fatalf("replay = %#v, %v", result, err)
	}
	if existing.replayed {
		t.Fatal("replay mutated the stored model")
	}
	foreign := context.WithValue(context.Background(), rbac.ContextUserIDKey, "caller-b")
	if _, err := service.FindByExternalReference(foreign, reference); err == nil || err.HttpCode != http.StatusNotFound {
		t.Fatalf("foreign lookup = %v", err)
	}
	if _, err := service.Create(context.Background(), &Gateway{ExternalReference: &reference}); err == nil || err.HttpCode != http.StatusForbidden {
		t.Fatalf("anonymous create = %v", err)
	}
	existing.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	if _, err := service.Create(ctx, &Gateway{ExternalReference: &reference}); err == nil || err.HttpCode != http.StatusConflict {
		t.Fatalf("deleted replay = %v", err)
	}
	if _, err := service.FindByExternalReference(ctx, reference); err == nil || err.HttpCode != http.StatusNotFound {
		t.Fatalf("deleted lookup = %v", err)
	}
}
